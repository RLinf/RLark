//go:build darwin

package nodeserver

import (
	"fmt"
	"net"
	"syscall"
)

// SOL_LOCAL is a constant value.
const SOL_LOCAL = 0

// LOCAL_PEERPID is a constant value.
const LOCAL_PEERPID = 0x002

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
	var ret int
	ctrlErr := rawConn.Control(func(fd uintptr) {
		// 2. 通过 SO_PEERCRED 获取对端凭证
		ret, err = syscall.GetsockoptInt(
			int(fd),
			SOL_LOCAL,
			LOCAL_PEERPID,
		)
	})
	if ctrlErr != nil {
		return 0, fmt.Errorf("call control: %w", ctrlErr)
	}
	if err != nil {
		return 0, fmt.Errorf("get sockopt int: %w", err)
	}
	return int32(ret), nil
}
