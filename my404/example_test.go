package my404_test

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/mkch/burrow/my404"
)

func ExampleHandler() {
	http.HandleFunc("/foo",
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("This is foo."))
		})

	handle404 := func(w io.Writer, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("404 Gopher is not here: %s", r.URL)))
	}
	handler := my404.Handler(http.DefaultServeMux, handle404)
	log.Fatal(http.ListenAndServe("localhost:8181", handler))

	// Please access http://localhost:8181/anything-except-foo in your browser
	// to get the refined 404 page:
	//
	//		404 Gohper is not here: /anything-except-foo
}
