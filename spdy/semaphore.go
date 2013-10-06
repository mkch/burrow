package spdy

import (
	"sync"
)

type semaphore struct {
	l        sync.Mutex
	notFull  sync.Cond
	notEmpty sync.Cond
	value    uint32
	maxValue uint32
}

func newSemaphore(initVlaue, maxValue uint32) *semaphore {
	if maxValue == 0 {
		panic("maxValue must > 0")
	}
	if initVlaue > maxValue {
		panic("initValue must <= maxValue")
	}

	s := &semaphore{
		value:    initVlaue,
		maxValue: maxValue,
	}
	s.notEmpty.L = &s.l
	s.notFull.L = &s.l
	return s
}

func (s *semaphore) IncLock() {
	s.l.Lock()
	if s.value == s.maxValue {
		for s.value == s.maxValue {
			s.notFull.Wait()
		}
	}
	s.value++
	s.notEmpty.Signal()
}

func (s *semaphore) DecLock() {
	s.l.Lock()
	if s.value == 0 {
		for s.value == 0 {
			s.notEmpty.Wait()
		}
	}
	s.value--
	s.notFull.Signal()
}

func (s *semaphore) Unlock() {
	s.l.Unlock()
}
