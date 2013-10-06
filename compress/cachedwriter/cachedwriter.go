package cachedwriter

import (
	"errors"
	"io"
	"log"
	"net/http"
)

// Objects implementing the ResetableWriter interface can be reset after creation.
//
// TODO: compress.flate.Writer and compress.gzip.Writer does not have Reset
// method by now(go1.1), but likely in the future.
// https://groups.google.com/forum/#!msg/golang-dev/zY1_18YB3VE/_A99CiuEIWsJ
// https://code.google.com/p/go/issues/detail?id=6317
type ResetableWriter interface {
	io.WriteCloser
	Reset(io.Writer) error
}

// Objects implementing the ResetableWriterCreator interface can creates
// ResetableWriters.
type ResetableWriterCreator interface {
	Create(io.Writer) (ResetableWriter, error)
}

type ResetableWriterCreatorFunc func(w io.Writer) (ResetableWriter, error)

func (f ResetableWriterCreatorFunc) Create(w io.Writer) (ResetableWriter, error) {
	return f(w)
}

// resetableWriter is returned by Factory.NewWriter. It acts as a safe guard
// for Factory.RecycleWriter to do type checking.
type resetableWriter struct {
	ResetableWriter
	factory *Factory
	closed  bool
}

var errClosed = errors.New("Writer closed")

// Do not close w.ResetableWriter, but just Flush() it. Returns w to cache.
// See realClose().
func (w *resetableWriter) Close() error {
	if w.closed {
		return errClosed
	}
	w.closed = true
	// Return to cache.
	return w.factory.recycleWriter(w)
}

func (w *resetableWriter) realClose() error {
	return w.ResetableWriter.Close()
}

func (w *resetableWriter) Reset(writer io.Writer) error {
	err := w.ResetableWriter.Reset(writer)
	if err == nil {
		w.closed = false
	}
	return err
}

// Factory is burrow compress.WriteFactory that caches certain number of writers
// avoiding unnecessary writer creation.
//
// See ``A leaky buffer'' in "Effective Go".
type Factory struct {
	// The concurrency saft cache.
	cache   chan *resetableWriter
	creator ResetableWriterCreator
}

func NewFactory(writerCreator ResetableWriterCreator, cacheSize int) *Factory {
	if cacheSize < 0 {
		cacheSize = 255
	}
	return &Factory{cache: make(chan *resetableWriter, cacheSize), creator: writerCreator}
}

// NewWriter returns a cached writer if the cache is not empty, or a newly created one.
func (f *Factory) NewWriter(w http.ResponseWriter) (io.WriteCloser, error) {
	var writer *resetableWriter
	var err error
	select {
	case writer = <-f.cache:
		err = writer.Reset(w) // Reuse cached one.
	default:
		var newWriter ResetableWriter
		newWriter, err = f.creator.Create(w)
		if err == nil {
			writer = &resetableWriter{ResetableWriter: newWriter, factory: f}
		}
	}
	if err != nil {
		return nil, err
	}
	log.Printf("%v\n", writer)
	return writer, nil
}

// RecycleWriter returns w to cache for later use if the cache is not full, or
// closes and abandons it.
func (f *Factory) recycleWriter(w *resetableWriter) error {
	select {
	case f.cache <- w:
		return nil
	default:
		return w.realClose()
	}
}
