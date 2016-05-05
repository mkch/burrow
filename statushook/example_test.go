package statushook_test

import (
	"fmt"
	"github.com/mkch/burrow/statushook"
	"log"
	"net/http"
)

func ExampleHandler() {
	http.HandleFunc("/foo",
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("This is foo."))
		})

	refined404Hook := func(code int, w http.ResponseWriter, r *http.Request) {
		if code == http.StatusNotFound {
			w.Write([]byte(fmt.Sprintf("404 Gohper is not here: %s", r.URL)))
		}
	}
	handler := statushook.Handler(http.DefaultServeMux, statushook.HookFunc(refined404Hook))
	log.Fatal(http.ListenAndServe("localhost:8181", handler))

	// Please access http://localhost:8181/anything-except-foo in your browser
	// to get the refined 404 page:
	//
	//		404 Gohper is not here: /anything-except-foo
}
