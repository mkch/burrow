package burrow

import "net/http"

func ExampleDir() {
	http.FileServer(&Dir{Dir: http.Dir("some/dir")})
}
