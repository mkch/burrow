package compress_test

import (
	"net/http"

	"github.com/mkch/burrow/compress"
)

func main() {
	ExampleNewHandler()
}

func ExampleNewHandler() {
	http.ListenAndServe(":8080", compress.NewHandler(http.DefaultServeMux, nil))
}
