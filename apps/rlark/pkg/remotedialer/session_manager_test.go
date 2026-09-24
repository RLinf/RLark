package remotedialer

import "testing"

func TestSessionManagerGetDialerPrefix(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "agent:node-agent:"
		clientKey = prefix + "node-a"
	)

	t.Run("direct client", func(t *testing.T) {
		t.Parallel()

		sm := newSessionManager()
		direct := &Session{}
		sm.clients[clientKey] = []*Session{direct}

		session, routeKey, err := sm.findSession(prefix + "*")
		if err != nil {
			t.Fatalf("expected prefix to match direct client: %v", err)
		}
		if session != direct {
			t.Fatal("selected unexpected direct session")
		}
		if routeKey != "" {
			t.Fatalf("direct route key = %q, want empty", routeKey)
		}
	})

	t.Run("peer client", func(t *testing.T) {
		t.Parallel()

		peer := &Session{
			remoteClientKeys: map[string]map[int]bool{
				clientKey: {1: true},
			},
		}
		sm := newSessionManager()
		sm.peers["peer"] = []*Session{peer}

		session, routeKey, err := sm.findSession(prefix + "*")
		if err != nil {
			t.Fatalf("expected prefix to match peer client: %v", err)
		}
		if session != peer {
			t.Fatal("selected unexpected peer session")
		}
		if routeKey != clientKey {
			t.Fatalf("peer route key = %q, want %q", routeKey, clientKey)
		}
	})
}
