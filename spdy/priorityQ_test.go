package spdy

import (
	"container/heap"
	"math/rand"
	"testing"
	"time"
)

func TestStreamPriorityQ(t *testing.T) {
	var q = &streamPriorityQ{}
	heap.Push(q, &stream{Priority: 3, ID: 3})
	heap.Push(q, &stream{Priority: 6, ID: 6})
	heap.Push(q, &stream{Priority: 1, ID: 1})

	head := heap.Pop(q).(*stream)
	if head.Priority != 6 || head.ID != 6 {
		t.Fatalf("%v", *head)
	}
}

func TestBlockingStreamPriorityQ(t *testing.T) {
	var bq = newBlockingStreamPriorityQ(10)

	go func() {
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&stream{Priority: 1, ID: 1})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&stream{Priority: 2, ID: 2})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&stream{Priority: 5, ID: 5})
	}()

	go func() {
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&stream{Priority: 1, ID: 11})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&stream{Priority: 3, ID: 13})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&stream{Priority: 7, ID: 17})
	}()
	time.Sleep(time.Millisecond * 100)
	var last *stream
	for i := 0; i < 6; i++ {
		s := bq.Pop()
		if last != nil {
			if s.Priority > last.Priority ||
				s.ID != uint32(s.Priority) && s.ID != uint32(s.Priority)+10 {
				t.Fatal()
			}
		}
		last = s
	}
}
