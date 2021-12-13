package internal_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"

	"github.com/mkch/burrow/internal"
)

func ExampleHijackResponseWriter() {
	var w http.ResponseWriter = &writer{}
	var hw http.ResponseWriter = &hijackWriter{}

	var writer = &MyResponseWriter{w, 1}
	writer.WriteHeader(200)
	writer.Write([]byte{1, 2})

	writer = &MyResponseWriter{hw, 2}
	var wrapper2 http.ResponseWriter = &internal.HijackResponseWriter{writer, writer.ResponseWriter.(http.Hijacker)}
	wrapper2.WriteHeader(404)
	wrapper2.Write([]byte{3, 4})
	wrapper2.(http.Hijacker).Hijack()

	writer = &MyResponseWriter{wrapper2, 3}
	var wrapper3 http.ResponseWriter = &internal.HijackResponseWriter{writer, writer.ResponseWriter.(http.Hijacker)}
	wrapper3.WriteHeader(500)
	wrapper3.Write([]byte{5, 6})
	wrapper3.(http.Hijacker).Hijack()

	// Output:
	// writer.WriteHeader
	// writer.Write [1 2]
	//   From MyResponseWriter #1
	// writer.WriteHeader
	// writer.Write [3 4]
	//   From MyResponseWriter #2
	// hijackWriter.Hijack
	// writer.WriteHeader
	// writer.Write [5 6]
	//   From MyResponseWriter #2
	//   From MyResponseWriter #3
	// hijackWriter.Hijack
}

// writer is a homemade http.ResponseWriter.
type writer struct{}

func (w *writer) WriteHeader(status int) {
	fmt.Println("writer.WriteHeader")
}

func (w *writer) Write(b []byte) (int, error) {
	fmt.Printf("writer.Write %v\n", b)
	return len(b), nil
}

func (w *writer) Header() http.Header {
	return nil
}

// hijackWriter is a homemade http.ResponseWriter and http.Hijacker.
type hijackWriter struct {
	writer
}

func (w *hijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	fmt.Println("hijackWriter.Hijack")
	return nil, nil, nil
}

// MyResponseWriter is a wrapper of http.ResponseWriter.
type MyResponseWriter struct {
	http.ResponseWriter
	id int
}

func (w *MyResponseWriter) Write(b []byte) (n int, err error) {
	// Call the original write.
	n, err = w.ResponseWriter.Write(b)
	// Do some extra stuff.
	fmt.Printf("  From MyResponseWriter #%v\n", w.id)
	return
}
