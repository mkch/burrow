package compress

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

const contentTypeHeader = "Content-Type"
const contentEncodingHeader = "Content-Encoding"
const acceptEncodingHeader = "Accept-Encoding"

// MimePolicy interface can be used to determine what
// MIME types are allowed to be compressed.
//
// See Handler function for details.
type MimePolicy interface {
	// AllowCompress returns true to allowed compression or false to prevent.
	AllowCompress(mime string) bool
}

// The MimePolicyFunc type is an adapter to allow the use of ordinary functions
// as MimePolicy. If f is a function with the appropriate signature,
// MimePolicyFunc(f) is a MimePolicy object that calls f.
type MimePolicyFunc func(string) bool

// AllowCompress calls f(mime).
func (f MimePolicyFunc) AllowCompress(mime string) bool {
	return f(mime)
}

// DefaultMimePolicy is the default MimePolicy that allows some of the common
// data types which should be compressed.
var DefaultMimePolicy = defaultMimePolicy
var defaultMimePolicy = MimePolicyFunc(func(mime string) bool {
	switch mime {
	case
		"application/json",
		"application/javascript",
		"image/bmp":
		return true
	default:
		return strings.HasPrefix(mime, "text/") ||
			strings.HasSuffix(mime, "/xml") ||
			strings.HasSuffix(mime, "+xml")

	}
})

// Writer interface is a compress writer.
type Writer interface {
	io.WriteCloser
	Reset(w io.Writer)
}

// WriterFactory creates new WriteCloser.
type WriterFactory interface {
	NewWriter(io.Writer) (Writer, error)
	ContentEncoding() string
}

type pooledGzipWriter struct {
	Writer
}

func (w pooledGzipWriter) Close() (err error) {
	err = w.Writer.Close()
	(*sync.Pool)(&defaultGzipWriterFactory).Put(w)
	return
}

type pooledGzipWriterFactory sync.Pool

func (f *pooledGzipWriterFactory) NewWriter(w io.Writer) (Writer, error) {
	if cached := (*sync.Pool)(f).Get(); cached != nil {
		result := cached.(Writer)
		result.Reset(w)
		return result, nil
	}
	return pooledGzipWriter{Writer: gzip.NewWriter(w)}, nil
}

func (*pooledGzipWriterFactory) ContentEncoding() string {
	return "gzip"
}

// Used by pooledGzipWriter.Close().
var defaultGzipWriterFactory pooledGzipWriterFactory

// DefaultGzipWriterFactory is the default WriterFactory of "gzip" encoding.
var DefaultGzipWriterFactory = &defaultGzipWriterFactory

type pooledDeflateWriter struct {
	Writer
}

func (w pooledDeflateWriter) Close() (err error) {
	err = w.Writer.Close()
	(*sync.Pool)(&defaultDeflateWriterFactory).Put(w)
	return
}

type pooledDeflateWriterFactory sync.Pool

func (f *pooledDeflateWriterFactory) NewWriter(w io.Writer) (Writer, error) {
	if cached := (*sync.Pool)(f).Get(); cached != nil {
		result := cached.(Writer)
		result.Reset(w)
		return result, nil
	}
	writer, err := flate.NewWriter(w, -1)
	if err != nil {
		return nil, err
	}
	return pooledDeflateWriter{Writer: writer}, nil
}

func (*pooledDeflateWriterFactory) ContentEncoding() string {
	return "deflate"
}

// Used by pooledDeflateWriter.Close().
var defaultDeflateWriterFactory pooledDeflateWriterFactory

// DefaultDeflateWriterFactory is the default WriterFactory of "deflate" encoding.
var DefaultDeflateWriterFactory = &defaultDeflateWriterFactory

// EncodingFactory is the interfact to create new
// WriterFactory according to the "Accept-Encoding".
type EncodingFactory interface {
	// NewWriterFactory returns a WriterFactory matches acceptEncoding(should be
	// the value of "Accept-Encoding" in the http request header).
	// Returns nil if encoding is not supported.
	NewWriterFactory(acceptEncoding string) WriterFactory
}

