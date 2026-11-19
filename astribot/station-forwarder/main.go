// station-forwarder maintains a fixed, narrowly scoped host firewall rule set.
// Existing rules are never deleted; changes require explicit operator cleanup.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type config struct {
	Input       string `json:"input"`
	Output      string `json:"output"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	SNAT        string `json:"snat"`
	Port        int    `json:"port"`
}
type rule struct {
	table, chain string
	args         []string
	insert       bool
}

func parse(b []byte) (config, error) {
	var c config
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, errors.New("expected one JSON object")
	}
	for _, s := range []string{c.Input, c.Output} {
		if s == "" || len(s) > 15 || strings.ContainsAny(s, "/ \t\n") {
			return c, errors.New("invalid interface")
		}
	}
	if c.Input == c.Output {
		return c, errors.New("input and output must differ")
	}
	for _, s := range []string{c.Source, c.Destination, c.SNAT} {
		a, err := netip.ParseAddr(s)
		if err != nil || !a.Is4() || a.IsUnspecified() || a.IsMulticast() || a.IsLoopback() {
			return c, errors.New("addresses must be unicast IPv4 host addresses")
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return c, errors.New("invalid TCP port")
	}
	return c, nil
}
func rules(c config) []rule {
	port := strconv.Itoa(c.Port)
	tag := []string{"-m", "comment", "--comment", "rlark-station-forwarder-v1"}
	build := func(a []string) []string { return append(a, tag...) }
	return []rule{
		{"filter", "FORWARD", append(build([]string{"-i", c.Input, "-o", c.Output, "-s", c.Source + "/32", "-d", c.Destination + "/32", "-p", "tcp", "--dport", port}), "-j", "ACCEPT"), true},
		{"filter", "FORWARD", append(build([]string{"-i", c.Output, "-o", c.Input, "-s", c.Destination + "/32", "-d", c.Source + "/32", "-p", "tcp", "--sport", port, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED"}), "-j", "ACCEPT"), true},
		{"nat", "POSTROUTING", append(build([]string{"-o", c.Output, "-s", c.Source + "/32", "-d", c.Destination + "/32", "-p", "tcp", "--dport", port}), "-j", "SNAT", "--to-source", c.SNAT), false},
	}
}

type executor func(context.Context, ...string) ([]byte, error)

func observeHost(c config) error {
	forwarding, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return fmt.Errorf("read IPv4 forwarding: %w", err)
	}
	if strings.TrimSpace(string(forwarding)) != "1" {
		return errors.New("IPv4 forwarding is disabled")
	}
	if _, err := net.InterfaceByName(c.Input); err != nil {
		return fmt.Errorf("input interface: %w", err)
	}
	out, err := net.InterfaceByName(c.Output)
	if err != nil {
		return fmt.Errorf("output interface: %w", err)
	}
	addresses, err := out.Addrs()
	if err != nil {
		return fmt.Errorf("output addresses: %w", err)
	}
	for _, a := range addresses {
		prefix, err := netip.ParsePrefix(a.String())
		if err == nil && prefix.Addr().String() == c.SNAT {
			return nil
		}
	}
	return fmt.Errorf("SNAT address %s is not assigned to %s", c.SNAT, c.Output)
}

func command(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "iptables", args...).CombinedOutput()
}
func configDigest(c config) string {
	canonical, _ := json.Marshal(c)
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]) + "\n"
}
func verifyPinned(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(data) != expected {
		return errors.New("forwarding configuration changed; remove old rules and state via independent management before reconfiguration")
	}
	return nil
}
func pinConfig(path, expected string) error {
	if err := verifyPinned(path, expected); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return verifyPinned(path, expected)
	}
	if err != nil {
		return err
	}
	if _, err = f.WriteString(expected); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
func apply(ctx context.Context, c config, run executor) error {
	for _, r := range rules(c) {
		args := append([]string{"-w", "5", "-t", r.table, "-C", r.chain}, r.args...)
		out, err := run(ctx, args...)
		if err == nil {
			continue
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || !(len(out) == 0 || strings.Contains(string(out), "Bad rule")) {
			return fmt.Errorf("check %s/%s rule: %w: %s", r.table, r.chain, err, strings.TrimSpace(string(out)))
		}
		op := "-A"
		if r.insert {
			op = "-I"
		}
		args = append([]string{"-w", "5", "-t", r.table, op, r.chain}, r.args...)
		if out, err = run(ctx, args...); err != nil {
			return fmt.Errorf("install %s/%s rule: %w: %s", r.table, r.chain, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func check(ctx context.Context, c config, run executor) error {
	if err := observeHost(c); err != nil {
		return err
	}
	for _, r := range rules(c) {
		args := append([]string{"-w", "5", "-t", r.table, "-C", r.chain}, r.args...)
		if out, err := run(ctx, args...); err != nil {
			return fmt.Errorf("check %s/%s rule: %w: %s", r.table, r.chain, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
func main() {
	file := flag.String("config", "/config/config.json", "immutable forwarding configuration")
	checkOnly := flag.Bool("check", false, "check prerequisites and exact rules without modifying them")
	flag.Parse()
	data, err := os.ReadFile(*file)
	if err != nil {
		log.Fatal(err)
	}
	c, err := parse(data)
	if err != nil {
		log.Fatal(err)
	}
	statePath := "/var/lib/rlark-station-forwarder/config.sha256"
	if *checkOnly {
		if err = verifyPinned(statePath, configDigest(c)); err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err = check(ctx, c, command); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err = pinConfig(statePath, configDigest(c)); err != nil {
		log.Fatal(err)
	}
	// A fixed configuration avoids leaving previously installed rules behind when
	// a ConfigMap is edited. Updating requires explicit removal and Pod restart.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		err := observeHost(c)
		if err == nil {
			err = apply(ctx, c, command)
		}
		if err != nil {
			log.Printf("forwarding reconcile failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
