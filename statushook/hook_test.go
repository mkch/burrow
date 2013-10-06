package statushook

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

const NotFoundPage = "<html> The gohper is not here!</html>"

func TestHook(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(Handler(mux, HookFunc(func(code int, w http.ResponseWriter, r *http.Request) {
		if code == http.StatusNotFound {
			switch r.URL.Path {
			case "/nothispage1":
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(NotFoundPage))
			case "/nothispage2":
				w.WriteHeader(http.StatusBadRequest)
			}
		}
	})))
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
		t.Fatalf("nothispage1 body %s", err)
	}

	resp, err = http.Get(server.URL + "/nothispage2")
	if err != nil {
		t.Fatalf("nothispage2 %s", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatal("status nothispage2")
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("nothispage2 body read %s", err)
	}
	if len(body) != 0 {
		t.Fatalf("nothispage body %s", err)
	}

	defer server.Close()

}
