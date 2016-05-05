package spdy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/mkch/burrow/spdy/framing"
	"github.com/mkch/burrow/spdy/framing/fields"
	"github.com/mkch/burrow/spdy/util"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
)

const maxFramePriority byte = 0xFF

func TLSNextProtoFuncV2(server *http.Server, tlsConn *tls.Conn, handler http.Handler) {
	(&conn{Version: 2, Server: server, Conn: tlsConn, Handler: handler}).Serve()
}

func TLSNextProtoFuncV3(server *http.Server, tlsConn *tls.Conn, handler http.Handler) {
	(&conn{Version: 3, Server: server, Conn: tlsConn, Handler: handler}).Serve()
}

var errGoAway = errors.New("GoAway")

type badFrame string

func (e badFrame) Error() string {
	return "Bad frame: " + string(e)
}

type pipe struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newPipe() *pipe {
	var p pipe
	p.reader, p.writer = io.Pipe()
	return &p
}

type stream struct {
	ID             uint32 // ID of this stream.
	Priority       byte
	Headers        framing.HeaderBlock // Headers of SynFrame if ingoing, nil for outgoing.
	mtxClosed      sync.RWMutex
	peerHalfClosed bool  // The remote end has half closed.
	halfClosed     bool  // Half closed.
	Reader         *pipe // Reader.reader can be used to read the request if ingoing.
	//sendFCW        *util.FlowCtrlWin
}

func (s *stream) TakePrecedenceOver(other util.PriorityItem) bool {
	otherStream := other.(*stream)
	if s.Priority == otherStream.Priority {
		return s.ID < otherStream.ID
	}
	return s.Priority > otherStream.Priority
}

func (s *stream) PeerHalfClosed() bool {
	s.mtxClosed.RLock()
	defer s.mtxClosed.RUnlock()
	return s.peerHalfClosed
}

func (s *stream) HalfClosed() bool {
	s.mtxClosed.RLock()
	defer s.mtxClosed.RUnlock()
	return s.halfClosed
}

func (s *stream) PeerHalfClose(c *conn) {
	s.mtxClosed.Lock()
	defer s.mtxClosed.Unlock()
	if s.peerHalfClosed {
		return
	}
	s.peerHalfClosed = true
	if s.halfClosed {
		c.deleteStream(s.ID)
	}
}

func (s *stream) HalfClose(c *conn) {
	s.mtxClosed.Lock()
	defer s.mtxClosed.Unlock()
	if s.halfClosed {
		return
	}
	s.halfClosed = true
	if s.peerHalfClosed {
		c.deleteStream(s.ID)
	}
}

type conn struct {
	Version uint16
	// Frome http.Server.TLSNextProto func.
	Server  *http.Server
	Conn    *tls.Conn
	Handler http.Handler

	r              *bufio.Reader
	w              *bufio.Writer
	mtxLiveStreams sync.RWMutex
	liveStreams    map[uint32]*stream
	decoder        *fields.Decoder
	encoderr       *fields.Encoder
	exit           chan bool

	streamQ          *util.BlockingPriorityQueue
	lastGoodStreamID uint32

	framesToWrite *util.BlockingPriorityQueue

	// sort.Sort is not stable, we need an sequence number.
	// This lock protects the following seq.
	lSeq          sync.Mutex
	frameWriteSeq uint32

	initWindowSize uint32
}

const recvFrameBufSize = 100
const sendFrameBufSize = 100

func (c *conn) Serve() {
	c.r = bufio.NewReader(c.Conn)
	c.w = bufio.NewWriter(c.Conn)
	c.liveStreams = make(map[uint32]*stream)
	c.decoder = fields.NewDecoder(c.r)
	var dict []byte
	var err error
	if dict, err = selectDict(c.Version); err != nil {
		return
	}
	c.decoder.SetZlibDict(dict)
	c.encoderr = fields.NewEncoder(c.w)
	c.exit = make(chan bool)
	c.streamQ = util.NewBlockingPriorityQueue(recvFrameBufSize)
	c.framesToWrite = util.NewBlockingPriorityQueue(sendFrameBufSize)

	log.Printf("SPDY connection created. Remote Addr: %v\n", c.Conn.RemoteAddr())

	go c.writeLoop()
	go c.readLoop()
	go c.serveLoop()
	for i := 0; i < 3; i++ {
		<-c.exit
	}
	log.Printf("SPDY connection closed. Remote Addr: %v\n", c.Conn.RemoteAddr())
}

