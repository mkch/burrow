package spdy

import (
	"github.com/mkch/burrow/spdy/framing"
	"net/http"
	"net/url"
	"sync"
)

var lockNextServerStreamID sync.Mutex
var nextServerStreamID uint32

func newServerStreamID() uint32 {
	lockNextServerStreamID.Lock()
	defer lockNextServerStreamID.Unlock()
	nextServerStreamID += 2
	return nextServerStreamID
}

type missingHeader string

func (e missingHeader) Error() string {
	return "Missing " + string(e) + " Header"
}

type duplicatedHeader string

func (e duplicatedHeader) Error() string {
	return "Duplicated " + string(e) + " Header"
}

type invalidHeader struct {
	Header string
	Err    error
}

func (e *invalidHeader) Error() string {
	return "Invalid " + e.Header + " Header"
}

func httpRequest(version uint16, stream *stream) (*http.Request, error) {
	switch version {
	case 2:
		return httpRequestV2(stream)
	case 3:
		return httpRequestV3(stream)
	default:
		return nil, framing.ErrUnsupportedVersion
	}
}

type responseWriter interface {
	http.ResponseWriter
	Close() error
}

type ResponseWriter interface {
	http.ResponseWriter
	// Push initiates an "SPDY Serve Push".
	// The server response to GET request of url will be pushed to user-agent.
	// originalRequest is the original request of the ResponseWriter.
	// The Scheme and Host fields of url can be empty to use the scheme and host
	// of the original request.
	Push(url *url.URL, originalRequest *http.Request) error
}

func newResponseWriter(version uint16, stream *stream, c *conn, ctrlFrame framing.ControlFrameWithHeaders) (responseWriter, error) {
	switch version {
	case 2:
		return newResponseWriterV2(stream, c, ctrlFrame), nil
	case 3:
		return newResponseWriterV3(stream, c, ctrlFrame), nil
	default:
		return nil, framing.ErrUnsupportedVersion
	}
}

const MAX_DATA_LEN int = 10240

// newServerPushSynStream creates a SynFrame for server push stream.
// The stream ID of the returned frame is streamID and is associated
// to stream associated. r is the request whose response will be pused.
// If the r.Scheme or r.Host is empty, the values gotten from header of associated
// will be used.
func newServerPushSynStream(version uint16, streamID uint32, associated *stream, r *http.Request) (f framing.SynStream, err error) {
	switch version {
	case 2:
		return newServerPushSynStreamV2(streamID, associated, r)
	case 3:
		return newServerPushSynStreamV3(streamID, associated, r)
	default:
		return nil, framing.ErrUnsupportedVersion
	}
}

// Push pushes the response of the rquest with url to client.
func serverPush(c *conn, associated *stream, url *url.URL, originalRequest *http.Request) error {
	if url.Scheme == "" {
		url.Scheme = originalRequest.URL.Scheme
	}
	if url.Host == "" {
		url.Host = originalRequest.URL.Host
	}
	r := &http.Request{
		Method:     "GET", // "The server MUST only push resources which would have been returned from a GET request."
		URL:        url,
		Proto:      originalRequest.Proto,
		ProtoMajor: originalRequest.ProtoMajor,
		ProtoMinor: originalRequest.ProtoMinor,
		Header:     originalRequest.Header,
		Host:       originalRequest.Host,
	}
	return c.push(associated, associated.Priority, r)
}

func Spdy(req *http.Request) bool {
	return req.Header.Get("x-spdy") == "true"
}
