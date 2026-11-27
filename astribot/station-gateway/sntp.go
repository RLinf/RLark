package main

import (
	"encoding/binary"
	"log"
	"net"
	"strconv"
	"syscall"
	"time"
)

// The site's time, served by every gateway on its tunnel address (SNTP,
// RFC 4330, udp/123): a site's robots hold their clocks to a hub's (the
// s1u-robot Addon's timeSource names one), which keeps its own system clock
// to public time with whatever the machine runs for that. A hub cut off from
// public time still answers, with stratum 10, so that the site keeps one time.
const (
	ntpPort       = 123
	ntpEpoch      = 2208988800 // seconds from 1900 to 1970
	stratumSynced = 2
	stratumLocal  = 10
	staUnsync     = 0x0040 // adjtimex status: the kernel's clock is not synchronised
)

func ntpTime(t time.Time) uint64 {
	return uint64(t.Unix()+ntpEpoch)<<32 | uint64(t.Nanosecond())<<32/1e9
}

// ntpReply answers one client request (mode 3) received at received, or nil
// for anything else.
func ntpReply(request []byte, received, sent time.Time, synced bool) []byte {
	if len(request) < 48 || request[0]&7 != 3 {
		return nil
	}
	reply := make([]byte, 48)
	stratum := byte(stratumSynced)
	if !synced {
		stratum = stratumLocal
	}
	reply[0] = request[0]&0x38 | 4                           // no leap second, the client's version, server mode
	reply[1], reply[2], reply[3] = stratum, request[2], 0xEC // poll as asked, precision about 60 ns
	copy(reply[12:16], "SITE")
	binary.BigEndian.PutUint64(reply[16:24], ntpTime(received))
	copy(reply[24:32], request[40:48]) // the client's transmit time, as the originate time
	binary.BigEndian.PutUint64(reply[32:40], ntpTime(received))
	binary.BigEndian.PutUint64(reply[40:48], ntpTime(sent))
	return reply
}

func clockSynced() bool {
	var timex syscall.Timex
	state, err := syscall.Adjtimex(&timex)
	return err == nil && state != 5 /* TIME_ERROR */ && timex.Status&staUnsync == 0
}

func serveTime(conn *net.UDPConn) {
	buffer := make([]byte, 512)
	for {
		n, client, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		received := time.Now()
		if reply := ntpReply(buffer[:n], received, time.Now(), clockSynced()); reply != nil {
			_, _ = conn.WriteToUDP(reply, client)
		}
	}
}

// timeService keeps the time served on the node's tunnel address as the
// configuration names it, every 30 s.
func timeService(read func() *desired) {
	var conn *net.UDPConn
	said := ""
	for ; ; time.Sleep(30 * time.Second) {
		at := ""
		if d := read(); d != nil {
			at = net.JoinHostPort(d.address.IP.String(), strconv.Itoa(ntpPort))
		}
		if conn != nil && conn.LocalAddr().String() == at {
			continue
		}
		if conn != nil {
			_ = conn.Close()
			conn = nil
		}
		state := "no tunnel address yet"
		if at != "" {
			address, _ := net.ResolveUDPAddr("udp", at)
			opened, err := net.ListenUDP("udp", address)
			if err != nil {
				state = "not serving the time at " + at + ": " + err.Error()
			} else {
				conn, state = opened, "serving the time at "+at
				go serveTime(opened)
			}
		}
		if state != said {
			said = state
			log.Print(state)
		}
	}
}
