package util

import (
	"errors"
	"github.com/kevin-yuan/burrow/spdy/framing"
	"log"
	"sync"
)

// When a SPDY connection is first established, the default initial window size
// for all streams is 64KB.
const DEFAULT_WINDOW_SIZE uint32 = 64 * 1024

var ErrWindowOverflow = errors.New("Window overflow")

// FlowCtrlWin is the implementation of SPDY Flow Control Window for sending.
type FlowCtrlWin struct {
	L        sync.Mutex
	notFull  *sync.Cond // Cond using l to signal window "not full".
	size     int64      // Amount remaining. Can be negative.
	initSize int64
}

// NewFlowCtrlWin calls  NewCtrlFlowWinInitSize(DEFAULT_WINDOW_SIZE)
func NewFlowCtrlWin() *FlowCtrlWin {
	win, err := NewFlowCtrlInitSize(DEFAULT_WINDOW_SIZE)
	if err != nil {
		panic(err)
	}
	return win
}

// NewFlowCtrlInitSize creates a new flow control window for sending.
// This method returns framing.ErrInvalidDeltaWindowSize if the delta size is
// invalid
func NewFlowCtrlInitSize(initSize uint32) (*FlowCtrlWin, error) {
	if initSize < 1 || initSize > framing.MAX_DELTA_WINDOW_SIZE {
		return nil, framing.ErrInvalidDeltaWindowSize
	}
	w := &FlowCtrlWin{size: int64(initSize), initSize: int64(initSize)}
	w.notFull = sync.NewCond(&w.L)
	log.Printf("[WIN] create with size %v\n", w.size)
	return w, nil
}

// InitSize chances the window size. Should be called when receiving a SETTINGS
// frame with ID SETTINGS_INITIAL_WINDOW_SIZE. This method returns
// framing.ErrInvalidDeltaWindowSize if the delta size is invalid.
func (w *FlowCtrlWin) InitSize(initSize uint32) error {
	if initSize < 1 || initSize > framing.MAX_DELTA_WINDOW_SIZE {
		return framing.ErrInvalidDeltaWindowSize
	}
	w.size += (int64(initSize) - w.initSize) // w.size can be negative now.
	w.initSize = int64(initSize)
	if w.size > 0 {
		w.notFull.Signal()
	}
	log.Printf("[WIN] init size %v\n", initSize)
	return nil
}

// Use takes up some amount of window. L must be locked before call this method.
// When sending data, lock L first, then call this method and send that amount
// of data, and unlock L when done.
func (w *FlowCtrlWin) Use(delta uint32) {
	for w.size < int64(delta) {
		w.notFull.Wait()
	}
	w.size -= int64(delta)
	log.Printf("[WIN] used %v\n", delta)
}

// Return returns some amount of window. L must be locked before call this method.
// When a WINDOW_UPDATE frame is received, lock L first, then call this method
// with the delta widnow size, and unlock L when done. This method returns
// framing.ErrInvalidDeltaWindowSize if the delta size is invalid, and returns
// ErrWindowDeltaOverflow if plusing this delta causes window over flow.
func (w *FlowCtrlWin) Return(delta uint32) error {
	if delta < 1 || uint32(delta) > framing.MAX_DELTA_WINDOW_SIZE {
		return framing.ErrInvalidDeltaWindowSize
	}
	newSize := w.size + int64(delta)
	if newSize > int64(framing.MAX_DELTA_WINDOW_SIZE) { // Window size overflow
		return ErrWindowOverflow
	}
	w.size = newSize
	if w.size > 0 {
		w.notFull.Signal()
	}
	log.Printf("[WIN] returned %v\n", delta)
	return nil
}
