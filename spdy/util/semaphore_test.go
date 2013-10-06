package util

import (
	"reflect"
	"testing"
	"time"
)

func TestSemaphoreEmpty(t *testing.T) {
	var c = make(chan int, 8)

	var s = newSemaphore(1, 4)
	go func() {
		time.Sleep(time.Millisecond * 100)
		s.IncLock()
		c <- 1
		s.Unlock()
	}()

	c <- 2
	s.DecLock()
	c <- 4
	s.Unlock()
	c <- 6
	s.DecLock()
	c <- 8
	s.Unlock()
	c <- 10

	result := make([]int, 6)
	for i, _ := range result {
		result[i] = <-c
	}

	if !reflect.DeepEqual(result, []int{2, 4, 6, 1, 8, 10}) {
		t.Fatalf("%v", result)
	}
}

func TestSemaphoreFull(t *testing.T) {
	var c = make(chan int, 8)

	var s = newSemaphore(0, 2)
	go func() {
		time.Sleep(time.Millisecond * 100)
		s.DecLock()
		c <- 1
		s.Unlock()
	}()

	s.IncLock()
	c <- 2
	s.Unlock()
	s.IncLock()
	c <- 4
	s.Unlock()
	c <- 6
	s.IncLock()
	c <- 8
	s.Unlock()

	result := make([]int, 5)
	for i, _ := range result {
		result[i] = <-c
	}

	if !reflect.DeepEqual(result, []int{2, 4, 6, 1, 8}) {
		t.Fatalf("%v", result)
	}
}

func BenchmarkSemaphore(b *testing.B) {
	var s = newSemaphore(1, 0xFFFFFFFF)
	for i := 0; i < b.N; i++ {
		s.IncLock()
		s.Unlock()
		s.DecLock()
		s.Unlock()
	}
}
