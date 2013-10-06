package spdy_test

import (
	"crypto/tls"
	"github.com/kevin-yuan/burrow/spdy"
	"log"
	"net/http"
)

func main() {
	ExampleTLSNextProtoFuncV2()
}

func ExampleTLSNextProtoFuncV2() {
	server := &http.Server{
		Addr: ":8080",
		TLSConfig: &tls.Config{
			NextProtos: []string{"spdy/2"},
		},
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){
			"spdy/2": spdy.TLSNextProtoFuncV2,
		},
	}

	http.HandleFunc("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("Hello, " + req.URL.String()))
		if spdy.Spdy(req) {
			w.Write([]byte(" from SPDY!"))
		}
	})

	err := server.ListenAndServeTLS("/path/to/host.crt", "/path/to/host.key")
	if err != nil {
		log.Fatal(err)
	}
}
