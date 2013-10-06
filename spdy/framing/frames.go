package framing

import (
	"errors"
	"github.com/kevin-yuan/burrow/spdy/framing/fields"
	"io"
)

// Control frame types.
const (
	FRAME_SYN_STREAM    uint16 = 1
	FRAME_SYN_RELY      uint16 = 2
	FRAME_RST_STREAM    uint16 = 3
	FRAME_SETTINGS      uint16 = 4
	FRAME_NOOP          uint16 = 5
	FRAME_PING          uint16 = 6
	FRAME_GOAWAY        uint16 = 7
	FRAME_HEADERS       uint16 = 8
	FRAME_WINDOW_UPDATE uint16 = 9
	FRAME_CREDENTIAL    uint16 = 0x1011
)

const (
	FLAG_NONE                                         byte = 0
	FLAG_FIN                                          byte = 0x01
	FLAG_UNIDIRECTIONAL                               byte = 0x02
	FLAG_SETTINGS_CLEAR_SETTINGS                      byte = 0x1
	FLAG_SETTINGS_CLEAR_PREVIOUSLY_PERSISTED_SETTINGS      = FLAG_SETTINGS_CLEAR_SETTINGS
)

// RstStream status codes.
const (
	STATUS_PROTOCOL_ERROR        uint32 = 1
	STATUS_INVALID_STREAM        uint32 = 2
	STATUS_REFUSED_STREAM        uint32 = 3
	STATUS_UNSUPPORTED_VERSION   uint32 = 4
	STATUS_CANCEL                uint32 = 5
	STATUS_INTERNAL_ERROR        uint32 = 6
	STATUS_FLOW_CONTROL_ERROR    uint32 = 7
	STATUS_STREAM_IN_USE         uint32 = 8
	STATUS_STREAM_ALREADY_CLOSED uint32 = 9
	STATUS_INVALID_CREDENTIALS   uint32 = 10
	STATUS_FRAME_TOO_LARGE       uint32 = 11
)

// V3 GoAway status code.
const (
	STATUS_GOAWAY_OK             uint32 = 0
	STATUS_GOAWAY_PROTOCOL_ERROR uint32 = 1
	STATUS_GOAWAY_INTERNAL_ERROR uint32 = 2
)

// Setting IDs.
const (
	ID_SETTINGS_UPLOAD_BANDWIDTH               uint32 = 1
	ID_SETTINGS_DOWNLOAD_BANDWIDTH             uint32 = 2
	ID_SETTINGS_ROUND_TRIP_TIME                uint32 = 3
	ID_SETTINGS_MAX_CONCURRENT_STREAMS         uint32 = 4
	ID_SETTINGS_CURRENT_CWND                   uint32 = 5
	ID_SETTINGS_DOWNLOAD_RETRANS_RATE          uint32 = 6
	ID_SETTINGS_INITIAL_WINDOW_SIZE            uint32 = 7
	ID_SETTINGS_CLIENT_CERTIFICATE_VECTOR_SIZE uint32 = 8
)

// Setting flags.
const (
	FLAG_SETTINGS_PERSIST_VALUE byte = 0x1
	FLAG_SETTINGS_PERSISTED     byte = 0x2
)

// SynStream priority range.
const (
	MIN_PRIORITY    byte = 0
	MAX_PRIORITY_V3 byte = 7
	MAX_PRIORITY_V2 byte = 3
)

const MAX_STREAM_ID uint32 = 0x8FFFFFFF

const (
	MIN_DELTA_WINDOW_SIZE uint32 = 1
	MAX_DELTA_WINDOW_SIZE uint32 = 0x7FFFFFFF //  2^31 - 1
)

var (
	ErrInvalidVersion          = errors.New("Invalid version")
	ErrInvalidControlFrameType = errors.New("Invalid conrol frame type")
	ErrUnsupportedVersion      = errors.New("Unsupported version")
	ErrInvalidFlags            = errors.New("Invalid flags")
	ErrInvalidStreamID         = errors.New("Invalid stream ID")
	ErrInvalidPriority         = errors.New("Invalid priority")
	ErrInvalidStatausCode      = errors.New("Invalid status code")
	ErrInvalidSettingID        = errors.New("Invalid setting ID")
	ErrInvalidSettingFlags     = errors.New("Invalid setting flags")
	ErrSetingIDExists          = errors.New("Setting ID already exists in frame")
	ErrInvalidDeltaWindowSize  = errors.New("Invalid delta window size")
	ErrInvalidSlot             = errors.New("Invalid slot")
	ErrInvalidHeaderName       = errors.New("Invalid header name")
	ErrInvalidHeaderValue      = errors.New("Invalid header value")
	ErrInvalidFrameLength      = errors.New("Invalid frame length")
	ErrFrameTooLarge           = errors.New("Frame too large")
)

func StatusCodeStreamInUse(version uint16) uint32 {
	switch version {
	case 2:
		return STATUS_PROTOCOL_ERROR
	case 3:
		return STATUS_STREAM_IN_USE
	default:
		panic(ErrUnsupportedVersion)
	}
}

