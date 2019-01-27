package compress

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkDefaultEncodingFactory(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var f WriterFactory
		if f = DefaultEncodingFactory.NewWriterFactory(" ,gzip, deflate,	sdch"); f != DefaultGzipWriterFactory || f.ContentEncoding() != "gzip" {
			b.Fatal()
		}
		if f = DefaultEncodingFactory.NewWriterFactory(" a,   deflate"); f != DefaultDeflateWriterFactory || f.ContentEncoding() != "deflate" {
			b.Fatal()
		}
		if f = DefaultEncodingFactory.NewWriterFactory(""); f != nil {
			b.Fatal()
		}
	}
}

var testData = []byte("Package http provides HTTP client and server implementations.")
var server = httptest.NewServer(DefaultHandler(http.HandlerFunc(
	func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, "text/plain")
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