func (c *conn) getStream(streamID uint32) *stream {
	c.mtxLiveStreams.RLock()
	defer c.mtxLiveStreams.RUnlock()
	return c.liveStreams[streamID]
}

func (c *conn) addStream(stream *stream) {
	c.mtxLiveStreams.Lock()
	defer c.mtxLiveStreams.Unlock()
	c.liveStreams[stream.ID] = stream
}

func (c *conn) deleteStream(streamID uint32) {
	c.mtxLiveStreams.Lock()
	defer c.mtxLiveStreams.Unlock()
	delete(c.liveStreams, streamID)
}

func (c *conn) nextFrameWriteSeq() (seq uint32) {
	c.lSeq.Lock()
	defer c.lSeq.Unlock()
	c.frameWriteSeq++
	return c.frameWriteSeq
}

func (c *conn) readLoop() {
	var err error
	for {
		var f framing.Frame
		f, err = framing.ReadFrame(c.decoder)
		if err != nil {
			break
		}
		if f.IsControl() {
			err = c.readControlFrame(f.(framing.ControlFrame))
		} else {
			err = c.readDataFrame(f.(*framing.DataFrame))
		}
		if err != nil {
			break
		}
	}
	if err != nil {
		if _, networkErr := err.(net.Error); err != errGoAway && err != io.EOF && !networkErr {
			log.Printf("SPDY read protocol error: %v\n", err)
			var (
				goAway framing.GoAway
				err    error
			)
			if goAway, err = framing.NewGoAway(c.Version, c.lastGoodStreamID); err != nil {
				log.Panicf("SPDY create frame error: %v\n", err)
			} else if setStatusCode, ok := goAway.(framing.ControlFrameWithSetStatusCode); ok {
				setStatusCode.SetStatusCode(framing.STATUS_GOAWAY_PROTOCOL_ERROR)
			}
			c.writeFrame(goAway, maxFramePriority)
		} else {
			log.Printf("SPDY read network error: %v\n", err)
		}
	}
	c.framesToWrite.Push(&frameWithPriority{Frame: nil})
	c.streamQ.Push(nil)
	c.exit <- true
}

