package remotedialer

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestSession_sessionKeys(t *testing.T) {
	t.Parallel()

	s := NewServerSession(rand.Int63(), "", nil)

	clientKey, sessionKey := "testkey", rand.Int()
	s.addSessionKey(clientKey, sessionKey)
	if got, want := len(s.remoteClientKeys), 1; got != want {
		t.Errorf("incorrect number of remote client keys, got: %d, want %d", got, want)
	}

	if got, want := s.getSessionKeys(clientKey), map[int]bool{sessionKey: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("incorrect result from getSessionKeys, got: %v, want %v", got, want)
	}

	s.removeSessionKey(clientKey, sessionKey)
	if got, want := len(s.remoteClientKeys), 0; got != want {
		t.Errorf("incorrect number of remote client keys after removal, got: %d, want %d", got, want)
	}
}

func TestSession_addRemoveRemoteClient(t *testing.T) {
	t.Parallel()

	s := NewServerSession(rand.Int63(), "", nil)
	clientKey, sessionKey := "test", rand.Int()

	msgAddress := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	if err := s.addRemoteClient(msgAddress); err != nil {
		t.Fatal(err)
	}
	if got, want := s.getSessionKeys(clientKey), map[int]bool{sessionKey: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("remote client session was not added correctly, got %v, want %v", got, want)
	}

	if err := s.removeRemoteClient(msgAddress); err != nil {
		t.Fatal(err)
	}
	if got, want := s.getSessionKeys(clientKey), 0; len(got) != want {
		t.Errorf("remote client session was not removed correctly, got %v, want len(%d)", got, want)
	}
}

func TestConnectHeaderRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		proto, address string
	}{
		{"tcp", "127.0.0.1:8080"},
		{"tcp", strings.Repeat("h", 200) + ":65500"},
		{"unix", "/var/run/socket"},
	}
	for _, tt := range tests {
		var buf strings.Builder
		if err := writeConnectHeader(&buf, tt.proto, tt.address); err != nil {
			t.Fatal(err)
		}
		proto, address, err := readConnectHeader(strings.NewReader(buf.String()))
		if err != nil {
			t.Fatal(err)
		}
		if proto != tt.proto || address != tt.address {
			t.Errorf("round trip mismatch, got %q/%q want %q/%q", proto, address, tt.proto, tt.address)
		}
	}
}

func TestReadConnectHeaderDoesNotOverread(t *testing.T) {
	t.Parallel()

	payload := "tcp/127.0.0.1:80\nHELLO PAYLOAD"
	r := strings.NewReader(payload)
	proto, address, err := readConnectHeader(r)
	if err != nil {
		t.Fatal(err)
	}
	if proto != "tcp" || address != "127.0.0.1:80" {
		t.Fatalf("unexpected header %q/%q", proto, address)
	}

	rest := make([]byte, len("HELLO PAYLOAD"))
	if _, err := r.Read(rest); err != nil {
		t.Fatal(err)
	}
	if got, want := string(rest), "HELLO PAYLOAD"; got != want {
		t.Errorf("payload consumed incorrectly, got %q want %q", got, want)
	}
}