func StatusCodeStreamAlreadyClosed(version uint16) uint32 {
	switch version {
	case 2:
		return STATUS_INVALID_STREAM
	case 3:
		//9 - STREAM_ALREADY_CLOSED. The endpoint received a data or SYN_REPLY
		// frame for a stream which is half closed.
		return STATUS_STREAM_ALREADY_CLOSED
	default:
		panic(ErrUnsupportedVersion)
	}
}

type ControlFrameWithFlags interface {
	ControlFrame
	Flags() byte
}

type ControlFrameWithSetFlags interface {
	ControlFrameWithFlags
	SetFlags(byte) error
}

type ControlFrameWithHeaders interface {
	ControlFrame
	Headers() HeaderBlock
}

type FrameWithStreamID interface {
	StreamID() uint32
}

type ControlFrameWithStatusCode interface {
	ControlFrame
	StatusCode() uint32
}

type ControlFrameWithSetStatusCode interface {
	ControlFrame
	StatusCode() uint32
	SetStatusCode(statusCode uint32) error
}

type Frame interface {
	// Whether a control frame.
	IsControl() bool
}

type ControlFrame interface {
	Frame
	Version() uint16
	Type() uint16
	setVersion(version uint16)
}

type controlFrame struct {
	version uint16
}

func (f *controlFrame) IsControl() bool {
	return true
}

func (f *controlFrame) Version() uint16 {
	return f.version
}

func (f *controlFrame) setVersion(version uint16) {
	f.version = version
}

type HeaderBlock interface {
	// Add a header
	Add(name string, value ...string) error
	// Get the first header with this name.
	GetFirst(name string) string
	// Get all headers with this name.
	Get(name string) []string
	// Names returns all names of headers.
	Names() (names []string)
}

type SynStream interface {
	ControlFrame
	Flags() byte
	StreamID() uint32
	AssociatedToStreamID() uint32
	SetAssociatedToStreamID(to uint32) error
	// See MIN_PRIORITY, MAX_PRIORITY_Vx
	Priority() byte
	SetPriority(pri byte) error
	Headers() HeaderBlock
}

