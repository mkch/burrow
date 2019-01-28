package compress

import (
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"log"
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

// DefaultMimePolicy is the default mime policy that allows some of the common
// data types which should be compressed.
var DefaultMimePolicy = MimePolicyFunc(func(mime string) bool {
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

// WriterFactory creates new WriteCloser.
type WriterFactory interface {
	NewWriter(io.Writer) (io.WriteCloser, error)
	ContentEncoding() string
}

type defaultGzipWriterFactory struct{}

func (defaultGzipWriterFactory) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return gzip.NewWriter(w), nil
}

func (defaultGzipWriterFactory) ContentEncoding() string {
	return "gzip"
}

// DefaultGzipWriterFactory is the default compress factory using "gzip" encoding.
var DefaultGzipWriterFactory WriterFactory = defaultGzipWriterFactory{}

type defaultDeflateWriterFactory struct{}

func (defaultDeflateWriterFactory) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return flate.NewWriter(w, -1)
}

func (defaultDeflateWriterFactory) ContentEncoding() string {
	return "deflate"
}

// DefaultDeflateWriterFactory is the default compress factory using "deflate" encoding.
var DefaultDeflateWriterFactory WriterFactory = defaultDeflateWriterFactory{}

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

// DefaultEncodingFactory is the default encoding factory for "gzip"
// and "deflate".
//
// This factory uses the position in string as the priority of encoding selection.
// It selects the first known encoding.
var DefaultEncodingFactory = EncodingFactoryFunc(func(acceptEncoding string) WriterFactory {
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
	prefix    []byte
	prefixLen int
	w         prefixWriteCloser // The destination writer. Nil if pWriter was closed.
}

// newPrefixDefinedWriter creates a prefixDefinedWriter which writes the first prefixLen bytes
// with writer.WritePrefix and writes any bytes following with writer.Write.
func newPrefixDefinedWriter(writer prefixWriteCloser, prefixLen int) *prefixDefinedWriter {
	if prefixLen <= 0 {
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
	if prefixLen <= 0 {
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
	avail := w.prefixLen - len(w.prefix)
	if avail == 0 {
		// w.w.WritePrefix has been called already.
		return w.w.Write(p)
	} else if avail > size {
		// Not enough bytes for prefix.
		w.prefix = append(w.prefix, p...)
		return size, nil
	} else {
		w.prefix = append(w.prefix, p[:avail]...)
		n, err := w.w.WritePrefix(w.prefix)
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

type compresser io.WriteCloser

type compressWriter struct {
	compresser        compresser
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

func internalNewResponseWriterRaw(w http.ResponseWriter, mimePolicy MimePolicy, writerFactory WriterFactory, minSizeToCompress int) (result *responseWriter) {
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

var responseWriterPool sync.Pool

// newResponseWriterCached returns a cached responseWriter if any available, or newly created one.
func newResponseWriter(w http.ResponseWriter, mimePolicy MimePolicy, writerFactory WriterFactory, minSizeToCompress int) (writer *responseWriter) {
	cached := responseWriterPool.Get()
	if cached != nil {
		writer = cached.(*responseWriter)
		writer.Reset(w, mimePolicy, writerFactory, minSizeToCompress)
	} else {
		writer = internalNewResponseWriterRaw(w, mimePolicy, writerFactory, minSizeToCompress)
	}
	return
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

func (w *responseWriter) Close() (err error) {
	if w.w == nil {
		return
	}
	err = w.w.Close()
	responseWriterPool.Put(w)
	return
}

func (w *responseWriter) Write(data []byte) (int, error) {
	return w.w.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.responseWriter.WriteHeader(statusCode)
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
	MinSizeToCompress int
}

// NewHandler function creates a Handler which compresses the response of h.
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
