/*
Package statushook provides a utility to hook a http.Handler before the status
code is written, giving the hook a chance to midify the response header, or
replace the entire response.

A typical use case is to hook a serve handler to provide global refined 404 page:

Given a simple "foo" server:
	http.HandleFunc("/foo",
			func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("This is foo."))
			})
	log.Fatal(http.ListenAndServe("localhost:8181", handler))

We hook it liek this:
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

Then we will get the refined 404 page if we request http://localhost:8181/anything-except-foo:
	404 Gohper is not here: /anything-except-foo
*/
package statushook
