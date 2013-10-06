package compress

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultCompressEncodingFactory(t *testing.T) {
	t.Parallel()
	var (
		f   WriterFactory
		enc string
	)
	if f, enc = DefaultEncodingFactory.NewWriterFactory("gzip, deflate,	sdch"); f == nil || enc != "gzip" {
		t.Fatal()
	}
	if f, enc = DefaultEncodingFactory.NewWriterFactory(" deflate, gzip"); f == nil || enc != "deflate" {
		t.Fatal()
	}
	if f, enc = DefaultEncodingFactory.NewWriterFactory(" x y, gzip "); f == nil || enc != "gzip" {
		t.Fatal()
	}
	if f, enc = DefaultEncodingFactory.NewWriterFactory(" x y, gzip,"); f == nil || enc != "gzip" {
		t.Fatal()
	}
	if f, enc = DefaultEncodingFactory.NewWriterFactory(" a , b,"); f != nil || enc != "" {
		t.Fatal()
	}
	if f, enc = DefaultEncodingFactory.NewWriterFactory(""); f != nil || enc != "" {
		t.Fatal()
	}
}

func TestResponseWriter(t *testing.T) {
	t.Parallel()
	/// flate
	test := &test{
		writerFactory:   DefaultDeflateWriterFactory,
		mimePolicy:      DefaultMimePolicy,
		contentEncoding: "deflate",
		newDecompressor: func(r io.Reader) io.ReadCloser { return flate.NewReader(r) },
		data:            []byte("some text to test."),
		contentType:     "text/plain",
	}
	testResponseWriter(t, test)

	test.contentEncoding = "x-known"
	testResponseWriter(t, test)

	test.data = []byte("<html>")
	test.contentType = ""
	testResponseWriter(t, test)

	test.data = nil
	test.contentType = ""
	testResponseWriter(t, test)

	/// gzip
	test.writerFactory = DefaultGzipWriterFactory
	test.contentEncoding = "gzip"
	test.newDecompressor = func(r io.Reader) io.ReadCloser { reader, _ := gzip.NewReader(r); return reader }
	testResponseWriter(t, test)

	test.contentEncoding = "x-known"
	testResponseWriter(t, test)

	test.data = []byte("<html>")
	test.contentType = ""
	testResponseWriter(t, test)

	test.data = nil
	test.contentType = ""
	testResponseWriter(t, test)

}

type test struct {
	writerFactory   WriterFactory
	mimePolicy      MimePolicy
	contentEncoding string
	newDecompressor func(io.Reader) io.ReadCloser
	data            []byte
	contentType     string
}

type NoClose struct {
	io.Reader
}

func (n *NoClose) Close() error {
	return nil
}

func testResponseWriter(t *testing.T, test *test) {
	recorder := httptest.NewRecorder() // To gather response.
	w := newResponseWriter(recorder, test.mimePolicy, test.writerFactory, test.contentEncoding)
	// Write
	w.Header().Set(ContentTypeHeader, test.contentType)
	n, err := w.Write(test.data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(test.data) {
		t.Fatalf("[%v]: %v vs. %v", "Written len", n, len(test.data))
	}
	err = w.Close()
	if err != nil {
		t.Fatal(err)
	}

	//   Test Content-Type
	contentType := test.contentType
	if contentType == "" && len(test.data) > 0 {
		contentType = http.DetectContentType(test.data)
	}
	recvContentType := recorder.Header().Get(ContentTypeHeader)
	if recvContentType != contentType {
		t.Fatalf(`[%s]: "%s" vs. \"%s" with data %v`, ContentTypeHeader, recvContentType, contentType, test.data)
	}
	compress := test.mimePolicy.AllowCompress(contentType) && len(test.data) >= MinSizeToCompress
	//   Test Content-Encoding
	recvContentEncoding := recorder.Header().Get(ContentEncodingHeader)
	contentEncoding := ""
	if compress {
		contentEncoding = test.contentEncoding
	}
	if recvContentEncoding != contentEncoding {
		t.Fatalf(`[%s]: "%s"" vs. "%s"`, ContentEncodingHeader, recvContentEncoding, contentEncoding)
	}
	//   Test body
	var reader io.ReadCloser
	if compress {
		reader = test.newDecompressor(recorder.Body)
	} else {
		reader = &NoClose{recorder.Body}
	}
	defer reader.Close()
	read, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, test.data) {
		t.Fatal("Body")
	}
}
