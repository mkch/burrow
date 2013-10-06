package util

import (
	"container/heap"
)

type PriorityItem interface {
	// Whether this item take precedence over the other item.
	TakePrecedenceOver(other PriorityItem) bool
}

type priorityQueue []PriorityItem

func (q priorityQueue) Len() int {
	return len(q)
}

func (q priorityQueue) Less(i, j int) bool {
	// "container/heap" pops the LEAST item frist.
	return !q[i].TakePrecedenceOver(q[j])
}

func (q priorityQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *priorityQueue) Push(x interface{}) {
	*q = append(*q, x.(PriorityItem))
}

func (q *priorityQueue) Pop() (item interface{}) {
	last := len(*q) - 1
	item = (*q)[last]
	*q = (*q)[:last]
	return
}

type BlockingPriorityQueue struct {
	q priorityQueue
	s *semaphore
}

func NewBlockingPriorityQueue(size uint32) *BlockingPriorityQueue {
	return &BlockingPriorityQueue{
		q: make(priorityQueue, 0, size),
		s: newSemaphore(0, size),
	}
}

func (bq *BlockingPriorityQueue) Push(item PriorityItem) {
	bq.s.IncLock()
	defer bq.s.Unlock()
	heap.Push(&bq.q, item)
}

func (bq *BlockingPriorityQueue) Pop() PriorityItem {
	bq.s.DecLock()
	defer bq.s.Unlock()
	return heap.Pop(&bq.q).(PriorityItem)
}