// The EncodingFactoryFunc type is an adapter to allow the use of
// ordinary functions as EncodingFactory. If f is a function with the
// appropriate signature, EncodingFactoryFunc(f) is a
// EncodingFactory object that calls f.
type EncodingFactoryFunc func(acceptEncoding string) WriterFactory

// NewWriterFactory calls f(acceptEncoding).
func (f EncodingFactoryFunc) NewWriterFactory(acceptEncoding string) WriterFactory {
	return f(acceptEncoding)
}

// DefaultEncodingFactory is the default EncodingFactory for "gzip" and "deflate" encoding.
// This factory uses the position in string as the priority of encoding selection.
// It selects the first known encoding.
var DefaultEncodingFactory = defaultEncodingFactory
var defaultEncodingFactory = EncodingFactoryFunc(func(acceptEncoding string) WriterFactory {
	var l = len(acceptEncoding)
	var b int = -1
	var e int = -1
	// Not using 'for i, r := range ...' on purpose.
	// Accept-Encoding value should be in Latin-1, so optimize.
	for i := 0; i <= l; i++ {
		var tok string
		if b == -1 {
			if i == l { // EOF
				return nil
			}
			r := acceptEncoding[i]
			if r != ' ' && r != ',' {
				b = i
				e = i
			}
			continue
		} else {
			if i == l { // EOF
				tok = acceptEncoding[b : e+1]
			} else {
				r := acceptEncoding[i]
				if r == ',' {
					tok = acceptEncoding[b : e+1]
					b = -1
					e = -1
				} else {
					if r != ' ' {
						e = i
					}
					continue
				}
			}
		}

		switch tok {
		case "gzip":
			return DefaultGzipWriterFactory
		case "deflate":
			return DefaultDeflateWriterFactory
		}
	}

	return nil

	////// ---> Or the easy-to-understand version:
	//for _, enc := range strings.Split(acceptEncoding, ",") {
	//	switch strings.TrimSpace(enc) {
	//	case "gzip":
	//		return DefaultGzipWriterFactory
	//	case "deflate":
	//		return DefaultDeflateWriterFactory
	//	}
	//}
	//// No supported encoding.
	//return nil, ""
})

type prefixWriteCloser interface {
	io.WriteCloser
	// WritePrefix writes the prefix(the first part of data).
	// It should be called zero or one time before any call to Writer.Write.
	WritePrefix([]byte) (n int, err error)
}

type prefixDefinedWriter struct {
	prefix        []byte
	prefixLen     int
	prefixWritten bool
	w             prefixWriteCloser // The destination writer. Nil if pWriter was closed.
}

// newPrefixDefinedWriter creates a prefixDefinedWriter which writes the first prefixLen bytes
// with writer.WritePrefix and writes any bytes following with writer.Write.
// If prefixLen is 0, the data of first Write() of returned prefixDefinedWriter will be the prefix.
func newPrefixDefinedWriter(writer prefixWriteCloser, prefixLen int) *prefixDefinedWriter {
	if prefixLen < 0 {
		panic(fmt.Errorf("newPrefixDefinedWriter: invalid prefixLen %v", prefixLen))
	}
	if writer == nil {
		panic(fmt.Errorf("newPrefixDefinedWriter: nil writer"))
	}
	return &prefixDefinedWriter{
		prefixLen: prefixLen,
		prefix:    make([]byte, 0, prefixLen),
		w:         writer}
}

