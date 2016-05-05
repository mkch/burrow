Burrow for gophers.
======
Toolkit for go net/http.

## Install
	go get github.com/mkch/burrow
	go install github.com/mkch/burrow
## Examples
### * Compress
	package compress_test
	
	import (
	    "github.com/mkch/burrow/compress"
	    "net/http"
	)
	
	func main() {
	    http.ListenAndServe(":8080", compress.DefaultHandler(http.DefaultServeMux))
	}
### * Session
	package session_test
	
	import (
		"github.com/mkch/burrow/session"
		"net/http"
	)
	
	var sessionManager = session.NewSessionManager()
	
	func main() {
		http.Handle("/foo", session.HTTPHandlerFunc(fooHandler))
		http.ListenAndServe(":8080", sessionManager.Handler(http.DefaultServeMux))
	}
	
	func fooHandler(w http.ResponseWriter, r *http.Request, s session.Session) {
		// Access session value with s.
	}
### * Status Hook
	package statushook_test
	
	import (
		"github.com/mkch/burrow/statushook"
		"net/http"
	)
	
	func main() {
		http.HandleFunc("/foo",
			func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("This is foo."))
			})
	
		refined404Hook := func(code int, w http.ResponseWriter, r *http.Request) {
			if code == http.StatusNotFound {
				w.Write([]byte("404 Gohper is not here: "+r.URL.String()))
			}
		}
		handler := statushook.Handler(http.DefaultServeMux, statushook.HookFunc(refined404Hook))
		http.ListenAndServe("localhost:8181", handler)
	
		// Please access http://localhost:8181/anything-except-foo in your browser
		// to get the refined 404 page:
		//
		//		404 Gohper is not here: /anything-except-foo
	}
### * Google SPDY™
	package spdy_test
	
	import (
		"crypto/tls"
		"github.com/mkch/burrow/spdy"
		"net/http"
	)
	
	func main() {
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
	
		server.ListenAndServeTLS("/path/to/host.crt", "/path/to/host.key")
	}