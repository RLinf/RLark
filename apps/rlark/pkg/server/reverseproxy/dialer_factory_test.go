package reverseproxy

import (
	"sync"
	"testing"
)

func TestDialerFactoryTryAcceptTracksPendingConnections(t *testing.T) {
	t.Parallel()

	f := NewDialerFactory()
	const clientKey = "client"

	if err := f.tryAccept(clientKey); err != nil {
		t.Fatalf("first connection was rejected: %v", err)
	}
	for i := 0; i < maxRejections; i++ {
		if err := f.tryAccept(clientKey); err == nil {
			t.Fatalf("connection %d was accepted before rejection threshold", i+2)
		}
	}
	if err := f.tryAccept(clientKey); err != nil {
		t.Fatalf("fallback connection was rejected: %v", err)
	}

	f.disconnect(clientKey)
	f.disconnect(clientKey)
	if _, ok := f.clients[clientKey]; ok {
		t.Fatal("client state remained after all connections disconnected")
	}
}

func TestDialerFactoryTryAcceptConcurrent(t *testing.T) {
	t.Parallel()

	f := NewDialerFactory()
	const attempts = 32

	var wg sync.WaitGroup
	results := make(chan bool, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- f.tryAccept("client") == nil
		}()
	}
	wg.Wait()
	close(results)

	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	want := 1 + (attempts-1)/(maxRejections+1)
	if accepted != want {
		t.Fatalf("accepted %d concurrent connections, want %d", accepted, want)
	}
}
