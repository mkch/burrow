package framing

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
	"sort"
	"strings"
)

type nameValueV2 struct {
	Name  string `field:"lenbits:16"`
	Value string `field:"lenbits:16"`
}

type headerBlockV2 []nameValueV2

// Search a header and returns the the index i and the header p.
// If a header with the name is not found, the returned p is nil and i is the
// index where a header with this name should be inserted.
// Name parameter should be in lower case.
func (h *headerBlockV2) search(name string) (i int, p *nameValueV2) {
	l := len(*h)
	i = sort.Search(l, func(i int) bool { return (*h)[i].Name >= name })
	if i < l && (*h)[i].Name == name {
		// Found
		p = &(*h)[i]
	}
	return
}

func (h *headerBlockV2) insert(i int, p nameValueV2) {
	if i < 0 || i > len(*h) {
		panic("Index out ouf bound")
	}
	// Expand one more.
	*h = append(*h, p)
	if i == len(*h)-1 {
		return
	}
	// Move [i:] one element rightward.
	copy((*h)[i+1:], (*h)[i:])
	(*h)[i] = p
}

func (h *headerBlockV2) delete(i int) {
	if i < 0 || i > len(*h) {
		panic("Index out ouf bound")
	}
	// Move [i+1:] one element leftward.
	copy((*h)[i:], (*h)[i+1:])
	// Reduce length
	(*h) = (*h)[:len(*h)-1]
}

// Add a header
func (h *headerBlockV2) Add(name string, value ...string) error {
	if len(name) == 0 {
		return ErrInvalidHeaderName
	}
	name = strings.ToLower(name)
	v := strings.Join(value, "\x00")
	if i, p := h.search(name); p != nil {
		p.Value = p.Value + "\x00" + v
	} else {
		h.insert(i, nameValueV2{Name: name, Value: v})
	}
	return nil
}

// Get the first header with this name.
func (h *headerBlockV2) GetFirst(name string) string {
	name = strings.ToLower(name)
	if _, p := h.search(name); p != nil {
		if i := strings.IndexRune(p.Value, '\x00'); i != -1 {
			return p.Value[:i]
		}
		return p.Value
	}
	return ""
}

// Get all headers with this name.
func (h *headerBlockV2) Get(name string) []string {
	name = strings.ToLower(name)
	if _, p := h.search(name); p != nil {
		return strings.Split(p.Value, "\x00")
	}
	return nil
}

// Names returns all names of headers.
func (h *headerBlockV2) Names() (names []string) {
	for _, p := range *h {
		names = append(names, p.Name)
	}
	return
}

type synStreamV2 struct {
	controlFrame  `field:"-"`
	Flags_        byte          `field:"bits:8"`
	Length        uint32        `field:"bits:24,limit"`
	X1            byte          `field:"bits:1"`
	StreamID_     uint32        `field:"bits:31"`
	X2            byte          `field:"bits:1"`
	AssociatedTo_ uint32        `field:"bits:31"`
	Priority_     byte          `field:"bits:2"`
	Unused        uint16        `field:"bits:14"`
	HeaderBlock_  []nameValueV2 `field:"lenbits:16,zlib"`
}

func newSynStreamV2(streamID uint32, flags byte) (*synStreamV2, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if flags != FLAG_NONE && flags != FLAG_FIN && flags != FLAG_UNIDIRECTIONAL {
		return nil, ErrInvalidFlags
	}
	return &synStreamV2{StreamID_: streamID, Flags_: flags}, nil
}

func (f *synStreamV2) Type() uint16 {
	return FRAME_SYN_STREAM
}

func (f *synStreamV2) AssociatedToStreamID() uint32 {
	return f.AssociatedTo_
}

func (f *synStreamV2) SetAssociatedToStreamID(to uint32) error {
	if to > MAX_STREAM_ID {
		return ErrInvalidStreamID
	}
	f.AssociatedTo_ = to
	return nil
}

func (f *synStreamV2) Priority() byte {
	return f.Priority_
}

func (f *synStreamV2) SetPriority(pri byte) error {
	if pri > MAX_PRIORITY_V2 {
		return ErrInvalidPriority
	}
	f.Priority_ = pri
	return nil
}

func (f *synStreamV2) StreamID() uint32 {
	return f.StreamID_
}

func (f *synStreamV2) Flags() byte {
	return f.Flags_
}

func (f *synStreamV2) Headers() HeaderBlock {
	return (*headerBlockV2)(&f.HeaderBlock_)
}

type settingEntryV2 struct {
	ID    uint32 `field:"bits:24"`
	Flags byte   `field:"bits:8"`
	Value uint32 `field:"bits:32"`
}

