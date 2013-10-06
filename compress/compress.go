package compress

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"strings"
)

const ContentTypeHeader = "Content-Type"
const ContentEncodingHeader = "Content-Encoding"
const AcceptEncodingHeader = "Accept-Encoding"

// Objects implementing the MimePolicy interface can be used to determine what
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

// Default MimePolicy is the default mime policy that allows some of the common
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

// Objects implementing the WriterFactory can be used to create new
// writer.
type WriterFactory interface {
	NewWriter(http.ResponseWriter) (io.WriteCloser, error)
}

// The WriterFactoryFunc type is an adapter to allow the use of
// ordinary functions as WriterFactory. If f is a function with the
// appropriate signature, WriterFactoryFunc(f) is a WriterFactory
// object that calls f.
type WriterFactoryFunc func(http.ResponseWriter) (io.WriteCloser, error)

// NewWriter calls f(w).
func (f WriterFactoryFunc) NewWriter(w http.ResponseWriter) (io.WriteCloser, error) {
	return f(w)
}

// DefaultGzipWriterFactory is the default compress factory using "gzip" encoding.
var DefaultGzipWriterFactory WriterFactory = WriterFactoryFunc(
	func(w http.ResponseWriter) (io.WriteCloser, error) {
		return gzip.NewWriter(w), nil
	})

// DefaultGzipWriterFactory is the default compress factory using "deflate" encoding.
var DefaultDeflateWriterFactory WriterFactory = WriterFactoryFunc(
	func(w http.ResponseWriter) (io.WriteCloser, error) {
		return flate.NewWriter(w, -1)
	})

// Objects implementing the EncodingFactory can be used to create new
// WriterFactory according to the "Accept-Encoding".
type EncodingFactory interface {
	// NewWriterFactory returns a WriterFactory and it's content encoding that
	// matche acceptEncoding(should be the value of "Accept-Encoding" in the
	// http request header). (nil, "") if encoding is not supported.
	NewWriterFactory(acceptEncoding string) (wf WriterFactory, matchedEncoding string)
}

// The EncodingFactoryFunc type is an adapter to allow the use of
// ordinary functions as EncodingFactory. If f is a function with the
// appropriate signature, EncodingFactoryFunc(f) is a
// EncodingFactory object that calls f.
type EncodingFactoryFunc func(acceptEncoding string) (wf WriterFactory, matchedEncoding string)

// NewWriterFactory calls f(acceptEncoding).
func (f EncodingFactoryFunc) NewWriterFactory(acceptEncoding string) (wf WriterFactory, matchedEncoding string) {
	return f(acceptEncoding)
}