// Reset discards the prefixDefinedWriter's state and makes it equivalent
// to the result of its original state from newPrefixDefinedWriter.
// This permits reusing a prefixDefinedWriter rather than allocating a new one.
func (w *prefixDefinedWriter) Reset(writer prefixWriteCloser, prefixLen int) {
	if prefixLen < 0 {
		panic(fmt.Errorf("prefixDefinedWriter.Reset: invalid prefixLen %v", prefixLen))
	}
	if writer == nil {
		panic(fmt.Errorf("prefixDefinedWriter.Reset: nil writer"))
	}
	w.prefixLen = prefixLen
	if cap(w.prefix) >= prefixLen {
		w.prefix = w.prefix[:0]
	} else {
		w.prefix = make([]byte, 0, prefixLen)
	}
	w.w = writer
}

func (w *prefixDefinedWriter) Write(p []byte) (int, error) {
	size := len(p)
	if size == 0 {
		return 0, nil
	}
	if w.prefixWritten {
		return w.w.Write(p)
	}
	if w.prefixLen == 0 {
		w.prefixWritten = true
		return w.w.WritePrefix(p)
	}
	avail := w.prefixLen - len(w.prefix)
	if avail > size {
		// Not enough bytes for prefix.
		w.prefix = append(w.prefix, p...)
		return size, nil
	}
	w.prefix = append(w.prefix, p[:avail]...)
	n, err := w.w.WritePrefix(w.prefix)
	w.prefixWritten = true
	pWritten := n - (w.prefixLen - avail)
	if err != nil {
		if pWritten < 0 {
			// No data of p was written.
			pWritten = 0
		}
		return pWritten, err
	}
	n, err = w.w.Write(p[avail:])
	return n + pWritten, err
}

func (w *prefixDefinedWriter) Close() (err error) {
	if w.w == nil {
		// Already closed.
		return
	}
	if w.prefixLen > len(w.prefix) {
		_, err = w.w.WritePrefix(w.prefix)
		if err != nil {
			return
		}
	}
	err = w.w.Close()
	w.w = nil
	return
}

type mimeWriter struct {
	header http.Header
	w      io.WriteCloser
}

func (w *mimeWriter) Reset(header http.Header, writer io.WriteCloser) {
	w.header = header
	w.w = writer
}

func (w *mimeWriter) WritePrefix(p []byte) (int, error) {
	contentType := w.header.Get(contentTypeHeader)
	if contentType == "" {
		contentType = http.DetectContentType(p)
		// Write header with detected MIME type.
		w.header.Set(contentTypeHeader, contentType)
	}
	return w.Write(p)
}