func (c *conn) readControlFrame(f framing.ControlFrame) error {
	switch f.Type() {
	case framing.FRAME_SYN_STREAM:
		frame := f.(framing.SynStream)
		streamID := frame.StreamID()
		// 0 is not a valid Stream-ID.
		// If the client is initiating the stream, the Stream-ID must be odd.
		// Stream-IDs from each side of the connection must increase monotonically.
		if streamID == 0 || streamID%2 == 0 || streamID < c.lastGoodStreamID {
			c.writeRstStreamID(streamID, framing.STATUS_PROTOCOL_ERROR)
			break
		}
		c.lastGoodStreamID = streamID
		if stream := c.getStream(streamID); stream != nil {
			c.writeRstStream(stream, framing.StatusCodeStreamInUse(c.Version))
			break
		}
		flags := frame.Flags()
		var reader *pipe
		if flags != framing.FLAG_FIN {
			reader = newPipe()
		}
		stream := &stream{
			ID:             streamID,
			Priority:       frame.Priority(),
			Headers:        frame.Headers(),
			peerHalfClosed: flags == framing.FLAG_FIN,
			halfClosed:     flags == framing.FLAG_UNIDIRECTIONAL,
			Reader:         reader,
			//sendFCW:        util.NewFlowCtrlWin(),
		}
		c.addStream(stream)
		c.streamQ.Push(stream)
	case framing.FRAME_RST_STREAM:
		frame := f.(framing.RstStream)
		streamID := frame.StreamID()
		log.Printf("SPDY stream #%v reset due to %v\n", streamID, frame.StatusCode())
		stream := c.getStream(streamID)
		if stream == nil {
			break
		}
		c.closeStream(stream)
	case framing.FRAME_PING:
		// PONG
		c.writeFrame(f, maxFramePriority)
	case framing.FRAME_SETTINGS:
		frame := f.(framing.Settings)
		log.Printf("SETTINGS: %v\n", frame)
		//if _, value, exists := frame.Entries().Get(framing.ID_SETTINGS_INITIAL_WINDOW_SIZE); exists {
		//		if value < 1 || value > framing.MAX_DELTA_WINDOW_SIZE {
		//			return framing.ErrInvalidDeltaWindowSize
		//		}
		//		if c.initWindowSize != 0 {
		//			return errors.New("Multiple ID_SETTINGS_INITIAL_WINDOW_SIZE on this connection")
		//		}
		//		c.initWindowSize = value
		//	func() {
		//		c.mtxLiveStreams.RLock()
		//		defer c.mtxLiveStreams.RUnlock()
		//
		//		for _, stream := range c.liveStreams {
		//			err := func() error {
		//				stream.sendFCW.L.Lock()
		//				defer stream.sendFCW.L.Unlock()
		//				return stream.sendFCW.InitSize(c.initWindowSize)
		//			}()
		//			if err != nil {
		//				return err
		//			}
		//		}
		//	}()
		//}
	case framing.FRAME_NOOP:
		if c.Version != 2 {
			return badFrame("FRAME_NOOP")
		}
	case framing.FRAME_WINDOW_UPDATE:
		if c.Version < 3 {
			return badFrame("WINDOW_UPDATE")
		}
		log.Printf("++++++++++WINDOW_UPDATE %v++\n", f)
		log.Panic("FRAME_WINDOW_UPDATE must be processed")
		//frame := f.(framing.WindowUpdate)
		//stream := c.getStream(frame.StreamID())
		//if stream == nil {
		//	c.writeRstStream(stream, framing.StatusCodeStreamAlreadyClosed(c.Version))
		//	return nil
		//}
		//stream.sendFCW.L.Lock()
		//defer stream.sendFCW.L.Unlock()
		//if err := stream.sendFCW.Return(frame.DeltaWindowSize()); err != nil {
		//	c.writeRstStream(stream, framing.STATUS_FLOW_CONTROL_ERROR)
		//}
	case framing.FRAME_GOAWAY:
		frame := f.(framing.GoAway)
		if s, ok := frame.(framing.ControlFrameWithStatusCode); ok {
			log.Printf("SPDY client GoAway. Last-good:%v Status:%v\n", frame.LastGoodStreamID(), s.StatusCode())
		} else {
			log.Printf("SPDY Client GoAway. Last-good:%v\n", frame.LastGoodStreamID())
		}
		return errGoAway
	default:
		return badFrame(fmt.Sprintf("type %v", f.Type()))
	}
	return nil
}

func (c *conn) readDataFrame(frame *framing.DataFrame) (err error) {
	streamID := frame.StreamID()
	stream := c.getStream(streamID)
	if stream == nil || stream.PeerHalfClosed() {
		c.writeRstStreamID(streamID, framing.StatusCodeStreamAlreadyClosed(c.Version))
		return
	}
	var n int64
	n, err = io.Copy(stream.Reader.writer, frame.Reader)
	if err != nil {
		log.Printf("SPDY readDataStream error: %v\n", err)
		if err == io.ErrClosedPipe { // Read closed, discard any data frame.
			io.Copy(ioutil.Discard, frame.Reader)
			return nil
		}
		return err
	}

	if n != int64(frame.Len()) {
		c.writeRstStream(stream, framing.STATUS_PROTOCOL_ERROR)
		return
	}
	if frame.Flags() == framing.FLAG_FIN {
		stream.PeerHalfClose(c)
		if err = stream.Reader.writer.Close(); err != nil {
			log.Printf("SPDY readDataStream close Reader.writer error: %v\n", err)
		}
	} else if c.Version >= 3 {
		var f framing.WindowUpdate
		if f, err = framing.NewWindowUpdate(c.Version, streamID, uint32(n)); err != nil {
			log.Panicf("SPDY can't create frame WINDOW_UPDATE: %v\n", err)
		}
		c.writeFrame(f, stream.Priority)
	}
	return
}

