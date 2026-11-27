package main

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestTheSiteTimeIsAnsweredToClientsOnly(t *testing.T) {
	request := make([]byte, 48)
	request[0] = 0x23 // version 4, client
	binary.BigEndian.PutUint64(request[40:48], 0x1122334455667788)
	received := time.Unix(1795600000, 500_000_000)
	reply := ntpReply(request, received, received.Add(time.Millisecond), true)
	if reply[0] != 0x24 || reply[1] != stratumSynced || binary.BigEndian.Uint64(reply[24:32]) != 0x1122334455667788 {
		t.Fatalf("a server's answer with the client's version and its transmit time: % x", reply[:32])
	}
	if seconds := binary.BigEndian.Uint32(reply[32:36]); seconds != 1795600000+ntpEpoch || binary.BigEndian.Uint32(reply[36:40]) != 1<<31 {
		t.Fatalf("received at %d.%d", seconds, binary.BigEndian.Uint32(reply[36:40]))
	}
	if ntpReply(request, received, received, false)[1] != stratumLocal {
		t.Fatal("a hub off public time answers as the site's local time")
	}
	server := append([]byte{}, request...)
	server[0] = 0x24
	if ntpReply(server, received, received, true) != nil || ntpReply(request[:20], received, received, true) != nil {
		t.Fatal("only a client's full request is answered")
	}
}

func TestTheTimeIsServedOnTheTunnelAddress(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go serveTime(conn)
	client, err := net.DialUDP("udp", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	request := make([]byte, 48)
	request[0] = 0x23
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	reply := make([]byte, 64)
	n, err := client.Read(reply)
	served := time.Unix(int64(binary.BigEndian.Uint32(reply[40:44]))-ntpEpoch, 0)
	if err != nil || n != 48 || time.Since(served).Abs() > 2*time.Second {
		t.Fatalf("answer: %d bytes, %v, served %s", n, err, served)
	}
}
