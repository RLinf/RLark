package reconcile

import (
	"sync"
	"testing"
	"time"
)

func TestKeyedLocksSerializesSameKey(t *testing.T) {
	var locks KeyedLocks

	release := locks.Lock("a")

	acquired := make(chan struct{})
	go func() {
		defer close(acquired)
		defer locks.Lock("a")()
	}()

	select {
	case <-acquired:
		t.Fatal("second holder acquired the same key while it was held")
	case <-time.After(50 * time.Millisecond):
	}

	release()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second holder did not acquire the key after release")
	}
}

func TestKeyedLocksDoesNotSerializeDistinctKeys(t *testing.T) {
	var locks KeyedLocks

	release := locks.Lock("a")
	defer release()

	acquired := make(chan struct{})
	go func() {
		defer close(acquired)
		defer locks.Lock("b")()
	}()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("a distinct key was blocked behind an unrelated key")
	}
}

func TestKeyedLocksReleasesEntriesWhenLastHolderUnlocks(t *testing.T) {
	var locks KeyedLocks

	const holders = 16
	var wg sync.WaitGroup
	wg.Add(holders)
	for range holders {
		go func() {
			defer wg.Done()
			defer locks.Lock("a")()
		}()
	}
	wg.Wait()

	locks.mu.Lock()
	remaining := len(locks.entries)
	locks.mu.Unlock()

	if remaining != 0 {
		t.Fatalf("expected no retained entries, got %d", remaining)
	}
}
