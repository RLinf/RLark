//go:build linux

package nodeserver

import (
	"fmt"
	"net"
	"syscall"
)

// GetPeerProcess returns the peerProcess.
func GetPeerProcess(conn net.Conn) (int32, error) {
	// 0. 将 net.Conn 转换为 *net.UnixConn
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("conn is not a UnixConn")
	}

	// 1. 从连接获取原始文件描述符
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("syscall conn: %w", err)
	}
	var ucred *syscall.Ucred
	ctrlErr := rawConn.Control(func(fd uintptr) {
		// 2. 通过 SO_PEERCRED 获取对端凭证
		ucred, err = syscall.GetsockoptUcred(
			int(fd),
			syscall.SOL_SOCKET,
			syscall.SO_PEERCRED,
		)
	})
	if ctrlErr != nil {
		return 0, fmt.Errorf("call control: %w", ctrlErr)
	}
	if err != nil {
		return 0, fmt.Errorf("get sockopt ucred: %w", err)
	}
	return ucred.Pid, nil
}
