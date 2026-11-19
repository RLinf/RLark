package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// keeper keeps the station-gateway a machine runs as a service of its own
// (rlark-station-gateway.service, put there once by the installer) at the
// version of the image this program was started from. It runs in the node's
// station-agent-host pod, so the image of that DaemonSet is the declaration
// for every such node of the cluster at once.
//
// Replacing the program does not touch the tunnel: the device and its sessions
// are the kernel's, the service only reconciles them. The service is told to
// end and its unit (Restart=always) starts the new file. While a new program
// is on trial the file .trial stands beside it; a program that does not stay
// up and reconcile within the wait is replaced by the one before it
// (.previous), and that version is not tried again on this node (.rejected).
// A machine that restarts during a trial takes the program before it back by
// itself, before the service starts: the tunnel may be the only way to the
// cluster, so nothing of the cluster can be needed for that. The restart may
// have had nothing to do with the program, so the version is tried once more
// (.interrupted) and rejected when its trial is cut short a second time.
type keeper struct {
	self        string // this program's own file
	target      string // the machine's program as this container sees it
	name        string // the same file as the machine's unit names it
	hold        string // a directory of the machine; a file in it says the machine is in use
	observation string // the file the service rewrites with every good pass
	dropIn      string // the directory of the unit's drop-ins
	// Where the service asks for its own document (document.go): the
	// directory of its configuration, and what is kept there.
	apiDirectory, apiURL, apiCA string
	proc                        string
	wait, stay                  time.Duration
	signal                      func(pid int) error
	sleep                       func(time.Duration)
	now                         func() time.Time
}

func newKeeper(target, name, hold, observation, dropIn string) (*keeper, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &keeper{self: self, target: target, name: name, hold: hold, observation: observation, dropIn: dropIn, proc: "/proc",
		wait: 2 * time.Minute, stay: 30 * time.Second,
		signal: func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }, sleep: time.Sleep, now: time.Now}, nil
}

// digest streams the file through the hash: the programs it is used on are
// megabytes, read every few seconds by the keeper and its readiness probe.
func digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// running lists the service's processes: pid and whether the file it was
// started from is still the one at its path.
func (k *keeper) running() (map[int]bool, error) {
	found := map[int]bool{}
	entries, err := os.ReadDir(k.proc)
	if err != nil {
		return nil, err
	}
	file, err := os.Stat(k.target)
	if err != nil {
		return nil, err
	}
	// The pod shares the machine's process namespace, and so does every
	// container on the machine: a program of the same path inside one of
	// them (a node of a cluster run in containers, say) is not the
	// machine's service. The machine's own processes are in the namespace
	// of its first process.
	own, err := os.Readlink(filepath.Join(k.proc, "1/ns/pid"))
	if err != nil {
		return nil, fmt.Errorf("see the machine's process namespace: %w", err)
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join(k.proc, entry.Name(), "cmdline"))
		if err != nil || string(bytes.SplitN(cmdline, []byte{0}, 2)[0]) != k.name {
			continue
		}
		if namespace, err := os.Readlink(filepath.Join(k.proc, entry.Name(), "ns/pid")); err != nil || namespace != own {
			continue
		}
		// The link leads to the file the process was started from, also
		// when another file has taken its path since.
		exe, err := os.Stat(filepath.Join(k.proc, entry.Name(), "exe"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("see what process %d runs: %w", pid, err)
		}
		found[pid] = os.SameFile(exe, file)
	}
	return found, nil
}

// current reports whether a process of the file at the path is running, after
// telling every process of a file that is no longer there to end.
func (k *keeper) current() (bool, error) {
	processes, err := k.running()
	if err != nil {
		return false, err
	}
	found := false
	for pid, current := range processes {
		if current {
			found = true
		} else if err := k.signal(pid); err != nil && !errors.Is(err, syscall.ESRCH) {
			return false, fmt.Errorf("end process %d: %w", pid, err)
		}
	}
	return found, nil
}

func (k *keeper) put(from string) error {
	program, err := os.Open(from)
	if err != nil {
		return err
	}
	defer program.Close()
	if err = replaceFile(k.target, program, 0755); err != nil {
		return err
	}
	_, err = k.current()
	return err
}

