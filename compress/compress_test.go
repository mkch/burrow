package compress

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

func TestDefaultCompressEncodingFactory(t *testing.T) {
	t.Parallel()
	var f WriterFactory
	if f = DefaultEncodingFactory.NewWriterFactory("gzip, deflate,	sdch"); f == nil || f.ContentEncoding() != "gzip" {
		t.Fatal()
	}
	if f = DefaultEncodingFactory.NewWriterFactory(" deflate, gzip"); f == nil || f.ContentEncoding() != "deflate" {
		t.Fatal()
	}
	if f = DefaultEncodingFactory.NewWriterFactory(" x y, gzip "); f == nil || f.ContentEncoding() != "gzip" {
		t.Fatal()
	}
	if f = DefaultEncodingFactory.NewWriterFactory(" x y, gzip,"); f == nil || f.ContentEncoding() != "gzip" {
		t.Fatal()
	}
	if f = DefaultEncodingFactory.NewWriterFactory(" a , b,"); f != nil {
		t.Fatal()
	}
	if f = DefaultEncodingFactory.NewWriterFactory(""); f != nil {
		t.Fatal()
	}
}

var largeString = strings.Repeat("abc", DefaultMinSizeToCompress)

func mustReadAll(t *testing.T, r io.Reader) []byte {
	p, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatalf("Read error: %v", err)
		return nil
	}
	return p
}

func TestResponseWriterDeflateNoCompress(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder() // To gather response.
	w := newResponseWriter(recorder, DefaultMimePolicy, DefaultDeflateWriterFactory, DefaultMinSizeToCompress)
	data := []byte("some text to test.")
	w.Header().Set(contentTypeHeader, "text/plain")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Written len: %v vs %v", n, len(data))
	}
	if err = w.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if enc := recorder.Header().Get(contentEncodingHeader); enc != "" {
		t.Fatalf("Content-Encoding: %#v vs %#v", enc, "")
	}
	if !bytes.Equal(mustReadAll(t, recorder.Body), data) {
		t.Fatal("Body")
	}
}

func TestResponseWriterDeflate(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder() // To gather response.
	w := newResponseWriter(recorder, DefaultMimePolicy, DefaultDeflateWriterFactory, DefaultMinSizeToCompress)
	data := []byte(largeString)
	w.Header().Set(contentTypeHeader, "text/html")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Written len: %v vs %v", n, len(data))
	}
	if err = w.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if enc := recorder.Header().Get(contentEncodingHeader); enc != "deflate" {
		t.Fatalf("Content-Encoding: %#v vs %#v", enc, "deflate")
	}
	if !bytes.Equal(mustReadAll(t, flate.NewReader(recorder.Body)), data) {
		t.Fatal("Body")
	}
}

func TestResponseWriterGzipNoCompress(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder() // To gather response.
	w := newResponseWriter(recorder, DefaultMimePolicy, DefaultGzipWriterFactory, DefaultMinSizeToCompress)
	data := []byte("some text to test.")
	w.Header().Set(contentTypeHeader, "text/plain")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Written len: %v vs %v", n, len(data))
	}
	if err = w.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if enc := recorder.Header().Get(contentEncodingHeader); enc != "" {
		t.Fatalf("Content-Encodeing: %#v vs %#v", enc, "")
	}
	if !bytes.Equal(mustReadAll(t, recorder.Body), data) {
		t.Fatal("Body")
	}
}

func TestResponseWriterGzip(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder() // To gather response.
	w := newResponseWriter(recorder, DefaultMimePolicy, DefaultGzipWriterFactory, DefaultMinSizeToCompress)
	data := []byte(largeString)
	w.Header().Set(contentTypeHeader, "text/html")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Written len: %v vs %v", n, len(data))
	}
	if err = w.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if enc := recorder.Header().Get(contentEncodingHeader); enc != "gzip" {
		t.Fatalf("Content-Encoding: %#v vs %#v", enc, "gzip")
	}
	decompressor, err := gzip.NewReader(recorder.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader error: %v", err)
	}
	if !bytes.Equal(mustReadAll(t, decompressor), data) {
		t.Fatal("Body")
	}
}

func TestCurlGzip(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Log(err)
		return
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			testCurl(t, "gzip")
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestCurlDeflate(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Log(err)
		return
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			testCurl(t, "deflate")
			wg.Done()
		}()
	}
	wg.Wait()
}
func TestCurlNoCompress(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Log(err)
		return
	}
	testCurl(t, "")
}

