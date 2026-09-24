package nodeserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

type testPodCred struct{}

func (testPodCred) IP() string          { return "10.0.0.1" }
func (testPodCred) IPPrefixLength() int { return 24 }

func TestWatchHostsReturnsWhenVersionChanges(t *testing.T) {
	var mu sync.RWMutex
	hosts := map[string]string{"pod-a": "10.0.0.1"}
	s := NewNodeServer(
		DefaultConfig(),
		func(context.Context, int32) (testPodCred, error) { return testPodCred{}, nil },
		nil,
		func(context.Context, testPodCred) (map[string]string, error) {
			mu.RLock()
			defer mu.RUnlock()
			copy := make(map[string]string, len(hosts))
			for k, v := range hosts {
				copy[k] = v
			}
			return copy, nil
		},
		func(testPodCred) (string, error) { return "pod", nil },
		func(string) (testPodCred, error) { return testPodCred{}, nil },
	)

	server := httptest.NewServer(s.localServiceRouter())
	defer server.Close()
	initialVersion := hostsVersion(hosts)

	result := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get(server.URL + "/watch_hosts?version=" + url.QueryEscape(initialVersion))
		if err != nil {
			result <- nil
			return
		}
		result <- resp
	}()

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	hosts = map[string]string{"pod-a": "10.0.0.2"}
	mu.Unlock()

	select {
	case resp := <-result:
		if resp == nil {
			t.Fatal("watch request failed")
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var got map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["pod-a"] != "10.0.0.2" {
			t.Fatalf("hosts = %v", got)
		}
		if resp.Header.Get("ETag") == initialVersion {
			t.Fatal("ETag did not change")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not return after hosts changed")
	}
}
