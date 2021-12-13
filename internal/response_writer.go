package internal

import (
	"bufio"
	"net"
	"net/http"
)

// HijackResponseWriter implements both http.ResponseWriter and http.Hijacker.
type HijackResponseWriter struct {
	http.ResponseWriter
	http.Hijacker
}

func (w HijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.Hijacker.Hijack()
}