func NewSynStream(version uint16, streamID uint32, flags byte) (f SynStream, err error) {
	switch version {
	case 2:
		f, err = newSynStreamV2(streamID, flags)
	case 3:
		f, err = newSynStreamV3(streamID, flags)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type SynReply interface {
	ControlFrame
	Flags() byte
	SetFlags(flags byte) error
	StreamID() uint32
	Headers() HeaderBlock
}

func NewSynReply(version uint16, streamID uint32) (f SynReply, err error) {
	switch version {
	case 2:
		f, err = newSynReplyV2(streamID)
	case 3:
		f, err = newSynReplyV3(streamID)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type RstStream interface {
	ControlFrame
	StreamID() uint32
	StatusCode() uint32
}

func NewRstStream(version uint16, streamID uint32, statusCode uint32) (f RstStream, err error) {
	switch version {
	case 2:
		f, err = newRstStreamV2(streamID, statusCode)
	case 3:
		f, err = newRstStreamV3(streamID, statusCode)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type SettingEntries interface {
	// A single SETTINGS frame MUST not contain multiple values for the same ID.
	Set(ID uint32, flags byte, value uint32) error
	Get(ID uint32) (flags byte, value uint32, exists bool)
	IDs() (IDs []uint32)
}

type Settings interface {
	ControlFrame
	Flags() byte
	Entries() SettingEntries
}

func NewSettings(version uint16, flags byte) (f Settings, err error) {
	switch version {
	case 2:
		f, err = newSettingsV2(flags)
	case 3:
		f, err = newSettingsV3(flags)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type Noop interface {
	ControlFrame
}

func NewNoop(version uint16) (f Noop, err error) {
	switch version {
	case 2:
		f = &noopV2{}
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type Ping interface {
	ControlFrame
	ID() uint32
}

func NewPing(version uint16, ID uint32) (f Ping, err error) {
	switch version {
	case 2:
	case 3:
		f = newPingV2(ID)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type GoAway interface {
	ControlFrame
	LastGoodStreamID() uint32
}

func NewGoAway(version uint16, lastGoodStreamID uint32) (f GoAway, err error) {
	switch version {
	case 2:
		f = newGoAwayV2(lastGoodStreamID)
	case 3:
		f = newGoAwayV3(lastGoodStreamID)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type Headers interface {
	ControlFrame
	StreamID() uint32
	Flags() byte
	Headers() HeaderBlock
}

func NewHeaders(version uint16, streamID uint32, flags byte) (f Headers, err error) {
	switch version {
	case 2:
		f, err = newHeadersV2(streamID, flags)
	case 3:
		f, err = newHeadersV3(streamID, flags)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type WindowUpdate interface {
	ControlFrame
	StreamID() uint32
	DeltaWindowSize() uint32
}

func NewWindowUpdate(version uint16, streamID uint32, deltaWindowSize uint32) (f WindowUpdate, err error) {
	switch version {
	case 3:
		f, err = newWindowUpdateV3(streamID, deltaWindowSize)
	default:
		return nil, ErrUnsupportedVersion
	}
	if err != nil {
		return nil, err
	}
	f.setVersion(version)
	return
}

type cframeCreator func() ControlFrame

var controlFrameSel = map[uint16]map[uint16]cframeCreator{
	2: map[uint16]cframeCreator{
		FRAME_SYN_STREAM: func() ControlFrame { return new(synStreamV2) },
		FRAME_SETTINGS:   func() ControlFrame { return new(settingsV2) },
		FRAME_GOAWAY:     func() ControlFrame { return new(goAwayV2) },
		FRAME_RST_STREAM: func() ControlFrame { return new(rstStreamV2) },
		FRAME_SYN_RELY:   func() ControlFrame { return new(synReplyV2) },
		FRAME_NOOP:       func() ControlFrame { return new(noopV2) },
		FRAME_PING:       func() ControlFrame { return new(pingV2) },
		FRAME_HEADERS:    func() ControlFrame { return new(headersV2) },
	},
	3: map[uint16]cframeCreator{
		FRAME_SYN_STREAM:    func() ControlFrame { return new(synStreamV3) },
		FRAME_SETTINGS:      func() ControlFrame { return new(settingsV3) },
		FRAME_GOAWAY:        func() ControlFrame { return new(goAwayV3) },
		FRAME_RST_STREAM:    func() ControlFrame { return new(rstStreamV3) },
		FRAME_SYN_RELY:      func() ControlFrame { return new(synReplyV3) },
		FRAME_PING:          func() ControlFrame { return new(pingV2) },
		FRAME_HEADERS:       func() ControlFrame { return new(headersV3) },
		FRAME_WINDOW_UPDATE: func() ControlFrame { return new(windowUpdateV3) },
	},
}

func createControlFrame(version uint16, ftype uint16) (f ControlFrame, err error) {
	m := controlFrameSel[version]
	if m == nil {
		return nil, ErrUnsupportedVersion
	}
	c := m[ftype]
	if c == nil {
		return nil, ErrInvalidControlFrameType
	}
	return c(), nil
}

func ReadFrame(decoder *fields.Decoder) (f Frame, err error) {
	// Control bit
	var cbit uint32
	if cbit, err = decoder.ReadBits(1); err != nil {
		return
	}
	if cbit == 1 {
		return readControlFrame(decoder)
	} else {
		return readDataFrame(decoder)
	}
}

func readControlFrame(decoder *fields.Decoder) (f ControlFrame, err error) {
	var v, t uint32
	// Version
	if v, err = decoder.ReadBits(15); err != nil {
		return
	}
	var version = uint16(v)
	if t, err = decoder.ReadBits(16); err != nil {
		return
	}
	var ftype uint16 = uint16(t)

	// Create control frame.
	if f, err = createControlFrame(version, ftype); err != nil {
		return
	}

	// Decode frame from.
	if err = decoder.Decode(f); err != nil {
		f = nil
		return
	}
	f.setVersion(version)
	return
}

func readDataFrame(decoder *fields.Decoder) (f *DataFrame, err error) {
	var frame DataFrame
	// Stream-ID
	if frame.streamID, err = decoder.ReadBits(31); err != nil {
		return
	}
	var flags uint32
	if flags, err = decoder.ReadBits(8); err != nil {
		return
	}
	frame.flags = byte(flags)
	if frame.length, err = decoder.ReadBits(24); err != nil {
		return
	}
	frame.Reader = io.LimitReader(decoder, int64(frame.length))
	return &frame, nil
}

func WriteFrame(encoder *fields.Encoder, frame Frame) (err error) {
	if frame.IsControl() {
		return writeControlFrame(encoder, frame.(ControlFrame))
	} else {
		return writeDataFrame(encoder, frame.(*DataFrame))
	}
}

func writeControlFrame(encoder *fields.Encoder, frame ControlFrame) (err error) {
	// Control bit
	if err = encoder.WriteBits(1, 1); err != nil {
		return
	}
	// Version
	if err = encoder.WriteBits(15, uint32(frame.Version())); err != nil {
		return
	}
	// Type
	if err = encoder.WriteBits(16, uint32(frame.Type())); err != nil {
		return
	}
	err = encoder.Encode(frame)
	return
}

func writeDataFrame(encoder *fields.Encoder, f *DataFrame) (err error) {
	// Control bit
	if err = encoder.WriteBits(1, 0); err != nil {
		return
	}
	// Stream-ID
	if err = encoder.WriteBits(31, f.streamID); err != nil {
		return
	}
	// Flags
	if err = encoder.WriteBits(8, uint32(f.flags)); err != nil {
		return
	}
	// Length
	if err = encoder.WriteBits(24, f.length); err != nil {
		return
	}
	// Data
	_, err = io.Copy(encoder, io.LimitReader(f, int64(f.length)))
	return
}