// reconciling reports whether the service has written its observation lately.
func (k *keeper) reconciling() bool {
	observed, err := os.Stat(k.observation)
	return err == nil && k.now().Sub(observed.ModTime()) < time.Minute
}

// guard is the unit's drop-in: a machine that starts with a trial of an
// earlier boot standing takes the program before it back, and rejects the
// version if that has happened to it before. The trial's file stays under
// another name as the record. systemd reads the drop-in at boot; this boot's
// trials are the keeper's.
func (k *keeper) guard() error {
	content := "# Written by station-gateway --keep-host-program (station-agent-host).\n[Service]\n" +
		"ExecStartPre=/bin/sh -c 'p=" + k.name + `; [ -e $$p.trial ] || exit 0; [ "$$(cut -d" " -f2 $$p.trial)" != "$$(cat /proc/sys/kernel/random/boot_id)" ] || exit 0; ` +
		`d=$$(cut -d" " -f1 $$p.trial); cp -p $$p.previous $$p.back && mv $$p.back $$p || exit 1; ` +
		// The first trial a restart cuts short is tried again; the second is the version's last.
		`if [ "$$(cat $$p.interrupted 2>/dev/null)" = "$$d" ]; then echo $$d > $$p.rejected; else echo $$d > $$p.interrupted; fi; mv $$p.trial $$p.trial-of-an-earlier-boot'` + "\n"
	path := filepath.Join(k.dropIn, "10-trial.conf")
	if had, _ := os.ReadFile(path); string(had) == content {
		return nil
	}
	return replaceFile(path, strings.NewReader(content), 0644)
}