func toV2BuggySettingID(n uint32) uint32 {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, n)
	copy(bytes[1:], bytes[:3])
	bytes[0] = 0
	return binary.BigEndian.Uint32(bytes)
}

func fromV2BuggySettingID(n uint32) uint32 {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, n)
	copy(bytes[:3], bytes[1:])
	bytes[3] = 0
	return binary.LittleEndian.Uint32(bytes)
}

type settingEntriesV2 []settingEntryV2

func (s *settingEntriesV2) search(ID uint32) (i int, p *settingEntryV2) {
	l := len(*s)
	i = sort.Search(l, func(i int) bool { return fromV2BuggySettingID((*s)[i].ID) >= fromV2BuggySettingID(ID) })
	if i < l && (*s)[i].ID == ID {
		// Found
		p = &(*s)[i]
	}
	return
}

func (s *settingEntriesV2) insert(i int, p settingEntryV2) {
	if i < 0 || i > len(*s) {
		panic("Index out ouf bound")
	}
	// Expand one more.
	*s = append(*s, p)
	if i == len(*s)-1 {
		return
	}
	// Move [i:] one element rightward.
	copy((*s)[i+1:], (*s)[i:])
	(*s)[i] = p
}

func (s *settingEntriesV2) delete(i int) {
	if i < 0 || i > len(*s) {
		panic("Index out ouf bound")
	}
	// Move [i+1:] one element leftward.
	copy((*s)[i:], (*s)[i+1:])
	// Reduce length
	(*s) = (*s)[:len(*s)-1]
}

// A single SETTINGS frame MUST not contain multiple values for the same ID.
func (s *settingEntriesV2) Set(ID uint32, flags byte, value uint32) error {
	if ID < 1 || ID > 7 {
		return ErrInvalidSettingID
	}
	ID = toV2BuggySettingID(ID)
	if flags != FLAG_NONE &&
		flags != FLAG_SETTINGS_PERSIST_VALUE && flags != FLAG_SETTINGS_PERSISTED {
		return ErrInvalidSettingFlags
	}
	if i, p := s.search(ID); p != nil {
		p.Flags = flags
		p.Value = value
	} else {
		s.insert(i, settingEntryV2{ID: ID, Flags: flags, Value: value})
	}
	return nil
}

func (s *settingEntriesV2) Get(ID uint32) (flags byte, value uint32, exists bool) {
	ID = toV2BuggySettingID(ID)
	if _, p := s.search(ID); p != nil {
		return p.Flags, p.Value, true
	}
	return 0, 0, false
}

func (s *settingEntriesV2) IDs() (IDs []uint32) {
	for _, p := range *s {
		IDs = append(IDs, p.ID)
	}
	return
}

type settingsV2 struct {
	controlFrame `field:"-"`
	Flags_       byte             `field:"bits:8"`
	Length       uint32           `field:"bits:24,limit"`
	Entries_     []settingEntryV2 `field:"lenbits:32"`
}

func newSettingsV2(flags byte) (*settingsV2, error) {
	if flags != FLAG_NONE && flags != FLAG_SETTINGS_CLEAR_SETTINGS {
		return nil, ErrInvalidSettingFlags
	}
	return &settingsV2{Flags_: flags}, nil
}

func (s *settingsV2) Flags() byte {
	return s.Flags_
}

func (s *settingsV2) Entries() SettingEntries {
	return (*settingEntriesV2)(&s.Entries_)
}

func (f *settingsV2) Type() uint16 {
	return FRAME_SETTINGS
}

type goAwayV2 struct {
	controlFrame      `field:"-"`
	Flags             byte   `field:"bits:8"`
	Length            uint32 `field:"bits:24,limit"`
	X                 byte   `field:"bits:1"`
	LastGoodStreamID_ uint32 `field:"bits:31"`
}

func newGoAwayV2(lastGood uint32) *goAwayV2 {
	return &goAwayV2{LastGoodStreamID_: lastGood}
}

func (f *goAwayV2) LastGoodStreamID() uint32 {
	return f.LastGoodStreamID_
}

func (f *goAwayV2) Type() uint16 {
	return FRAME_GOAWAY
}

type rstStreamV2 struct {
	controlFrame `field:"-"`
	Flags        byte   `field:"bits:8"`
	Length       uint32 `field:"bits:24,limit"`
	X            byte   `field:"bits:1"`
	StreamID_    uint32 `field:"bits:31"`
	StatusCode_  uint32 `field:"bits:32"`
}

func newRstStreamV2(streamID uint32, statusCode uint32) (*rstStreamV2, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if statusCode < 1 || statusCode > 7 {
		return nil, ErrInvalidStatausCode
	}
	return &rstStreamV2{StreamID_: streamID, StatusCode_: statusCode}, nil
}

