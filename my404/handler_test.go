package my404_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkch/burrow/my404"
)

const NotFoundPage = "<html> The gopher is not here!</html>"
const NotFoundPage2 = "<html> The gopher is not here 2!</html>"

func TestHandler(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(my404.Handler(mux, func(w io.Writer, r *http.Request) {
		switch r.URL.Path {
		case "/nothispage1":
			w.Write([]byte(NotFoundPage))
		case "/nothispage2":
			w.Write([]byte(NotFoundPage2))
		}
	}))
	mux.HandleFunc("/foo", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("foo"))
	})

	resp, err := http.Get(server.URL + "/foo")
	if err != nil {
		t.Fatalf("foo %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status foo")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("foo body read %s", err)
	}
	if string(body) != "foo" {
		t.Fatalf("foo body %s", err)
	}

	resp, err = http.Get(server.URL + "/nothispage1")
	if err != nil {
		t.Fatalf("nothispage1 %s", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status nothispage1")
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("nothispage1 body read %s", err)
	}
	if string(body) != NotFoundPage {
		t.Fatalf("nothispage1 body %s", body)
	}

	resp, err = http.Get(server.URL + "/nothispage2")
	if err != nil {
		t.Fatalf("nothispage2 %s", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("status nothispage2")
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("nothispage2 body read %s", err)
	}
	if string(body) != NotFoundPage2 {
		t.Fatalf("nothispage2 body %s", body)
	}

	defer server.Close()

}
