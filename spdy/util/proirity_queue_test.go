package util

import (
	"container/heap"
	"math/rand"
	"strconv"
	"testing"
	"time"
)

type Item struct {
	Priority int
	Message  string
}

func (i *Item) TakePrecedenceOver(other PriorityItem) bool {
	return i.Priority < other.(*Item).Priority
}

func TestPriorityQ(t *testing.T) {
	var q = &priorityQueue{}
	heap.Init(q)
	heap.Push(q, &Item{3, "Three"})
	heap.Push(q, &Item{6, "Six"})
	heap.Push(q, &Item{1, "One"})

	var head *Item
	head = heap.Pop(q).(*Item)
	if head.Priority != 6 || head.Message != "Six" {
		t.Fatalf("%v", *head)
	}
	head = heap.Pop(q).(*Item)
	if head.Priority != 3 || head.Message != "Three" {
		t.Fatalf("%v", *head)
	}
	head = heap.Pop(q).(*Item)
	if head.Priority != 1 || head.Message != "One" {
		t.Fatalf("%v", *head)
	}
}

func TestBlockingStreamPriorityQ(t *testing.T) {
	var bq = NewBlockingPriorityQueue(10)

	go func() {
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&Item{1, "1"})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&Item{2, "2"})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&Item{5, "5"})
	}()

	go func() {
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&Item{1, "11"})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&Item{3, "13"})
		time.Sleep(time.Microsecond * time.Duration(rand.Int63n(10)))
		bq.Push(&Item{7, "17"})
	}()
	time.Sleep(time.Millisecond * 100)
	var last *Item
	for i := 0; i < 6; i++ {
		s := bq.Pop().(*Item)
		if last != nil {
			if s.Priority > last.Priority ||
				s.Message != strconv.Itoa(s.Priority) && s.Message != "1"+strconv.Itoa(s.Priority) {
				t.Fatal()
			}
		}
		last = s
	}
}
