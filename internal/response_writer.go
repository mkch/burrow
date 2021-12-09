package internal

import (
	"bufio"
	"net"
	"net/http"
)

type ResponseWriterWrapper interface {
	http.ResponseWriter
	Original() http.ResponseWriter
}

func WrapResponseWriter(w ResponseWriterWrapper) http.ResponseWriter {
	if _, ok := w.Original().(hijackResponseWriter); ok {
		return responseWriterWrapper{w}
	}
	return w
}

type hijackResponseWriter interface {
	http.ResponseWriter
	http.Hijacker
}

type responseWriterWrapper struct {
	w ResponseWriterWrapper
}

func (w responseWriterWrapper) Header() http.Header {
	return w.w.Header()
}

func (w responseWriterWrapper) WriteHeader(statusCode int) {
	w.w.WriteHeader(statusCode)
}

func (w responseWriterWrapper) Write(data []byte) (int, error) {
	return w.w.Write(data)
}

func (w responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.w.Original().(http.Hijacker).Hijack()
}
