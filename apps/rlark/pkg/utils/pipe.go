package utils

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/xjasonlyu/tun2socks/v2/buffer"
)

const (
	// StreamDrainTimeout is the read deadline applied while waiting for the
	// opposite copy direction to finish after one direction has completed.
	StreamDrainTimeout = 60 * time.Second
)

// CopyStream copies data from src to dst using a pooled relay buffer.
func CopyStream(dst, src io.ReadWriteCloser) error {
	var ret error
	buf := buffer.Get(buffer.RelayBufferSize)
	if buf == nil {
		_, ret = io.Copy(dst, src)
	} else {
		defer func() { _ = buffer.Put(buf) }()
		_, ret = io.CopyBuffer(dst, src, buf)
	}
	return ret
}

func halfCloseStream(dst, src io.ReadWriteCloser, direction string, logger *logr.Logger) {
	if cr, ok := src.(interface{ CloseRead() error }); ok {
		if err := cr.CloseRead(); err != nil && logger != nil {
			logger.V(1).Info("[PIPE] close read", "dir", direction, "err", err)
		}
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		if err := cw.CloseWrite(); err != nil && logger != nil {
			logger.V(1).Info("[PIPE] close write", "dir", direction, "err", err)
		}
	}
}

func setStreamDrainDeadline(dst io.ReadWriteCloser, direction string, logger *logr.Logger) {
	if srd, ok := dst.(interface{ SetReadDeadline(time.Time) error }); ok {
		if err := srd.SetReadDeadline(time.Now().Add(StreamDrainTimeout)); err != nil && logger != nil {
			logger.V(1).Info("[PIPE] set the other side read deadline", "dir", direction, "err", err)
		}
	}
}

// RelayStreams copies data bidirectionally between two streams until both copy
// directions finish. When one direction finishes, it attempts to half-close
// that direction and sets a drain deadline if the destination supports read
// deadlines. Optional names identify the streams in diagnostic logs.
func RelayStreams(stream1, stream2 io.ReadWriteCloser, names ...string) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	name1, name2 := "origin", "remote"
	if len(names) >= 2 {
		name1, name2 = names[0], names[1]
	}

	logger := log.GetLogger()
	go relayStream(stream2, stream1, "from_"+name1+"_to_"+name2, &wg, &logger)
	go relayStream(stream1, stream2, "from_"+name2+"_to_"+name1, &wg, &logger)
	wg.Wait()
}

func relayStream(dst, src io.ReadWriteCloser, direction string, wg *sync.WaitGroup, logger *logr.Logger) {
	defer wg.Done()
	if err := CopyStream(dst, src); err != nil && logger != nil {
		logger.V(1).Info("[PIPE] stream copy error", "dir", direction, "err", err)
	}
	halfCloseStream(dst, src, direction, logger)
	setStreamDrainDeadline(dst, direction, logger)
}

// RelayConnections copies data bidirectionally between two network connections.
// After either direction finishes, it sets read deadlines on both connections
// and waits for the opposite direction without half-closing either connection.
// Returned errors correspond to conn1 to conn2 and conn2 to conn1,
// respectively. Optional names identify the connections in returned errors.
func RelayConnections(conn1, conn2 net.Conn, names ...string) (error, error) {
	name1, name2 := "origin", "remote"
	if len(names) >= 2 {
		name1, name2 = names[0], names[1]
	}
	errors := make([]error, 2)
	sigch := make(chan struct{}, 2)
	go func() {
		if err := CopyStream(conn2, conn1); err != nil && err != io.EOF {
			errors[0] = fmt.Errorf("copy from %s to %s: %w", name1, name2, err)
		}
		sigch <- struct{}{}
	}()
	go func() {
		if err := CopyStream(conn1, conn2); err != nil && err != io.EOF {
			errors[1] = fmt.Errorf("copy from %s to %s: %w", name2, name1, err)
		}
		sigch <- struct{}{}
	}()
	<-sigch
	_ = conn1.SetReadDeadline(time.Now().Add(StreamDrainTimeout))
	_ = conn2.SetReadDeadline(time.Now().Add(StreamDrainTimeout))
	<-sigch
	return errors[0], errors[1]
}
