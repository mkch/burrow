package my404

import (
	"io"
	"net/http"

	"github.com/mkch/burrow/internal"
)

type responseWriter struct {
	http.ResponseWriter
	request *http.Request
	handler func(io.Writer, *http.Request)
	status  int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if w.status == 0 {
		w.ResponseWriter.WriteHeader(statusCode)
		w.status = statusCode
	}
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.status == http.StatusNotFound {
		w.handler(w.ResponseWriter, w.request)
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) Original() http.ResponseWriter {
	return w.ResponseWriter
}

// Handler returns a http.Handler which calls handle404 instead of w.Write to write the response body after
// a 404 status code was written to w.
func Handler(h http.Handler, handle404 func(w io.Writer, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(
			internal.WrapResponseWriter(&responseWriter{ResponseWriter: w, request: r, handler: handle404}),
			r)
	})
}
