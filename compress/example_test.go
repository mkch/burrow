package compress_test

import (
	"github.com/mkch/burrow/compress"
	"net/http"
)

func main() {
	ExampleDefaultHandler()
}

func ExampleDefaultHandler() {
	http.ListenAndServe(":8080", compress.DefaultHandler(http.DefaultServeMux))
}
