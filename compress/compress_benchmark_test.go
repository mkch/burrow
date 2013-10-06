package compress

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkDefaultEncodingFactory(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var (
			f   WriterFactory
			enc string
		)
		if f, enc = DefaultEncodingFactory.NewWriterFactory(" ,gzip, deflate,	sdch"); /*f != DefaultGzipWriterFactory ||*/ enc != "gzip" {
			b.Fatal()
		}
		if f, enc = DefaultEncodingFactory.NewWriterFactory(" a,   deflate"); /*f != DefaultDeflateWriterFactory ||*/ enc != "deflate" {
			b.Fatal()
		}
		if f, enc = DefaultEncodingFactory.NewWriterFactory(""); f != nil || enc != "" {
			b.Fatal()
		}
	}
}

var testData = []byte("Package http provides HTTP client and server implementations.")
var server = httptest.NewServer(DefaultHandler(http.HandlerFunc(
	func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(ContentTypeHeader, "text/plain")
		w.Write(testData)
	})))

func BenchmarkResponseWriter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		response, err := http.Get(server.URL)
		if err != nil {
			b.Fatal(err)
		}
		response.Body.Close()
	}
}
