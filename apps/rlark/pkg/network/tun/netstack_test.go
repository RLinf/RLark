package tun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type fakeTunDevice struct {
	mu     sync.Mutex
	writes [][]byte
	err    error
}

func (f *fakeTunDevice) Read([]byte) (int, error) { return 0, errors.New("closed") }
func (f *fakeTunDevice) Close() error             { return nil }
func (f *fakeTunDevice) Write(data []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, append([]byte(nil), data...))
	return len(data), f.err
}

func TestHandleSendWritesPacketAndStopsOnCancel(t *testing.T) {
	ep := channel.New(1, 1500, "")
	ctx, cancel := context.WithCancel(context.Background())
	device := &fakeTunDevice{}
	ns := &netstack{}
	done := make(chan error, 1)
	go func() { done <- ns.handleSend(ctx, ep, device) }()

	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData([]byte{1, 2, 3, 4}),
	})
	var packets stack.PacketBufferList
	packets.PushBack(pkt)
	if n, err := ep.WritePackets(packets); err != nil || n != 1 {
		t.Fatalf("WritePackets() = (%d, %v), want (1, nil)", n, err)
	}
	pkt.DecRef()

	deadline := time.Now().Add(time.Second)
	for {
		device.mu.Lock()
		written := len(device.writes)
		device.mu.Unlock()
		if written == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("packet was not written to TUN")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleSend did not stop after cancellation")
	}

	device.mu.Lock()
	defer device.mu.Unlock()
	if got := device.writes[0]; len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("unexpected TUN packet: %v", got)
	}
}
