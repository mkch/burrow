package framing

import (
	"sort"
	"strings"
)

type nameValueV3 struct {
	Name  string `field:"lenbits:32"`
	Value string `field:"lenbits:32"`
}

type headerBlockV3 []nameValueV3

// Search a header and returns the the index i and the header p.
// If a header with the name is not found, the returned p is nil and i is the
// index where a header with this name should be inserted.
// Name parameter should be in lower case.
func (h *headerBlockV3) search(name string) (i int, p *nameValueV3) {
	l := len(*h)
	i = sort.Search(l, func(i int) bool { return (*h)[i].Name >= name })
	if i < l && (*h)[i].Name == name {
		// Found
		p = &(*h)[i]
	}
	return
}

func (h *headerBlockV3) insert(i int, p nameValueV3) {
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

func (h *headerBlockV3) delete(i int) {
	if i < 0 || i > len(*h) {
		panic("Index out ouf bound")
	}
	// Move [i+1:] one element leftward.
	copy((*h)[i:], (*h)[i+1:])
	// Reduce length
	(*h) = (*h)[:len(*h)-1]
}

// Add a header
func (h *headerBlockV3) Add(name string, value ...string) error {
	if len(name) == 0 {
		return ErrInvalidHeaderName
	}
	name = strings.ToLower(name)
	v := strings.Join(value, "\x00")
	if i, p := h.search(name); p != nil {
		p.Value = p.Value + "\x00" + v
	} else {
		h.insert(i, nameValueV3{Name: name, Value: v})
	}
	return nil
}

// Get the first header with this name.
func (h *headerBlockV3) GetFirst(name string) string {
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
func (h *headerBlockV3) Get(name string) []string {
	name = strings.ToLower(name)
	if _, p := h.search(name); p != nil {
		return strings.Split(p.Value, "\x00")
	}
	return nil
}

// Names returns all names of headers.
func (h *headerBlockV3) Names() (names []string) {
	for _, p := range *h {
		names = append(names, p.Name)
	}
	return
}

type synStreamV3 struct {
	controlFrame  `field:"-"`
	Flags_        byte          `field:"bits:8"`
	Length        uint32        `field:"bits:24,limit"`
	X1            byte          `field:"bits:1"`
	StreamID_     uint32        `field:"bits:31"`
	X2            byte          `field:"bits:1"`
	AssociatedTo_ uint32        `field:"bits:31"`
	Priority_     byte          `field:"bits:3"`
	Unused        uint16        `field:"bits:5"`
	Slot_         byte          `field:"bits:8"`
	HeaderBlock_  []nameValueV3 `field:"lenbits:32,zlib"`
}

func newSynStreamV3(streamID uint32, flags byte) (*synStreamV3, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if flags != FLAG_NONE && flags != FLAG_FIN && flags != FLAG_UNIDIRECTIONAL {
		return nil, ErrInvalidFlags
	}
	return &synStreamV3{StreamID_: streamID, Flags_: flags}, nil
}

func (f *synStreamV3) Type() uint16 {
	return FRAME_SYN_STREAM
}

func (f *synStreamV3) AssociatedToStreamID() uint32 {
	return f.AssociatedTo_
}

func (f *synStreamV3) SetAssociatedToStreamID(to uint32) error {
	if to > MAX_STREAM_ID {
		return ErrInvalidStreamID
	}
	f.AssociatedTo_ = to
	return nil
}

func (f *synStreamV3) Priority() byte {
	return f.Priority_
}

func (f *synStreamV3) SetPriority(pri byte) error {
	if pri > MAX_PRIORITY_V3 {
		return ErrInvalidPriority
	}
	f.Priority_ = pri
	return nil
}

func (f *synStreamV3) StreamID() uint32 {
	return f.StreamID_
}

func (f *synStreamV3) Flags() byte {
	return f.Flags_
}

func (f *synStreamV3) Headers() HeaderBlock {
	return (*headerBlockV3)(&f.HeaderBlock_)
}

func (f *synStreamV3) Slot() byte {
	return f.Slot_
}

func (f *synStreamV3) SetSlot(slot byte) {
	f.Slot_ = slot
}

type synReplyV3 struct {
	controlFrame `field:"-"`
	Flags_       byte          `field:"bits:8"`
	Length       uint32        `field:"bits:24,limit"`
	X            byte          `field:"bits:1"`
	StreamID_    uint32        `field:"bits:31"`
	HeaderBlock_ []nameValueV3 `field:"lenbits:32,zlib"`
}

func newSynReplyV3(streamID uint32) (*synReplyV3, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}

	return &synReplyV3{StreamID_: streamID}, nil
}

func (f *synReplyV3) Headers() HeaderBlock {
	return (*headerBlockV3)(&f.HeaderBlock_)
}

func (f *synReplyV3) Flags() byte {
	return f.Flags_
}

func (f *synReplyV3) SetFlags(flags byte) error {
	if flags != FLAG_NONE && flags != FLAG_FIN {
		return ErrInvalidFlags
	}
	f.Flags_ = flags
	return nil
}

func (f *synReplyV3) StreamID() uint32 {
	return f.StreamID_
}

func (f *synReplyV3) Type() uint16 {
	return FRAME_SYN_RELY
}

type rstStreamV3 struct {
	controlFrame `field:"-"`
	Flags        byte   `field:"bits:8"`
	Length       uint32 `field:"bits:24,limit"`
	X            byte   `field:"bits:1"`
	StreamID_    uint32 `field:"bits:31"`
	StatusCode_  uint32 `field:"bits:32"`
}

func newRstStreamV3(streamID uint32, statusCode uint32) (*rstStreamV3, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if statusCode < STATUS_PROTOCOL_ERROR || statusCode > STATUS_FRAME_TOO_LARGE {
		return nil, ErrInvalidStatausCode
	}
	return &rstStreamV3{StreamID_: streamID, StatusCode_: statusCode}, nil
}

func (f *rstStreamV3) Type() uint16 {
	return FRAME_RST_STREAM
}

func (f *rstStreamV3) StreamID() uint32 {
	return f.StreamID_
}

func (f *rstStreamV3) StatusCode() uint32 {
	return f.StatusCode_
}

type settingEntryV3 struct {
	ID    uint32 `field:"bits:24"`
	Flags byte   `field:"bits:8"`
	Value uint32 `field:"bits:32"`
}

type settingEntriesV3 []settingEntryV3

func (s *settingEntriesV3) search(ID uint32) (i int, p *settingEntryV3) {
	l := len(*s)
	i = sort.Search(l, func(i int) bool { return (*s)[i].ID >= ID })
	if i < l && (*s)[i].ID == ID {
		// Found
		p = &(*s)[i]
	}
	return
}

func (s *settingEntriesV3) insert(i int, p settingEntryV3) {
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

func (s *settingEntriesV3) delete(i int) {
	if i < 0 || i > len(*s) {
		panic("Index out ouf bound")
	}
	// Move [i+1:] one element leftward.
	copy((*s)[i:], (*s)[i+1:])
	// Reduce length
	(*s) = (*s)[:len(*s)-1]
}

// A single SETTINGS frame MUST not contain multiple values for the same ID.
func (s *settingEntriesV3) Set(ID uint32, flags byte, value uint32) error {
	if ID < 1 || ID > 7 {
		return ErrInvalidSettingID
	}
	if flags != FLAG_NONE &&
		flags != FLAG_SETTINGS_PERSIST_VALUE && flags != FLAG_SETTINGS_PERSISTED {
		return ErrInvalidSettingFlags
	}
	if i, p := s.search(ID); p != nil {
		p.Flags = flags
		p.Value = value
	} else {
		s.insert(i, settingEntryV3{ID: ID, Flags: flags, Value: value})
	}
	return nil
}

func (s *settingEntriesV3) Get(ID uint32) (flags byte, value uint32, exists bool) {
	if _, p := s.search(ID); p != nil {
		return p.Flags, p.Value, true
	}
	return 0, 0, false
}

func (s *settingEntriesV3) IDs() (IDs []uint32) {
	for _, p := range *s {
		IDs = append(IDs, p.ID)
	}
	return
}

type settingsV3 struct {
	controlFrame `field:"-"`
	Flags_       byte             `field:"bits:8"`
	Length       uint32           `field:"bits:24,limit"`
	Entries_     []settingEntryV3 `field:"lenbits:32"`
}

func newSettingsV3(flags byte) (*settingsV3, error) {
	if flags != FLAG_NONE && flags != FLAG_SETTINGS_CLEAR_SETTINGS {
		return nil, ErrInvalidSettingFlags
	}
	return &settingsV3{Flags_: flags}, nil
}

func (s *settingsV3) Flags() byte {
	return s.Flags_
}

func (s *settingsV3) Entries() SettingEntries {
	return (*settingEntriesV3)(&s.Entries_)
}

func (f *settingsV3) Type() uint16 {
	return FRAME_SETTINGS
}

type goAwayV3 struct {
	controlFrame      `field:"-"`
	Flags             byte   `field:"bits:8"`
	Length            uint32 `field:"bits:24,limit"`
	X                 byte   `field:"bits:1"`
	LastGoodStreamID_ uint32 `field:"bits:31"`
	StatusCode_       uint32 `field:"bits:32"`
}

func newGoAwayV3(lastGood uint32) *goAwayV3 {
	return &goAwayV3{LastGoodStreamID_: lastGood}
}

func (f *goAwayV3) LastGoodStreamID() uint32 {
	return f.LastGoodStreamID_
}

func (f *goAwayV3) Type() uint16 {
	return FRAME_GOAWAY
}

func (f *goAwayV3) StatusCode() uint32 {
	return f.StatusCode_
}

func (f *goAwayV3) SetStatusCode(statusCode uint32) error {
	if statusCode != STATUS_GOAWAY_OK && statusCode != STATUS_GOAWAY_PROTOCOL_ERROR &&
		statusCode != STATUS_GOAWAY_INTERNAL_ERROR {
		return ErrInvalidStatausCode
	}
	f.StatusCode_ = statusCode
	return nil
}

type headersV3 struct {
	controlFrame `field:"-"`
	Flags_       byte          `field:"bits:8"`
	Length       uint32        `field:"bits:24,limit"`
	X            byte          `fields:"bits:1"`
	StreamID_    uint32        `field:"bits:31"`
	HeaderBlock  []nameValueV3 `field:"lenbits:16,zlib"`
}

func newHeadersV3(streamID uint32, flags byte) (*headersV3, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if flags != FLAG_NONE && flags != FLAG_FIN {
		return nil, ErrInvalidFlags
	}
	return &headersV3{StreamID_: streamID, Flags_: flags}, nil
}

func (f *headersV3) StreamID() uint32 {
	return f.StreamID_
}

func (f *headersV3) Flags() byte {
	return f.Flags_
}

func (f *headersV3) Headers() HeaderBlock {
	return (*headerBlockV3)(&f.HeaderBlock)
}

func (f *headersV3) Type() uint16 {
	return FRAME_HEADERS
}

type windowUpdateV3 struct {
	controlFrame     `field:"-"`
	Flags_           byte   `field:"bits:8"`
	Length           uint32 `field:"bits:24,limit"`
	X                byte   `field:"bits:1"`
	StreamID_        uint32 `field:"bits:31"`
	X1               byte   `field:"bits:1"`
	DeltaWindowSize_ uint32 `field:"bits:31"`
}

func newWindowUpdateV3(streamID uint32, deltaWindowSize uint32) (*windowUpdateV3, error) {
	if streamID == 0 || streamID > MAX_STREAM_ID {
		return nil, ErrInvalidStreamID
	}
	if deltaWindowSize < MIN_DELTA_WINDOW_SIZE || deltaWindowSize > MAX_DELTA_WINDOW_SIZE {
		return nil, ErrInvalidDeltaWindowSize
	}
	return &windowUpdateV3{StreamID_: streamID, DeltaWindowSize_: deltaWindowSize}, nil
}

func (f *windowUpdateV3) StreamID() uint32 {
	return f.StreamID_
}

func (f *windowUpdateV3) DeltaWindowSize() uint32 {
	return f.DeltaWindowSize_
}

func (f *windowUpdateV3) Type() uint16 {
	return FRAME_WINDOW_UPDATE
}
