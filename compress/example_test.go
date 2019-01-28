package compress_test

import (
	"io"
	"net/http"

	"github.com/mkch/burrow/compress"
)

func ExampleNewHandler() {
	http.ListenAndServe(":8080", compress.NewHandler(http.DefaultServeMux, nil))
}

func ExampleNewResponseWriter() {
	handler := func(w http.ResponseWriter, r *http.Request) {
		cw, _ := compress.NewResponseWriter(w, compress.DefaultGzipWriterFactory)
		io.WriteString(cw, "content to write")
	}
	http.ListenAndServe(":8080", http.HandlerFunc(handler))
}
