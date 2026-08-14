package tun

import (
	"encoding/binary"
	"io"
	"net"
)

// MaxPacketSize 是单个 IP 包的最大字节数（含 IP 头），对应标准 IPv4 理论最大值。
const MaxPacketSize = 65535

// SendPacket 通过 TCP 连接发送一个数据包，使用 2 字节大端长度前缀进行帧封装。
//
// 格式：[2字节 totalLen][data]
// 如果 data 超过 MaxPacketSize 则截断。
func SendPacket(conn net.Conn, data []byte) error {
	if len(data) > MaxPacketSize {
		data = data[:MaxPacketSize]
	}

	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(data)))

	// 先写长度头，再写数据
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		return err
	}
	return nil
}

// RecvPacket 从 TCP 连接接收一个完整的帧封装数据包。
//
// 返回的 data 为 nil 且 err 为 nil 表示收到长度为 0 的空帧（非正常情况，向前兼容保护）。
func RecvPacket(conn net.Conn) ([]byte, error) {
	// 读 2 字节的长度头
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	size := binary.BigEndian.Uint16(header)
	if size == 0 {
		return nil, nil
	}

	// 读指定长度的数据包
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}