func (w *mimeWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func (w *mimeWriter) Close() error {
	return w.w.Close()
}

type compressWriter struct {
	compresser        Writer
	writerFactory     WriterFactory
	orig              http.ResponseWriter
	mimePolicy        MimePolicy
	minSizeToCompress int
}

func (w *compressWriter) Reset(writerFactory WriterFactory, orig http.ResponseWriter, mimePolicy MimePolicy, minSizeToCompress int) {
	w.compresser = nil
	w.writerFactory = writerFactory
	w.orig = orig
	w.mimePolicy = mimePolicy
	w.minSizeToCompress = minSizeToCompress
}

func (w *compressWriter) WritePrefix(p []byte) (int, error) {
	if len(p) >= w.minSizeToCompress {
		if w.orig.Header().Get(contentEncodingHeader) != "" {
			return w.orig.Write(p)
		}
		if w.mimePolicy.AllowCompress(w.orig.Header().Get(contentTypeHeader)) {
			var err error
			if w.compresser, err = w.writerFactory.NewWriter(w.orig); err != nil {
				return 0, err
			}
			w.orig.Header().Set(contentEncodingHeader, w.writerFactory.ContentEncoding())
		}
	}
	return w.Write(p)
}

func (w *compressWriter) Write(p []byte) (int, error) {
	if w.compresser != nil {
		return w.compresser.Write(p)
	}
	return w.orig.Write(p)
}

func (w *compressWriter) Close() error {
	if w.compresser != nil {
		return w.compresser.Close()
	}
	return nil
}

// A ResponseWriter takes data written to it and writes the compressed form of that data to an underlying ResponseWriter.
type ResponseWriter interface {
	http.ResponseWriter
	io.Closer
}

type responseWriter struct {
	responseWriter http.ResponseWriter
	mimePolicy     MimePolicy
	writerFactory  WriterFactory

	w        *prefixDefinedWriter
	mime     mimeWriter
	cw       *prefixDefinedWriter
	compress compressWriter
}

const mimeDetectBufLen = 512

func internalNewResponseWriter(w http.ResponseWriter, mimePolicy MimePolicy, writerFactory WriterFactory, minSizeToCompress int) (result *responseWriter) {
	result = &responseWriter{
		responseWriter: w,
		mimePolicy:     mimePolicy,
		writerFactory:  writerFactory}

	result.compress.Reset(writerFactory, w, mimePolicy, minSizeToCompress)
	result.cw = newPrefixDefinedWriter(&result.compress, minSizeToCompress)
	result.mime.Reset(w.Header(), result.cw)
	result.w = newPrefixDefinedWriter(&result.mime, mimeDetectBufLen)

	return
}

func internalNewHijackerResponseWriter(w http.ResponseWriter, mimePolicy MimePolicy, writerFactory WriterFactory, minSizeToCompress int) (result *hijackerResponseWriter) {
	return &hijackerResponseWriter{responseWriter: *internalNewResponseWriter(w, mimePolicy, writerFactory, minSizeToCompress)}
}

var responseWriterPool sync.Pool
var hijackerResponseWriterPool sync.Pool

// newResponseWriterCached returns a cached responseWriter if any available, or newly created one.
func newResponseWriter(w http.ResponseWriter, mimePolicy MimePolicy, writerFactory WriterFactory, minSizeToCompress int) ResponseWriter {
	if _, ok := w.(http.Hijacker); ok {
		// w is an http.Hijacker, the return value must be also a hijackerResponseWriter.
		cached := hijackerResponseWriterPool.Get()
		if cached != nil {
			writer := cached.(*hijackerResponseWriter)
			writer.Reset(w, mimePolicy, writerFactory, minSizeToCompress)
			return writer
		}
		return internalNewHijackerResponseWriter(w, mimePolicy, writerFactory, minSizeToCompress)
	}

	cached := responseWriterPool.Get()
	if cached != nil {
		writer := cached.(*responseWriter)
		writer.Reset(w, mimePolicy, writerFactory, minSizeToCompress)
		return writer
	}
	return internalNewResponseWriter(w, mimePolicy, writerFactory, minSizeToCompress)

}

func (w *responseWriter) Reset(respw http.ResponseWriter, mimePolicy MimePolicy, writerFactory WriterFactory, minSizeToCompress int) {
	w.responseWriter = respw
	w.mimePolicy = mimePolicy
	w.writerFactory = writerFactory

	w.compress.Reset(writerFactory, respw, mimePolicy, minSizeToCompress)
	w.cw.Reset(&w.compress, minSizeToCompress)
	w.mime.Reset(w.Header(), w.cw)
	w.w.Reset(&w.mime, mimeDetectBufLen)
}
func (w *responseWriter) Header() http.Header {
	return w.responseWriter.Header()
}

func (w *responseWriter) close() (err error) {
	if w.w == nil {
		return
	}
	err = w.w.Close()
	return
}

func (w *responseWriter) Close() (err error) {
	err = w.close()
	responseWriterPool.Put(w)
	return
}

func (w *responseWriter) Write(data []byte) (int, error) {
	return w.w.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.responseWriter.WriteHeader(statusCode)
}

// ResponseWriter returns the raw http.ResponseWriter.
// For debug purpose only.
func (w *responseWriter) ResponseWriter() http.ResponseWriter {
	return w.responseWriter
}

type hijackerResponseWriter struct {
	responseWriter
}

func (w *hijackerResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.responseWriter.responseWriter.(http.Hijacker).Hijack()
}

func (w *hijackerResponseWriter) Close() (err error) {
	err = w.close()
	hijackerResponseWriterPool.Put(w)
	return
}

// DefaultMinSizeToCompress is the default minimum body size to enable compression.
const DefaultMinSizeToCompress = 1024

// HandlerConfig is used to create a Handler.
type HandlerConfig struct {
	// MimePolicy determines what MIME types are allowed to be compressed. Nil MimePolicy is equivalent to DefaultMimePolicy.
	MimePolicy MimePolicy
	// EncodingFactory is used to create WriterFactory. Nil EncodingFactory is equivalent to DefaultEncodingFactory.
	EncodingFactory EncodingFactory
	// MinSizeToCompress specifies the minimum length of response body that enables compression.
	// Zero MinSizeToCompress is equivalent to DefaultMinSizeToCompress.
	// -1 means no minimum length limit.
	MinSizeToCompress int
}

// NewHandler function creates a Handler which takes response written to it
// and then writes the compressed form of response to h if compression is enabled by config,
// or writes the data to h as-is.
// Parameter config specifies the way the compression performs. Nil config is
// equivalent to &HandlerConfig{}.
func NewHandler(h http.Handler, config *HandlerConfig) http.Handler {
	var mimePolicy MimePolicy
	var encodingFactory EncodingFactory
	var minSizeToCompress int
	if config != nil {
		mimePolicy = config.MimePolicy
		encodingFactory = config.EncodingFactory
		minSizeToCompress = config.MinSizeToCompress
	}
	if mimePolicy == nil {
		mimePolicy = DefaultMimePolicy
	}
	if encodingFactory == nil {
		encodingFactory = DefaultEncodingFactory
	}
	if minSizeToCompress == 0 {
		minSizeToCompress = DefaultMinSizeToCompress
	} else if minSizeToCompress == -1 {
		minSizeToCompress = 0
	} else if minSizeToCompress < 0 {
		panic(fmt.Errorf("NewHandler: invalid minSizeToCompress %v", minSizeToCompress))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writerFactory := encodingFactory.NewWriterFactory(r.Header.Get(acceptEncodingHeader)); writerFactory != nil {
			cw := newResponseWriter(w, mimePolicy, writerFactory, minSizeToCompress)
			defer func() {
				if err := cw.Close(); err != nil {
					log.Fatalf("Close responseWriter failed: %v\n", err)
				}
			}()
			w = cw
		}
		h.ServeHTTP(w, r)
	})
}

