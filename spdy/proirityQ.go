package spdy

import (
	"container/heap"
	"github.com/kevin-yuan/burrow/spdy/framing"
)

type streamPriorityQ []*stream

func (q streamPriorityQ) Len() int {
	return len(q)
}

func (q streamPriorityQ) Less(i, j int) bool {
	if q[i].Priority == q[j].Priority {
		return q[i].ID < q[j].ID
	}
	return q[i].Priority > q[j].Priority
}

func (q streamPriorityQ) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *streamPriorityQ) Push(x interface{}) {
	*q = append(*q, x.(*stream))
}

func (q *streamPriorityQ) Pop() (item interface{}) {
	last := len(*q) - 1
	item = (*q)[last]
	*q = (*q)[:last]
	return
}

type blockingStreamPriorityQ struct {
	q streamPriorityQ
	s *semaphore
}

func newBlockingStreamPriorityQ(size uint32) *blockingStreamPriorityQ {
	return &blockingStreamPriorityQ{
		q: make(streamPriorityQ, 0, size),
		s: newSemaphore(0, size),
	}
}

func (bq *blockingStreamPriorityQ) Push(stream *stream) {
	bq.s.IncLock()
	defer bq.s.Unlock()
	heap.Push(&bq.q, stream)
}

func (bq *blockingStreamPriorityQ) Pop() *stream {
	bq.s.DecLock()
	defer bq.s.Unlock()
	return heap.Pop(&bq.q).(*stream)
}

const maxFramePriority = 0xFF

type frameWithPriority struct {
	Priority byte   // The priority of the belonging stream or maxFramePriority for immediately.
	Seq      uint32 // Sequence number of this frame.
	Frame    framing.Frame
}

type framePriorityQ []*frameWithPriority

func (q framePriorityQ) Len() int {
	return len(q)
}

func (q framePriorityQ) Less(i, j int) bool {
	if q[i].Priority == q[j].Priority {
		return q[i].Seq < q[j].Seq
	}
	return q[i].Priority > q[j].Priority
}

func (q framePriorityQ) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *framePriorityQ) Push(x interface{}) {
	*q = append(*q, x.(*frameWithPriority))
}

func (q *framePriorityQ) Pop() (item interface{}) {
	last := len(*q) - 1
	item = (*q)[last]
	*q = (*q)[:last]
	return
}

type blockingFramePriorityQ struct {
	q framePriorityQ
	s *semaphore
}

func newBlockingFamePriorityQQ(size uint32) *blockingFramePriorityQ {
	return &blockingFramePriorityQ{
		q: make(framePriorityQ, 0, size),
		s: newSemaphore(0, size),
	}
}

func (bq *blockingFramePriorityQ) Push(stream *frameWithPriority) {
	bq.s.IncLock()
	defer bq.s.Unlock()
	heap.Push(&bq.q, stream)
}

func (bq *blockingFramePriorityQ) Pop() *frameWithPriority {
	bq.s.DecLock()
	defer bq.s.Unlock()
	return heap.Pop(&bq.q).(*frameWithPriority)
}
