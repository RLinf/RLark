package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// bashOnly is what bash takes and dash does not, which `dash -n` does not report: it fails only when the line runs
// (11-17: ${platform/\//-} in import_images stopped every join with "Bad substitution"). Pattern substitution,
// substrings, case changes and indirection, [[ ]], here-strings, &> and array assignments.
var bashOnly = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*(/|:[0-9]|: -|\^|,)|\$\{!|\[\[ |<<<|&>|(^|[ ;])[A-Za-z_][A-Za-z0-9_]*=\(`)

// The installer's scripts run under /bin/sh, dash on Debian and Ubuntu: every script that says so, and lib.sh, which
// they source, has none of bash's own syntax.
func TestTheScriptsRunUnderShHaveNoBashSyntax(t *testing.T) {
	scripts, err := filepath.Glob("*.sh")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, script := range scripts {
		data, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		if script != "lib.sh" && !strings.HasPrefix(string(data), "#!/bin/sh\n") {
			continue
		}
		checked++
		lines := bufio.NewScanner(strings.NewReader(string(data)))
		for number := 1; lines.Scan(); number++ {
			if line := lines.Text(); !strings.HasPrefix(strings.TrimSpace(line), "#") && bashOnly.MatchString(line) {
				t.Errorf("%s:%d is bash, not sh: %s", script, number, strings.TrimSpace(line))
			}
		}
	}
	if checked < 2 {
		t.Fatalf("only %d scripts run under /bin/sh were checked", checked)
	}
}

// import_images, as dash runs it, imports the archives of images/ and of this machine's platform's directory, not
// another platform's, and pins what each import unpacked.
func TestImportImagesUnderDashTakesThisPlatformsArchives(t *testing.T) {
	images, stubs := t.TempDir(), t.TempDir()
	for _, archive := range []string{"node.tar", "linux-amd64/installer.tar", "linux-arm64/installer.tar"} {
		path := filepath.Join(images, archive)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stub := func(name, body string) {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stub("uname", `echo "$ARCH"`+"\n")
	// ctr: an import says what it unpacked, as ctr does; every call is logged.
	stub("ctr", `echo "$*" >> "$LOG"
case "$*" in *" images import "*) for last; do :; done; echo "unpacking image-of-$(basename "$(dirname "$last")")-$(basename "$last") (sha256:00)...done" ;; esac
`)
	for arch, want := range map[string][]string{
		"x86_64":  {"import --platform linux/amd64 " + images + "/node.tar", "label image-of-" + filepath.Base(images) + "-node.tar", "import --platform linux/amd64 " + images + "/linux-amd64/installer.tar", "label image-of-linux-amd64-installer.tar"},
		"aarch64": {"import --platform linux/arm64 " + images + "/node.tar", "label image-of-" + filepath.Base(images) + "-node.tar", "import --platform linux/arm64 " + images + "/linux-arm64/installer.tar", "label image-of-linux-arm64-installer.tar"},
	} {
		log := filepath.Join(t.TempDir(), "ctr.log")
		command := exec.Command("dash", "-c", `. ./lib.sh; B=$(mktemp -d); ctr_of() { echo "$STUBS/ctr"; }; import_images "$IMAGES"`)
		command.Env = append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"), "STUBS="+stubs, "IMAGES="+images, "ARCH="+arch, "LOG="+log, "SOCKET=/run/test.sock")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", arch, err, output)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		var calls []string
		for _, call := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			fields := strings.Fields(call)
			switch {
			case slices.Contains(fields, "import"):
				calls = append(calls, "import --platform "+fields[slices.Index(fields, "--platform")+1]+" "+fields[len(fields)-1])
			case slices.Contains(fields, "label") && slices.Contains(fields, "io.cri-containerd.pinned=pinned"):
				calls = append(calls, "label "+fields[slices.Index(fields, "label")+1])
			}
		}
		if !slices.Equal(calls, want) {
			t.Errorf("%s: ctr was called for %q, want %q", arch, calls, want)
		}
	}
}
