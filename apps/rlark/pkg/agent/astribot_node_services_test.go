package agent

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A node's service on its Unix socket is reached on the Agent's local server, its path and body as they came.
func TestANodeServiceIsReachedOnItsSocket(t *testing.T) {
	directory, err := os.MkdirTemp("", "services")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	defer func(was string) { nodeServices = was }(nodeServices)
	nodeServices = directory
	listener, err := net.Listen("unix", directory+"/s1u-power.sock")
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(r.Method + " " + r.URL.Path + " " + r.Header.Get("X-Remote-User") + " " + string(body)))
	}))
	defer listener.Close()

	agent := httptest.NewServer(NewAgent(DefaultConfig()).localHTTPRouter())
	defer agent.Close()
	request, _ := http.NewRequest(http.MethodPost, agent.URL+"/api/astribot/services/s1u-power/pose", strings.NewReader(`{"action":"hold"}`))
	request.Header.Set("X-Remote-User", "alice")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusConflict || string(body) != `POST /pose alice {"action":"hold"}` {
		t.Fatalf("%d %s", response.StatusCode, body)
	}
	response, err = http.Get(agent.URL + "/api/astribot/services/s1u-video/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("a service the node does not run: %d", response.StatusCode)
	}
}
