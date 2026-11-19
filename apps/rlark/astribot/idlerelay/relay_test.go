package idlerelay

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// A target that sends a little and then stays silent with the connection open (the KS3 read and the frozen
// upstream stream of 11-14): the relay closes the tunnel after the idle time; a target that keeps talking is
// relayed in full.
func TestRelayClosesASilentTunnelAndPassesALiveOne(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		for {
			conn, err := target.Accept()
			if err != nil {
				return
			}
			go func() {
				line, _ := bufio.NewReader(conn).ReadString('\n')
				if line == "silent\n" {
					conn.Write([]byte("partial"))
					time.Sleep(time.Minute)
					return
				}
				for i := 0; i < 5; i++ {
					conn.Write([]byte("chunk"))
					time.Sleep(300 * time.Millisecond)
				}
				conn.Close()
			}()
		}
	}()

	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, proxy, 1*time.Second, nil)

	through := func(first string) (string, time.Duration) {
		conn, err := net.Dial("tcp", proxy.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conn.Write([]byte("CONNECT " + target.Addr().String() + " HTTP/1.1\r\nHost: " + target.Addr().String() + "\r\n\r\n"))
		reader := bufio.NewReader(conn)
		response, err := http.ReadResponse(reader, nil)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("CONNECT: %v %v", response, err)
		}
		start := time.Now()
		conn.Write([]byte(first))
		body, _ := io.ReadAll(reader)
		return string(body), time.Since(start)
	}

	if body, took := through("silent\n"); body != "partial" || took > 4*time.Second {
		t.Fatalf("silent target: got %q after %v, want the partial reply and a close after about 1s", body, took)
	}
	if body, _ := through("live\n"); body != "chunkchunkchunkchunkchunk" {
		t.Fatalf("live target: got %q", body)
	}
}

// A relay with a list of allowed targets (a site's way out to its package indexes) tunnels to those hosts
// and the names under an allowed domain, and answers 403 for any other.
func TestRelayRefusesATargetThatIsNotAllowed(t *testing.T) {
	allow := []string{"repo.example.com", ".pythonhosted.org", "127.0.0.1"}
	for target, want := range map[string]bool{
		"repo.example.com:443": true, "files.pythonhosted.org:443": true, "pythonhosted.org:443": true,
		"evil-pythonhosted.org:443": false, "repo.example.com.evil.net:443": false, "192.168.31.1:80": false, "no-port": false,
	} {
		if got := Allowed(allow, target); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", target, got, want)
		}
	}
	if !Allowed(nil, "anything.example:443") {
		t.Error("an empty list allows every target")
	}

	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, proxy, time.Second, allow)
	status := func(host string) int {
		conn, err := net.Dial("tcp", proxy.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conn.Write([]byte("CONNECT " + host + " HTTP/1.1\r\nHost: " + host + "\r\n\r\n"))
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}
	if code := status(target.Addr().String()); code != http.StatusOK {
		t.Errorf("allowed target: %d", code)
	}
	if code := status("localhost:" + target.Addr().String()[len("127.0.0.1:"):]); code != http.StatusForbidden {
		t.Errorf("target outside the list: %d", code)
	}
}

func TestInterfaceAddressIsTheInterfacesIPv4(t *testing.T) {
	if address, err := InterfaceAddress("lo"); err != nil || address != "127.0.0.1" {
		t.Errorf("lo: %q, %v", address, err)
	}
	if _, err := InterfaceAddress("no-such-interface"); err == nil {
		t.Error("an interface that does not exist")
	}
}