// push pushes the response of r to user-agent.
// Fields of r other than Path and RawQuery are ignored to obey "same-origin policy".
func (c *conn) push(associated *stream, priority byte, r *http.Request) (err error) {
	stream := &stream{
		ID:             newServerStreamID(),
		Priority:       priority,
		peerHalfClosed: true,
	}
	c.addStream(stream)
	var synStream framing.SynStream
	if synStream, err = newServerPushSynStream(c.Version, stream.ID, associated, r); err != nil {
		log.Panic(err)
	}
	var w responseWriter
	if w, err = newResponseWriter(c.Version, stream, c, synStream); err != nil {
		return
	}
	c.Handler.ServeHTTP(w, r)
	w.Close()
	return
}

func (c *conn) serveLoop() {
loop:
	for {
		stream := c.streamQ.Pop().(*stream)
		if stream == nil {
			break loop
		}
		go c.serveStream(stream)
	}
	c.exit <- true
}

func (c *conn) serveStream(stream *stream) {
	var err error
	var req *http.Request
	if req, err = httpRequest(c.Version, stream); err != nil {
		log.Printf("Convert stream #v to http request error: %v\n", err)
		c.writeRstStream(stream, framing.STATUS_PROTOCOL_ERROR)
		return
	}

	if stream.HalfClosed() {
		log.Printf("SPDY won't serve stream #%v, already half-closed.\n", stream.ID)
		return
	}

	var synReply framing.SynReply
	synReply, err = framing.NewSynReply(c.Version, stream.ID)
	if err != nil {
		log.Panic(err)
	}

	var w responseWriter
	if w, err = newResponseWriter(c.Version, stream, c, synReply); err != nil {
		panic(err)
	}
	defer func() {
		var err error
		if err = w.Close(); err != nil {
			log.Printf("SPDY serveStream close responseWriter error: %v\n", err)
		}
		if stream.Reader != nil {
			if err = stream.Reader.reader.Close(); err != nil {
				log.Printf("SPDY serveStream close stream.Reader.reader error: %v\n", err)
			}
		}
		stream.HalfClose(c)
	}()
	c.Handler.ServeHTTP(w, req)
}

func (c *conn) closeStream(stream *stream) {
	if stream.Reader != nil {
		stream.Reader.writer.Close()
		stream.Reader.reader.Close()
	}
	c.deleteStream(stream.ID)
}

func (c *conn) writeFrame(f framing.Frame, priority byte) {
	if frame, ok := f.(framing.FrameWithStreamID); ok {
		if stream := c.getStream(frame.StreamID()); stream == nil || stream.HalfClosed() {
			log.Printf("SPDY Write on stream #%v discarded.\n", frame.StreamID())
			return
		}
	}
	c.framesToWrite.Push(&frameWithPriority{
		Priority: priority,
		Seq:      c.nextFrameWriteSeq(),
		Frame:    f,
	})
}

func (c *conn) writeRstStreamID(streamID uint32, statusCode uint32) {
	log.Printf("Server reset stream #%v due to %v\n", streamID, statusCode)
	if f, err := framing.NewRstStream(c.Version, streamID, statusCode); err != nil {
		log.Panicf("SPDY create frame error: %v\n", err)
	} else {
		c.writeFrame(f, maxFramePriority)
	}
}

func (c *conn) writeRstStream(stream *stream, statusCode uint32) {
	if stream.HalfClosed() {
		return
	}
	c.writeRstStreamID(stream.ID, statusCode)
}

func (c *conn) writeLoop() {
	var err error
loop:
	for {
		f := c.framesToWrite.Pop().(*frameWithPriority)
		if f.Frame == nil {
			break loop
		}
		if err = framing.WriteFrame(c.encoderr, f.Frame); err != nil {
			break loop
		}
		if err = c.w.Flush(); err != nil {
			break loop
		}
	}
	if err != nil {
		logFunc := log.Printf
		if _, netErr := err.(net.Error); err != io.EOF && !netErr {
			logFunc = log.Panicf
		}
		logFunc("SPDY write error: %v\n", err)
	}
	c.exit <- true
}

type frameWithPriority struct {
	Priority byte
	Seq      uint32
	Frame    framing.Frame
}

func (f *frameWithPriority) TakePrecedenceOver(other util.PriorityItem) bool {
	otherFrame := other.(*frameWithPriority)
	if f.Priority == otherFrame.Priority {
		return f.Seq < otherFrame.Seq
	}
	return f.Priority > otherFrame.Priority
}