// lay keeps station-api.json and the CA it names in the directory of the
// service's configuration, from what this pod was given: a machine installed
// before the service asked for its own document, or whose Station API has
// another address or CA since, gets them with the DaemonSet. The CA is there
// before the file that names it.
func (k *keeper) lay() error {
	ca, err := os.ReadFile(k.apiCA)
	if err != nil {
		return err
	}
	pointer, _ := json.Marshal(stationAPI{URL: k.apiURL, CA: "ca.crt"})
	for _, file := range []struct {
		name    string
		content []byte
	}{{"ca.crt", ca}, {stationAPIFile, append(pointer, '\n')}} {
		path := filepath.Join(k.apiDirectory, file.name)
		if had, _ := os.ReadFile(path); bytes.Equal(had, file.content) {
			continue
		}
		if err := replaceFile(path, bytes.NewReader(file.content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// settledState is what the pod's readiness says: no trial stands, this
// version was not rejected, and a machine that has this version runs it. A
// machine that waits for its turn (in use) is settled.
func (k *keeper) settledState() error {
	own, err := digest(k.self)
	if err != nil {
		return err
	}
	if rejected, _ := os.ReadFile(k.target + ".rejected"); strings.TrimSpace(string(rejected)) == own {
		return errors.New("this version was rejected on this machine")
	}
	if _, err := os.Stat(k.target + ".trial"); err == nil {
		return errors.New("a new program is on trial")
	}
	if failure, err := os.ReadFile(k.target + ".failure"); err == nil {
		return errors.New("the last pass failed: " + strings.TrimSpace(string(failure)))
	}
	theirs, err := digest(k.target)
	if err != nil || theirs != own {
		return err
	}
	processes, err := k.running()
	if err != nil {
		return err
	}
	for _, current := range processes {
		if current {
			return nil
		}
	}
	return errors.New("the machine has this version and does not run it")
}

// pass does one round and says what state the machine is left in.
func (k *keeper) pass() (string, error) {
	own, err := digest(k.self)
	if err != nil {
		return "", err
	}
	trialPath, previous := k.target+".trial", k.target+".previous"
	trial, _ := os.ReadFile(trialPath)
	trial = append(bytes.Fields(trial), nil)[0]
	judged := true
	switch {
	case string(trial) == own:
		// A trial this program began and did not see the end of.
		if _, err = k.current(); err != nil {
			return "", err
		}
	case len(trial) > 0:
		// A trial of another version: the program before it is the known one.
		if err = k.put(previous); err != nil {
			return "", err
		}
		if err = os.Remove(trialPath); err != nil {
			return "", err
		}
		return "took back an unfinished trial", nil
	default:
		if rejected, _ := os.ReadFile(k.target + ".rejected"); strings.TrimSpace(string(rejected)) == own {
			return "rejected", nil
		}
		theirs, err := digest(k.target)
		if err != nil {
			return "", err
		}
		if theirs == own {
			if running, err := k.current(); err != nil || running {
				return "kept", err
			}
			return "kept, waiting for the service to start it", nil
		}
		if entries, _ := os.ReadDir(k.hold); len(entries) > 0 {
			return "held: " + entries[0].Name(), nil
		}
		if err = k.guard(); err != nil {
			return "", err
		}
		// A program that is not running cannot be made worse, and its
		// successor cannot be judged by what the machine does without it.
		processes, err := k.running()
		if err != nil {
			return "", err
		}
		judged = len(processes) > 0 && k.reconciling()
		program, err := os.Open(k.target)
		if err != nil {
			return "", err
		}
		err = replaceFile(previous, program, 0755)
		program.Close()
		if err != nil {
			return "", err
		}
		if judged {
			boot, err := os.ReadFile(filepath.Join(k.proc, "sys/kernel/random/boot_id"))
			if err != nil {
				return "", err
			}
			if err = replaceFile(trialPath, strings.NewReader(own+" "+strings.TrimSpace(string(boot))+"\n"), 0644); err != nil {
				return "", err
			}
		}
		if err = k.put(k.self); err != nil {
			return "", err
		}
		if !judged {
			return "replaced; the program before it was not running, so this one was not judged", nil
		}
	}
	if reason := k.settled(); reason != "" {
		if err = os.WriteFile(k.target+".rejected", []byte(own+"\n"), 0644); err != nil {
			return "", err
		}
		if err = k.put(previous); err != nil {
			return "", fmt.Errorf("%s, and the program before it could not be put back: %w", reason, err)
		}
		if err = os.Remove(trialPath); err != nil {
			return "", err
		}
		return "rejected", errors.New(reason + ": the program before it is back")
	}
	os.Remove(k.target + ".interrupted")
	os.Remove(k.target + ".trial-of-an-earlier-boot")
	return "replaced", os.Remove(trialPath)
}

// settled waits for one process of the new file that stays up and reconciles,
// and returns why there is none.
func (k *keeper) settled() string {
	since, first, seen := k.now(), 0, time.Time{}
	for k.now().Sub(since) < k.wait {
		k.sleep(time.Second)
		processes, _ := k.running()
		alive := false
		for pid, current := range processes {
			if !current {
				continue
			}
			if pid != first {
				first, seen = pid, k.now()
			}
			alive = true
		}
		if !alive {
			first = 0
			continue
		}
		observed, err := os.Stat(k.observation)
		if k.now().Sub(seen) >= k.stay && err == nil && observed.ModTime().After(seen) {
			return ""
		}
	}
	if first == 0 {
		return "the new program was not running after " + k.wait.String()
	}
	return "the new program did not stay up and reconcile within " + k.wait.String()
}

func (k *keeper) run(stop <-chan os.Signal) {
	state, laid := "", ""
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		// Not part of the pass: the program is replaced whether or not the
		// machine can be told where Station API is, and the other way round.
		if k.apiDirectory != "" {
			said := "keeping " + stationAPIFile + " for " + k.apiURL
			if err := k.lay(); err != nil {
				said = "keep " + stationAPIFile + ": " + err.Error()
			}
			if said != laid {
				laid = said
				log.Print(said)
			}
		}
		next, err := k.pass()
		// What the last pass could not do stands beside the program, for the
		// pod's readiness and for whoever looks at the machine.
		if err != nil {
			log.Printf("keep %s: %v", k.name, err)
			if err := os.WriteFile(k.target+".failure", []byte(err.Error()+"\n"), 0644); err != nil {
				log.Printf("keep %s: %v", k.name, err)
			}
		} else {
			os.Remove(k.target + ".failure")
		}
		if next != state && next != "" {
			state = next
			log.Printf("%s: %s", k.name, state)
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}
