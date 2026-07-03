package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRepeatContinuesAfterPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0

	go func() {
		repeat(ctx, 5*time.Millisecond, "test", func(context.Context) error {
			mu.Lock()
			calls++
			call := calls
			mu.Unlock()
			if call == 1 {
				panic("intentional test panic")
			}
			if call >= 3 {
				cancel()
			}
			return nil
		})
	}()

	// Wait for repeat to process several iterations after the panic.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	got := calls
	mu.Unlock()
	if got < 3 {
		t.Fatalf("repeat did not continue after panic: calls=%d", got)
	}
}

func TestWaitWorkersReturnsTrueWhenWorkersStop(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	if !waitWorkers(&wg, time.Second) {
		t.Fatal("waitWorkers returned false for workers that stopped")
	}
}

func TestWaitWorkersReturnsFalseOnTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Done()

	if waitWorkers(&wg, 20*time.Millisecond) {
		t.Fatal("waitWorkers returned true despite timeout")
	}
}
