package spdy

import (
	"bytes"
	"errors"
	"github.com/mkch/burrow/spdy/framing"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func httpRequestV2(stream *stream) (*http.Request, error) {
	var err error
	host := stream.Headers.Get("host")
	if len(host) == 0 {
		return nil, missingHeader("host")
	} else if len(host) != 1 {
		return nil, duplicatedHeader("host")
	}
	method := stream.Headers.Get("method")
	if len(method) == 0 {
		return nil, missingHeader("method")
	} else if len(method) != 1 {
		return nil, duplicatedHeader("method")
	}
	scheme := stream.Headers.Get("scheme")
	if len(scheme) == 0 {
		return nil, missingHeader("scheme")
	} else if len(scheme) != 1 {
		return nil, duplicatedHeader("scheme")
	}
	urlHeaders := stream.Headers.Get("url")
	if len(urlHeaders) == 0 {
		return nil, missingHeader("url")
	} else if len(urlHeaders) != 1 {
		return nil, duplicatedHeader("url")
	}
	var requestUrl *url.URL
	if requestUrl, err = url.ParseRequestURI(urlHeaders[0]); err != nil {
		return nil, &invalidHeader{"url", err}
	}
	protocol := stream.Headers.Get("version")
	if len(protocol) == 0 {
		return nil, missingHeader("protocol")
	} else if len(protocol) != 1 {
		return nil, duplicatedHeader("Protocol")
	}
	var protoMajor, protoMinor int
	if protoVer := strings.Split(protocol[0], "/"); len(protoVer) != 2 {
		return nil, &invalidHeader{"protocol", errors.New("Invalid protocol format")}
	} else if ver := strings.Split(protoVer[1], "."); len(ver) != 2 {
		return nil, &invalidHeader{"protocol", errors.New("Invalid protocol version")}
	} else if protoMajor, err = strconv.Atoi(ver[0]); err != nil {
		return nil, &invalidHeader{"protocol", err}
	} else if protoMinor, err = strconv.Atoi(ver[1]); err != nil {
		return nil, &invalidHeader{"protocol", err}
	}

	req := &http.Request{
		Method:     method[0],
		Header:     make(http.Header),
		URL:        requestUrl,
		Proto:      protocol[0],
		ProtoMajor: protoMajor,
		ProtoMinor: protoMinor,
		// ContentLength records the length of the associated content.
		// The value -1 indicates that the length is unknown.
		// Values >= 0 indicate that the given number of bytes may
		// be read from Body.
		// For outgoing requests, a value of 0 means unknown if Body is not nil.
		ContentLength: -1,
		Host:          host[0],
	}

	if stream.Reader != nil {
		req.Body = stream.Reader.reader
	}

	for _, name := range stream.Headers.Names() {
		switch name {
		case "method", "scheme", "url", "version", "protocol":
			continue
		}
		for _, value := range stream.Headers.Get(name) {
			req.Header.Add(name, value)
		}
	}
	req.Header.Add("x-spdy", "true")
	return req, nil
}

func newServerPushSynStreamV2(streamID uint32, associated *stream, r *http.Request) (f framing.SynStream, err error) {
	if f, err = framing.NewSynStream(2, streamID, framing.FLAG_UNIDIRECTIONAL); err != nil {
		return
	}
	f.SetAssociatedToStreamID(associated.ID)
	headers := f.Headers()
	url := *r.URL
	if url.Scheme == "" {
		url.Scheme = associated.Headers.GetFirst("scheme")
	}
	if url.Host == "" {
		url.Host = associated.Headers.GetFirst("host")
	}
	url.Path = r.URL.Path
	url.RawQuery = r.URL.RawQuery
	headers.Add("url", url.String())
	log.Println(headers)
	return
}

type responseWriterV2 struct {
	stream            *stream
	conn              *conn
	header            http.Header
	ctrlFrame         framing.ControlFrameWithHeaders
	writeHeaderCalled bool // WriteHeader() method called or not.
	ctrlFrameWritten  bool // ctrlFrame frame written or not.
	buf               bytes.Buffer
	contentLen        int // The "Content-Length" header value. 0 if not available.
	writtenLen        int // How many bytes has written as data frame(response body).
}

func newResponseWriterV2(stream *stream, c *conn, ctrlFrame framing.ControlFrameWithHeaders) *responseWriterV2 {
	return &responseWriterV2{
		stream:    stream,
		conn:      c,
		header:    make(http.Header),
		ctrlFrame: ctrlFrame,
	}
}

func (w *responseWriterV2) Header() http.Header {
	return w.header
}

func (w *responseWriterV2) Write(p []byte) (int, error) {
	if !w.writeHeaderCalled {
		w.WriteHeader(http.StatusOK)
	}
	if !w.ctrlFrameWritten {
		w.conn.writeFrame(w.ctrlFrame, w.stream.Priority)
		w.ctrlFrameWritten = true
	}
	var lenP = len(p)
	for l := lenP; l > 0; l = len(p) {
		avai := MAX_DATA_LEN - w.buf.Len()
		if l < avai {
			w.buf.Write(p)
			break
		} else {
			if n, err := w.buf.Write(p[:avai]); err != nil {
				return n, err
			}
			if err := w.writeBufFrame(false); err != nil {
				return lenP - len(p), err
			}
			w.buf.Reset()
			p = p[avai:]
		}
	}
	return lenP, nil
}

func (w *responseWriterV2) Close() error {
	if !w.ctrlFrameWritten { // No response body at all.
		if flags, ok := w.ctrlFrame.(framing.ControlFrameWithSetFlags); ok {
			flags.SetFlags(framing.FLAG_FIN)
		} else {
			log.Printf("Server push stream #%v has no response body", w.stream.ID)
			return nil
		}
		w.conn.writeFrame(w.ctrlFrame, w.stream.Priority)
		w.ctrlFrameWritten = true
	} else if w.contentLen == 0 || // Content-Length is not available
		w.buf.Len() > 0 { // Buffer is not empty
		w.writeBufFrame(true)
	}
	return nil
}

func (w *responseWriterV2) writeBufFrame(fin bool) error {
	bufLen := w.buf.Len()
	if bufLen == 0 {
		log.Printf("SPDY send empty data frame with FLAG_FIN on stream #%v\n", w.stream.ID)
	}

	f := new(framing.DataFrame)
	f.SetStreamID(w.stream.ID)
	f.SetLen(uint32(bufLen))
	var writtenLen = w.writtenLen + bufLen
	var forceFin bool
	if w.contentLen != 0 {
		if writtenLen > w.contentLen {
			log.Printf("Stream #%v Content-Length mismatch!", w.stream.ID)
			w.buf.Reset()
			w.conn.writeRstStream(w.stream, framing.STATUS_INTERNAL_ERROR)
			return errors.New("Content-Length mismatch")
		}
		forceFin = writtenLen == w.contentLen
	}
	if fin || forceFin {
		f.SetFlags(framing.FLAG_FIN)
	}
	// Use append() to clone w.buf.Bytes().
	f.Reader = bytes.NewReader(append([]byte(nil), w.buf.Bytes()...))
	w.conn.writeFrame(f, w.stream.Priority)
	w.writtenLen = writtenLen
	return nil
}

// Just store the header, not sending.
func (w *responseWriterV2) WriteHeader(statusCode int) {
	if w.writeHeaderCalled {
		return
	}
	headers := w.ctrlFrame.Headers()
	headers.Add("status", strconv.Itoa(statusCode))
	headers.Add("version", "HTTP/1.1")
	for name, values := range w.header {
		name = strings.ToLower(name)
		switch name {
		case "connection", "keep-alive", "transfer-encoding":
			continue
		case "content-length":
			if len(values) > 0 {
				if l, err := strconv.Atoi(values[0]); err == nil {
					w.contentLen = l
				}
			}
		}
		for _, value := range values {
			headers.Add(name, value)
		}
	}
	w.writeHeaderCalled = true
}

// Push pushes the response of the rquest with url to client.
func (w *responseWriterV2) Push(url *url.URL, originalRequest *http.Request) error {
	return serverPush(w.conn, w.stream, url, originalRequest)
}