// DefaultEncodingFactory is the default encoding factory for "gzip"
// and "deflate".
//
// This factory uses the position in string as the priority of encoding selection.
// It selects the first known encoding.
var DefaultEncodingFactory = EncodingFactoryFunc(func(acceptEncoding string) (wf WriterFactory, matchedEncoding string) {
	var l = len(acceptEncoding)
	var b int = -1
	var e int = -1
	// Not using 'for i, r := range ...' on purpose.
	// Accept-Encoding value should be in Latin-1, so optimize.
	for i := 0; i <= l; i++ {
		var tok string
		if b == -1 {
			if i == l { // EOF
				return nil, ""
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
			return DefaultGzipWriterFactory, tok
		case "deflate":
			return DefaultDeflateWriterFactory, tok
		}
	}

	return nil, ""

	////// ---> Or the easy-to-understand version:
	//for _, enc := range strings.Split(acceptEncoding, ",") {
	//	switch strings.TrimSpace(enc) {
	//	case "gzip":
	//		return DefaultGzipWriterFactory, "gzip"
	//	case "deflate":
	//		return DefaultDeflateWriterFactory, "deflate"
	//	}
	//}
	//// No supported encoding.
	//return nil, ""
})

type responseWriter struct {
	respw           http.ResponseWriter
	policy          MimePolicy
	factory         WriterFactory
	contentEncoding string
	headerWritten   bool
	// Buffer for minSizeToCompress in newResponseWriter() function.
	// See Write method.
	buf []byte
	// Whether buf is not needed.
	buffered bool

	// The final writer. Nil: not determined Non-nil: the final writer to use,
	// eithre http.Response it self or its gzip.Writer wrapper.
	w io.Writer
	c io.Closer
}

// Only responses with larger body length than MinSizeToCompress are compressed.
// Small data is not worth compressing.
const MinSizeToCompress = 256

func newResponseWriter(w http.ResponseWriter, policy MimePolicy, factory WriterFactory, contentEncoding string) *responseWriter {
	return &responseWriter{
		respw:           w,
		policy:          policy,
		factory:         factory,
		contentEncoding: contentEncoding,
		buf:             make([]byte, 0, MinSizeToCompress)}
}

const responseWriterCacheSize = 1024

var responseWriterCache = make(chan *responseWriter, responseWriterCacheSize)

// newResponseWriterCached returns a cached responseWriter if any available, or newly created one.
func newResponseWriterCached(w http.ResponseWriter, policy MimePolicy, factory WriterFactory, contentEncoding string) (writer *responseWriter) {
	select {
	case writer = <-responseWriterCache:
		writer.Reset(w, policy, factory, contentEncoding)
	default:
		writer = newResponseWriter(w, policy, factory, contentEncoding)
	}
	return
}

func returnResponseWriterToCache(writer *responseWriter) {
	select {
	case responseWriterCache <- writer:
	default:
		// Cache is full. Let it go.
	}
}

func (w *responseWriter) Reset(respw http.ResponseWriter, policy MimePolicy, factory WriterFactory, contentEncoding string) {
	w.respw = respw
	w.policy = policy
	w.factory = factory
	w.contentEncoding = contentEncoding
	w.headerWritten = false
	w.buf = w.buf[:0]
	w.w = nil
	w.c = nil
}

func (w *responseWriter) Header() http.Header {
	return w.respw.Header()
}

func (w *responseWriter) Close() (err error) {
	// Write any buffering data if any.
	if !w.buffered && len(w.buf) > 0 {
		_, err = w.write(w.buf, true)
		return err
	}
	if w.c != nil {
		err := w.c.Close()
		return err
	}
	return nil
}

func (w *responseWriter) Write(data []byte) (int, error) {
	origDataLen := len(data)
	if origDataLen == 0 {
		return 0, nil
	}
	// buf is not needed. Write through.
	if w.buffered {
		return w.write(data, false)
	}
	// Get available space. Never grow over capaticy.
	avail := cap(w.buf) - len(w.buf)
	if len(data) <= avail {
		// Write all to buf.
		w.buf = append(w.buf, data...)
		data = nil
		return origDataLen, nil
	} else {
		// Fill up buf.
		w.buf = append(w.buf, data[:avail]...)
		data = data[avail:]
		// Write buf.
		n, err := w.write(w.buf, false)
		if err != nil {
			copy(w.buf, w.buf[n:])
			w.buf = w.buf[:cap(w.buf)-n]
			return avail, err
		}
		// buf is no longer needed.
		w.buffered = true
		// Write remaining data.
		n, err = w.write(data, false)
		if err != nil {
			return avail + n, err
		}
		return origDataLen, nil
	}
}

func (w *responseWriter) write(data []byte, final bool) (int, error) {
	// The final writter is determined, just write to it.
	if w.w != nil {
		return w.w.Write(data)
	}

	// "Content-Encoding" has been written or set by client code. Respect it.
	if w.headerWritten || w.respw.Header().Get(ContentEncodingHeader) != "" {
		w.w = w.respw
		return w.w.Write(data)
	}

	contentType := w.respw.Header().Get(ContentTypeHeader)
	if contentType == "" {
		contentType = http.DetectContentType(data)
		// Write header with detected MIME type.
		w.respw.Header().Set(ContentTypeHeader, contentType)
	}

	// Compress or not.
	// If final is true, there is no enough data to fill the buffer until
	// responseWriter.Close() is called, so the whole response is smaller than
	// minSizeToCompress of newResponseWriter(). Do not compress.
	if !final && w.policy.AllowCompress(contentType) {
		w.respw.Header().Set(ContentEncodingHeader, w.contentEncoding)
		witeCloser, err := w.factory.NewWriter(w.respw)
		if err != nil {
			return 0, err
		}
		w.w = witeCloser
		w.c = witeCloser
	} else {
		w.w = w.respw
	}
	return w.w.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.respw.WriteHeader(statusCode)
	w.headerWritten = true
}

// Handler function wraps a http handler to use http compression.
// policy determines what MIME types are allowed to be compressed, DefaultMimePolicy
// if nill.
// encFactory is used to create WriterFactory, DefaultEncodingFactoryif nil.
func Handler(h http.Handler, policy MimePolicy, encFactory EncodingFactory) http.Handler {
	if policy == nil {
		policy = DefaultMimePolicy
	}
	if encFactory == nil {
		encFactory = DefaultEncodingFactory
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writerFactory, encoding := encFactory.NewWriterFactory(r.Header.Get(AcceptEncodingHeader)); writerFactory != nil && encoding != "" {
			cw := newResponseWriterCached(w, policy, writerFactory, encoding)
			defer func() {
				if err := cw.Close(); err != nil {
					log.Fatal("Colse responseWriter failed: %v\n", err)
				}
				returnResponseWriterToCache(cw)
			}()
			w = cw
		}
		h.ServeHTTP(w, r)
	})
}

// DefaultHandler calls Handler(h, nil, nil)
func DefaultHandler(h http.Handler) http.Handler {
	return Handler(h, nil, nil)
}
