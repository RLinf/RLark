package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// machine is a host as the keeper sees it: the program's file, /proc, and a
// unit that starts the file at its path again when its process is ended.
type machine struct {
	t       *testing.T
	k       *keeper
	clock   time.Time
	pid     int
	refuse  int // signals the machine refuses before it obeys
	healthy func(program string) bool
}

func newMachine(t *testing.T, own, theirs string) *machine {
	dir := t.TempDir()
	m := &machine{t: t, clock: time.Unix(1_800_000_000, 0), pid: 100, healthy: func(string) bool { return true }}
	for name, content := range map[string]string{"self": own, "station-gateway": theirs} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, sub := range []string{"proc/sys/kernel/random", "hold", "unit.d"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "proc/sys/kernel/random/boot_id"), []byte("boot-1\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "proc/1/ns"), 0755)
	os.Symlink("pid:[4026531836]", filepath.Join(dir, "proc/1/ns/pid"))
	m.k = &keeper{self: filepath.Join(dir, "self"), target: filepath.Join(dir, "station-gateway"), name: "/opt/rlark/bin/station-gateway",
		hold: filepath.Join(dir, "hold"), observation: filepath.Join(dir, "observation"), dropIn: filepath.Join(dir, "unit.d"), proc: filepath.Join(dir, "proc"),
		wait: 2 * time.Minute, stay: 30 * time.Second, now: func() time.Time { return m.clock }}
	m.k.signal = func(pid int) error {
		if m.refuse > 0 {
			m.refuse--
			return syscall.EPERM
		}
		os.RemoveAll(filepath.Join(m.k.proc, strconv.Itoa(pid)))
		m.pid++
		m.start()
		return nil
	}
	m.k.sleep = func(d time.Duration) {
		m.clock = m.clock.Add(d)
		if m.healthy(m.program()) {
			m.observe()
		} else if m.clock.Second()%10 == 0 {
			// A program that exits is started again by its unit, under a new pid.
			m.k.signal(m.pid)
		}
	}
	m.start()
	m.observe()
	return m
}

func (m *machine) observe() {
	os.WriteFile(m.k.observation, nil, 0644)
	os.Chtimes(m.k.observation, m.clock, m.clock)
}

func (m *machine) program() string {
	content, _ := os.ReadFile(m.k.target)
	return string(content)
}

// start is the unit starting the file at the path. As in /proc, the process's
// exe stays the file it was started from when another takes the path.
func (m *machine) start() {
	dir := filepath.Join(m.k.proc, strconv.Itoa(m.pid))
	os.Mkdir(dir, 0755)
	os.WriteFile(filepath.Join(dir, "cmdline"), []byte(m.k.name+"\x00--config\x00x"), 0644)
	os.Link(m.k.target, filepath.Join(dir, "exe"))
	os.Mkdir(filepath.Join(dir, "ns"), 0755)
	os.Symlink("pid:[4026531836]", filepath.Join(dir, "ns/pid"))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestKeeperReplacesTheMachinesProgramAndLeavesItAloneAfterwards(t *testing.T) {
	m := newMachine(t, "new", "old")
	os.WriteFile(m.k.target+".interrupted", []byte("an earlier attempt\n"), 0644)
	if state, err := m.k.pass(); state != "replaced" || err != nil || m.program() != "new" || m.pid != 101 || exists(m.k.target+".interrupted") {
		t.Fatalf("state %q err %v program %q pid %d", state, err, m.program(), m.pid)
	}
	// A pass that failed is not settled until one succeeds.
	os.WriteFile(m.k.target+".failure", []byte("stat /proc/1/exe: permission denied\n"), 0644)
	if err := m.k.settledState(); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("a failed pass counts as settled: %v", err)
	}
	os.Remove(m.k.target + ".failure")
	if previous, _ := os.ReadFile(m.k.target + ".previous"); string(previous) != "old" || exists(m.k.target+".trial") || !exists(filepath.Join(m.k.dropIn, "10-trial.conf")) {
		t.Fatalf("after the trial: previous %q, trial %v", previous, exists(m.k.target+".trial"))
	}
	if state, err := m.k.pass(); state != "kept" || err != nil || m.pid != 101 || m.k.settledState() != nil {
		t.Fatalf("a kept program was touched: %q %v pid %d", state, err, m.pid)
	}
}

func TestKeeperWaitsWhileTheMachineIsInUse(t *testing.T) {
	m := newMachine(t, "new", "old")
	held := filepath.Join(m.k.hold, "fleet-robot")
	os.WriteFile(held, nil, 0644)
	if state, err := m.k.pass(); state != "held: fleet-robot" || err != nil || m.program() != "old" || m.pid != 100 {
		t.Fatalf("state %q err %v program %q pid %d", state, err, m.program(), m.pid)
	}
	// A machine that waits for its turn does not stop the rollout to the others.
	if err := m.k.settledState(); err != nil {
		t.Fatalf("a held machine is not settled: %v", err)
	}
	os.Remove(held)
	if state, _ := m.k.pass(); state != "replaced" {
		t.Fatalf("state %q after the machine was free", state)
	}
}

func TestKeeperPutsTheProgramBackWhenTheNewOneDoesNotStayUp(t *testing.T) {
	m := newMachine(t, "new", "old")
	m.healthy = func(program string) bool { return program == "old" }
	state, err := m.k.pass()
	if state != "rejected" || err == nil || !strings.Contains(err.Error(), "did not stay up") || m.program() != "old" || exists(m.k.target+".trial") {
		t.Fatalf("state %q err %v program %q", state, err, m.program())
	}
	pid := m.pid
	if state, err := m.k.pass(); state != "rejected" || err != nil || m.pid != pid || m.program() != "old" || m.k.settledState() == nil {
		t.Fatalf("a rejected version was tried again or counts as settled: %q %v", state, err)
	}
	// Another version is another attempt.
	os.WriteFile(m.k.self, []byte("newer"), 0755)
	m.healthy = func(string) bool { return true }
	m.observe()
	if state, err := m.k.pass(); state != "replaced" || err != nil || m.program() != "newer" {
		t.Fatalf("state %q err %v program %q", state, err, m.program())
	}
}

func TestKeeperDoesNotJudgeTheSuccessorOfAProgramThatWasNotRunning(t *testing.T) {
	m := newMachine(t, "new", "old")
	os.RemoveAll(filepath.Join(m.k.proc, "100"))
	m.k.signal = func(int) error { t.Fatal("nothing runs"); return nil }
	state, err := m.k.pass()
	if !strings.HasPrefix(state, "replaced; the program before it was not running") || err != nil || m.program() != "new" || exists(m.k.target+".trial") || exists(m.k.target+".rejected") {
		t.Fatalf("state %q err %v program %q", state, err, m.program())
	}
	if state, _ := m.k.pass(); state != "kept, waiting for the service to start it" || m.k.settledState() == nil {
		t.Fatalf("state %q", state)
	}
}

func TestKeeperFinishesATrialItCouldNotStart(t *testing.T) {
	m := newMachine(t, "new", "old")
	m.refuse = 1
	// The file is replaced and the old process could not be ended: that is
	// neither kept nor settled.
	if state, err := m.k.pass(); err == nil || !strings.Contains(err.Error(), "end process 100") || m.program() != "new" {
		t.Fatalf("state %q err %v", state, err)
	}
	if err := m.k.settledState(); err == nil || !exists(m.k.target+".trial") {
		t.Fatalf("a trial that did not start counts as settled")
	}
	if state, err := m.k.pass(); state != "replaced" || err != nil || m.pid != 101 || exists(m.k.target+".trial") {
		t.Fatalf("state %q err %v pid %d", state, err, m.pid)
	}
}

func TestKeeperTakesBackATrialOfAnotherVersion(t *testing.T) {
	m := newMachine(t, "newer", "new")
	os.WriteFile(m.k.target+".previous", []byte("old"), 0755)
	os.WriteFile(m.k.target+".trial", []byte("digest-of-new boot-1\n"), 0644)
	if state, err := m.k.pass(); state != "took back an unfinished trial" || err != nil || m.program() != "old" || exists(m.k.target+".trial") {
		t.Fatalf("state %q err %v program %q", state, err, m.program())
	}
}

// The unit's drop-in, run as systemd runs it: a trial of an earlier boot is
// taken back before the service starts, one of this boot is left alone.
func TestGuardTakesATrialBackAfterARestartOfTheMachine(t *testing.T) {
	m := newMachine(t, "new", "old")
	m.k.name = m.k.target
	if err := m.k.guard(); err != nil {
		t.Fatal(err)
	}
	unit, _ := os.ReadFile(filepath.Join(m.k.dropIn, "10-trial.conf"))
	line := strings.TrimSuffix(strings.SplitN(string(unit), "ExecStartPre=/bin/sh -c '", 2)[1], "'\n")
	run := func() {
		if out, err := exec.Command("/bin/sh", "-c", strings.ReplaceAll(line, "$$", "$")).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	boot, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	os.WriteFile(m.k.target, []byte("new"), 0755)
	os.WriteFile(m.k.target+".previous", []byte("old"), 0755)
	run()
	os.WriteFile(m.k.target+".trial", []byte("digest-of-new "+string(boot)), 0644)
	run()
	if m.program() != "new" || !exists(m.k.target+".trial") {
		t.Fatalf("a trial of this boot was taken back: %q", m.program())
	}
	// The first trial a restart cuts short is tried again, the second is the version's last.
	os.WriteFile(m.k.target+".trial", []byte("digest-of-new an-earlier-boot\n"), 0644)
	run()
	interrupted, _ := os.ReadFile(m.k.target + ".interrupted")
	if info, _ := os.Stat(m.k.target); m.program() != "old" || exists(m.k.target+".trial") || exists(m.k.target+".rejected") || string(interrupted) != "digest-of-new\n" || info.Mode().Perm() != 0755 {
		t.Fatalf("program %q trial %v interrupted %q", m.program(), exists(m.k.target+".trial"), interrupted)
	}
	os.WriteFile(m.k.target, []byte("new"), 0755)
	os.WriteFile(m.k.target+".trial", []byte("digest-of-new another-boot\n"), 0644)
	run()
	if rejected, _ := os.ReadFile(m.k.target + ".rejected"); m.program() != "old" || exists(m.k.target+".trial") || string(rejected) != "digest-of-new\n" {
		t.Fatalf("program %q rejected %q after the second interrupted trial", m.program(), rejected)
	}
}

// A program of the same path in a container on the machine (the pod sees
// every process of the machine) is not the machine's service.
func TestKeeperLeavesAContainersProgramOfTheSamePathAlone(t *testing.T) {
	m := newMachine(t, "new", "old")
	inside := filepath.Join(m.k.proc, "777")
	os.MkdirAll(filepath.Join(inside, "ns"), 0755)
	os.WriteFile(filepath.Join(inside, "cmdline"), []byte(m.k.name+"\x00--config\x00x"), 0644)
	os.WriteFile(filepath.Join(m.k.proc, "its-file"), []byte("another build"), 0755)
	os.Link(filepath.Join(m.k.proc, "its-file"), filepath.Join(inside, "exe"))
	os.Symlink("pid:[4026532999]", filepath.Join(inside, "ns/pid"))
	signal := m.k.signal
	m.k.signal = func(pid int) error {
		if pid == 777 {
			t.Fatal("a container's process was told to end")
		}
		return signal(pid)
	}
	if state, err := m.k.pass(); state != "replaced" || err != nil {
		t.Fatalf("state %q err %v", state, err)
	}
	if state, err := m.k.pass(); state != "kept" || err != nil {
		t.Fatalf("state %q err %v", state, err)
	}
}

func TestKeeperLaysWhereTheServiceAsksForItsDocument(t *testing.T) {
	m := newMachine(t, "v2", "v2")
	directory := t.TempDir()
	m.k.apiDirectory, m.k.apiURL, m.k.apiCA = directory, "https://station-api.rlark.internal:18443", filepath.Join(directory, "mounted-ca.crt")
	// Without the CA nothing is laid: the file never names a CA that is not there.
	if err := m.k.lay(); err == nil {
		t.Fatal("laid without a CA")
	}
	if _, err := os.Stat(filepath.Join(directory, stationAPIFile)); err == nil {
		t.Fatalf("%s without its CA", stationAPIFile)
	}
	os.WriteFile(m.k.apiCA, []byte("the CA\n"), 0600)
	if err := m.k.lay(); err != nil {
		t.Fatal(err)
	}
	// Byte for byte what the installer writes (astribot/station/bootstrap).
	pointer, _ := os.ReadFile(filepath.Join(directory, stationAPIFile))
	ca, _ := os.ReadFile(filepath.Join(directory, "ca.crt"))
	if string(pointer) != `{"url":"https://station-api.rlark.internal:18443","ca":"ca.crt"}`+"\n" || string(ca) != "the CA\n" {
		t.Fatalf("laid %q and %q", pointer, ca)
	}
	// What is there already is left as it is; what differs is replaced.
	before, _ := os.Stat(filepath.Join(directory, stationAPIFile))
	os.WriteFile(m.k.apiCA, []byte("another CA\n"), 0600)
	if err := m.k.lay(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(directory, stationAPIFile))
	if ca, _ := os.ReadFile(filepath.Join(directory, "ca.crt")); !os.SameFile(before, after) || string(ca) != "another CA\n" || after.Mode().Perm() != 0644 {
		t.Fatalf("second pass: CA %q, the file written again: %v", ca, !os.SameFile(before, after))
	}
}
