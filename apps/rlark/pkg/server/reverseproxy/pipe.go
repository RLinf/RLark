package reverseproxy

import (
	"io"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/xjasonlyu/tun2socks/v2/buffer"
)

const (
	// tcpWaitTimeout implements a TCP half-close timeout.
	tcpWaitTimeout = 60 * time.Second
)

// PipeConnections pipes bidirectional data between origin and remote connections.
func PipeConnections(origin, remote io.ReadWriteCloser) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	go unidirectionalStream(remote, origin, "origin->remote", &wg)
	go unidirectionalStream(origin, remote, "remote->origin", &wg)

	wg.Wait()
}

func unidirectionalStream(dst, src io.ReadWriteCloser, dir string, wg *sync.WaitGroup) {
	logger := log.GetLogger()
	defer wg.Done()
	buf := buffer.Get(buffer.RelayBufferSize)
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		logger.V(1).Info("[TCP] copy data", "dir", dir, "err", err)
	}
	if err := buffer.Put(buf); err != nil {
		logger.V(1).Info("[TCP] put buffer", "dir", dir, "err", err)
	}
	// Do the upload/download side TCP half-close.
	if cr, ok := src.(interface{ CloseRead() error }); ok {
		if err := cr.CloseRead(); err != nil {
			logger.V(1).Info("[TCP] close read", "dir", dir, "err", err)
		}
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		if err := cw.CloseWrite(); err != nil {
			logger.V(1).Info("[TCP] close write", "dir", dir, "err", err)
		}
	}
	// Set TCP half-close timeout.
	if srd, ok := dst.(interface{ SetReadDeadline(time.Time) error }); ok {
		if err := srd.SetReadDeadline(time.Now().Add(tcpWaitTimeout)); err != nil {
			logger.V(1).Info("[TCP] set read deadline", "dir", dir, "err", err)
		}
	}
}
