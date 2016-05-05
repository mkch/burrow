package session_test

import (
	"github.com/mkch/burrow/session"
	"log"
	"net/http"
)

var sessionManager = session.NewSessionManager()

func main() {
	log.Println("Starting web server...")

	http.Handle("/foo", session.HTTPHandlerFunc(fooHandler))

	log.Fatal(http.ListenAndServe(":8080", sessionManager.Handler(http.DefaultServeMux)))
}

func fooHandler(w http.ResponseWriter, r *http.Request, s session.Session) {
	// Access session value with s.
}