func testCurl(t *testing.T, encoding string) {
	var art = `<html>

<pre>
		 _____ ____  _     ____  _      _____   _      _____ ____    ____  _____ ____  _     _____ ____    ____  ____  _      ____  ____  _____ ____  ____  _____ ____ 
		/  __//  _ \/ \   /  _ \/ \  /|/  __/  / \  /|/  __//  _ \  / ___\/  __//  __\/ \ |\/  __//  __\  /   _\/  _ \/ \__/|/  __\/  __\/  __// ___\/ ___\/  __//  _ \
		| |  _| / \|| |   | / \|| |\ ||| |  _  | |  |||  \  | | //  |    \|  \  |  \/|| | //|  \  |  \/|  |  /  | / \|| |\/|||  \/||  \/||  \  |    \|    \|  \  | | \|
		| |_//| \_/|| |_/\| |-||| | \||| |_//  | |/\|||  /_ | |_\\  \___ ||  /_ |    /| \// |  /_ |    /  |  \_ | \_/|| |  |||  __/|    /|  /_ \___ |\___ ||  /_ | |_/|
		\____\\____/\____/\_/ \|\_/  \|\____\  \_/  \|\____\\____/  \____/\____\\_/\_\\__/  \____\\_/\_\  \____/\____/\_/  \|\_/   \_/\_\\____\\____/\____/\____\\____/
																																										
																																					
</pre>   

</html>`
	var handler = func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, art); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	svr := httptest.NewServer(NewHandler(http.HandlerFunc(handler), &HandlerConfig{MinSizeToCompress: len(art)}))
	defer svr.Close()

	var args []string
	if encoding != "" {
		args = append(args, "-i", "-H", "Accept-Encoding: "+encoding, "--compressed")
	}
	args = append(args, svr.URL)

	if out, err := exec.Command("curl", args...).Output(); err != nil {
		t.Fatal(err)
	} else {
		output := string(out)
		i := strings.Index(output, art)
		if i < 0 {
			t.Fatal("Body")
		}
		headers := output[:i]
		if encoding != "" {
			if !strings.Contains(headers, "Content-Encoding: "+encoding) {
				t.Fatal("Incorrect Content-Encoding header")
			}
		} else {
			if strings.Contains(headers, "Content-Encoding:") {
				t.Fatal("Content-Encoding header should not be present")
			}
		}
	}
}

func TestHandlerHijacker(t *testing.T) {
	var handler = func(w http.ResponseWriter, r *http.Request) {
		// If the raw ResponseWriter is an http.Hijacker, w must be an http.Hijacker and vice-vase.
		if _, rawIsHijacker := w.(interface{ ResponseWriter() http.ResponseWriter }).ResponseWriter().(http.Hijacker); rawIsHijacker {
			if _, wIsHijacker := w.(http.Hijacker); !wIsHijacker {
				panic("Should be a Hijacker.")
			}
		} else {
			if _, wIsHijacker := w.(http.Hijacker); wIsHijacker {
				panic("Should not be a Hijacker.")
			}
		}
		w.WriteHeader(200)
	}
	svr := httptest.NewServer(NewHandler(http.HandlerFunc(handler), nil))
	defer svr.Close()

	if response, err := http.Get(svr.URL); err != nil {
		t.Fatal(err)
	} else {
		io.Copy(ioutil.Discard, response.Body)
	}
}

func TestResponseWriterHijacker(t *testing.T) {
	var handler = func(raw http.ResponseWriter, r *http.Request) {
		w, _ := NewResponseWriter(raw, DefaultGzipWriterFactory)
		// If the raw ResponseWriter is an http.Hijacker, w must be an http.Hijacker and vice-vase.
		if _, rawIsHijacker := raw.(http.Hijacker); rawIsHijacker {
			if _, wIsHijacker := w.(http.Hijacker); !wIsHijacker {
				panic("Should be a Hijacker.")
			}
		} else {
			if _, wIsHijacker := w.(http.Hijacker); wIsHijacker {
				panic("Should not be a Hijacker.")
			}
		}
		w.WriteHeader(200)
	}
	svr := httptest.NewServer(http.HandlerFunc(handler))
	defer svr.Close()

	if response, err := http.Get(svr.URL); err != nil {
		t.Fatal(err)
	} else {
		io.Copy(ioutil.Discard, response.Body)
	}
}

func TestNewResponseWriter(t *testing.T) {
	writerFactory := DefaultDeflateWriterFactory
	recorder := httptest.NewRecorder()
	if w, err := NewResponseWriter(recorder, writerFactory); err != nil {
		t.Fatal(err)
	} else {
		data := []byte("abc def")
		if n, err := w.Write(data); err != nil {
			t.Fatal(err)
		} else if n != len(data) {
			t.Fatal("len")
		} else {
			w.Close()
			if recorder.Header().Get(contentEncodingHeader) != writerFactory.ContentEncoding() {
				t.Fatal(contentEncodingHeader)
			}
			recv := mustReadAll(t, flate.NewReader(recorder.Body))
			if !bytes.Equal(recv, data) {
				t.Fatal("Body")
			}
		}
	}

}