func (f *rstStreamV2) Type() uint16 {
	return FRAME_RST_STREAM
}

func (f *rstStreamV2) StreamID() uint32 {
	return f.StreamID_
}

func (f *rstStreamV2) StatusCode() uint32 {
	return f.StatusCode_
}

type synReplyV2 struct {
	controlFrame `field:"-"`
	Flags_       byte          `field:"bits:8"`
	Length       uint32        `field:"bits:24,limit"`
	X            byte          `field:"bits:1"`
	StreamID_    uint32        `field:"bits:31"`
	Unused       uint16        `field:"bits:16"`
	HeaderBlock_ []nameValueV2 `field:"lenbits:16,zlib"`
}

func newSynReplyV2(streamID uint32) (*synReplyV2, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}

	return &synReplyV2{StreamID_: streamID}, nil
}

func (f *synReplyV2) Headers() HeaderBlock {
	return (*headerBlockV2)(&f.HeaderBlock_)
}

func (f *synReplyV2) Flags() byte {
	return f.Flags_
}

func (f *synReplyV2) SetFlags(flags byte) error {
	if flags != FLAG_NONE && flags != FLAG_FIN {
		return ErrInvalidFlags
	}
	f.Flags_ = flags
	return nil
}

func (f *synReplyV2) StreamID() uint32 {
	return f.StreamID_
}

func (f *synReplyV2) Type() uint16 {
	return FRAME_SYN_RELY
}

type noopV2 struct {
	controlFrame `field:"-"`
	Flags        byte   `field:"bits:8"`
	Length       uint32 `field:"bits:24,limit"`
}

func (f *noopV2) Type() uint16 {
	return FRAME_NOOP
}

type pingV2 struct {
	controlFrame `field:"-"`
	Flags        byte   `field:"bits:8"`
	Length       uint32 `field:"bits:24,limit"`
	ID_          uint32 `field:"bits:32"`
}

func (f *pingV2) Type() uint16 {
	return FRAME_PING
}

func newPingV2(ID uint32) *pingV2 {
	return &pingV2{ID_: ID}
}

func (f *pingV2) ID() uint32 {
	return f.ID_
}

type headersV2 struct {
	controlFrame `field:"-"`
	Flags_       byte          `field:"bits:8"`
	Length       uint32        `field:"bits:24,limit"`
	X            byte          `fields:"bits:1"`
	StreamID_    uint32        `field:"bits:31"`
	Unused       uint16        `field:"bits:16"`
	HeaderBlock  []nameValueV2 `field:"lenbits:16,zlib"`
}

func newHeadersV2(streamID uint32, flags byte) (*headersV2, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if flags != FLAG_NONE && flags != FLAG_FIN {
		return nil, ErrInvalidFlags
	}
	return &headersV2{StreamID_: streamID, Flags_: flags}, nil
}

func (f *headersV2) StreamID() uint32 {
	return f.StreamID_
}

func (f *headersV2) Flags() byte {
	return f.Flags_
}

func (f *headersV2) Headers() HeaderBlock {
	return (*headerBlockV2)(&f.HeaderBlock)
}

func (f *headersV2) Type() uint16 {
	return FRAME_HEADERS
}

type DataFrame struct {
	io.Reader
	streamID uint32
	flags    byte
	length   uint32
}

// IsControl returns false.
func (d *DataFrame) IsControl() bool {
	return false
}

func (d *DataFrame) Close() (err error) {
	_, err = io.Copy(ioutil.Discard, d)
	return
}

func (d *DataFrame) StreamID() uint32 {
	return d.streamID
}

func (d *DataFrame) SetStreamID(id uint32) (err error) {
	if id == 0 || id > MAX_STREAM_ID {
		return ErrInvalidStreamID
	}
	d.streamID = id
	return nil
}

func (d *DataFrame) Flags() byte {
	return d.flags
}

func (d *DataFrame) SetFlags(flags byte) (err error) {
	if flags != FLAG_NONE && flags != FLAG_FIN {
		return ErrInvalidFlags
	}
	d.flags = flags
	return nil
}

func (d *DataFrame) SetLen(len uint32) {
	d.length = len
}

func (d *DataFrame) Len() uint32 {
	return d.length
}

func NewDataFrame(streamID uint32, r io.Reader, len uint32) *DataFrame {
	return &DataFrame{streamID: streamID, Reader: r, length: len}
}

func NewDataFrameBytes(streamID uint32, p []byte) *DataFrame {
	return NewDataFrame(streamID, bytes.NewBuffer(p), uint32(len(p)))
}

func NewDataFrameString(streamID uint32, s string) *DataFrame {
	return NewDataFrameBytes(streamID, []byte(s))
}