type compressResponseWriter struct {
	http.ResponseWriter
	Writer
}

func (w *compressResponseWriter) Write(p []byte) (int, error) {
	return w.Writer.Write(p)
}

func (w *compressResponseWriter) Close() error {
	if w.Writer != nil {
		err := w.Writer.Close()
		w.Writer = nil
		return err
	}
	return nil
}

type hijackerCompressResponseWriter struct {
	compressResponseWriter
}

func (w *hijackerCompressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

// NewResponseWriter function creates a ResponseWriter that takes data written to it
// and then writes the compressed form of that data to w.
// The "Content-Encoding" header of w will be set to the return value of calling writerFactory.ContentEncoding().
func NewResponseWriter(w http.ResponseWriter, writerFactory WriterFactory) (ResponseWriter, error) {
	compresser, err := writerFactory.NewWriter(w)
	if err != nil {
		return nil, err
	}
	var result ResponseWriter
	writer := compressResponseWriter{
		ResponseWriter: w,
		Writer:         compresser,
	}
	if _, ok := w.(http.Hijacker); ok {
		result = &hijackerCompressResponseWriter{
			compressResponseWriter: writer,
		}
	} else {
		result = &compressResponseWriter{
			ResponseWriter: w,
			Writer:         compresser,
		}
	}
	writer.Header().Set(contentEncodingHeader, writerFactory.ContentEncoding())
	return result, nil
}
