package sidecar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildManagedSection(t *testing.T) {
	hosts := map[string]string{
		"pod-b.domain1.domain": "10.0.0.2",
		"pod-a.domain1.domain": "10.0.0.1",
	}

	got := buildManagedSection(hosts)

	if !strings.HasPrefix(got, hostsBeginMarker+"\n") {
		t.Fatalf("expected section to start with begin marker, got:\n%s", got)
	}
	if !strings.HasSuffix(got, hostsEndMarker+"\n") {
		t.Fatalf("expected section to end with end marker, got:\n%s", got)
	}

	// Entries should be sorted by hostname.
	if !strings.Contains(got, "10.0.0.1\tpod-a.domain1.domain\n10.0.0.2\tpod-b.domain1.domain\n") {
		t.Fatalf("expected sorted entries, got:\n%s", got)
	}
}

func TestWatchOnceUpdatesHosts(t *testing.T) {
	hosts := map[string]string{"pod-a.domain": "10.0.0.2"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/watch_hosts" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("ETag", hostsVersion(hosts))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pod-a.domain":"10.0.0.2"}`))
	}))
	defer server.Close()

	hostsFile := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFile, []byte("127.0.0.1\tlocalhost\n"), 0644); err != nil {
		t.Fatal(err)
	}
	hs := newHostsSyncer(rewriteHostTransport{base: http.DefaultTransport, host: server.URL}, hostsFile, 0)
	result := hs.watchOnce(context.Background(), "")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if !result.supported {
		t.Fatal("watch should be supported")
	}
	if err := hs.applyHosts(result.hosts, result.version); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "10.0.0.2\tpod-a.domain") {
		t.Fatalf("hosts file = %q", content)
	}
}

func TestWatchOnceFallsBackForOldServer(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	hs := newHostsSyncer(rewriteHostTransport{base: http.DefaultTransport, host: server.URL}, "", 0)

	result := hs.watchOnce(context.Background(), "")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.supported {
		t.Fatal("404 watch endpoint should be treated as unsupported")
	}
}

func TestRepairHostsFileRestoresExternalOverwrite(t *testing.T) {
	hostsFile := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFile, []byte("127.0.0.1\tlocalhost\n"), 0644); err != nil {
		t.Fatal(err)
	}
	hs := newHostsSyncer(nil, hostsFile, 0)
	hs.hosts = map[string]string{"pod-a.domain": "10.0.0.2"}

	if err := hs.repairHostsFile(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "10.0.0.2\tpod-a.domain") {
		t.Fatal("managed hosts section was not restored")
	}
}

type rewriteHostTransport struct {
	base http.RoundTripper
	host string
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	target, _ := url.Parse(t.host)
	clone.URL.Scheme = target.Scheme
	clone.URL.Host = target.Host
	return t.base.RoundTrip(clone)
}

func TestBuildManagedSection_Empty(t *testing.T) {
	got := buildManagedSection(map[string]string{})
	expected := hostsBeginMarker + "\n" + hostsEndMarker + "\n"
	if got != expected {
		t.Fatalf("expected:\n%q\ngot:\n%q", expected, got)
	}
}

func TestReplaceManagedSection_NoExistingSection(t *testing.T) {
	original := "127.0.0.1\tlocalhost\n::1\tlocalhost\n"
	section := buildManagedSection(map[string]string{
		"pod-a.domain1.domain": "10.0.0.1",
	})

	got, err := replaceManagedSection(original, section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(got, original) {
		t.Fatalf("original content should be preserved at the top, got:\n%s", got)
	}
	if !strings.Contains(got, hostsBeginMarker) || !strings.Contains(got, hostsEndMarker) {
		t.Fatalf("managed section should be appended, got:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.1\tpod-a.domain1.domain") {
		t.Fatalf("new entry should be present, got:\n%s", got)
	}
}

func TestReplaceManagedSection_ReplaceExisting(t *testing.T) {
	original := "127.0.0.1\tlocalhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.99\told-entry.domain\n" +
		hostsEndMarker + "\n" +
		"192.168.1.1\trouter\n"

	section := buildManagedSection(map[string]string{
		"pod-a.domain1.domain": "10.0.0.1",
		"pod-b.domain1.domain": "10.0.0.2",
	})

	got, err := replaceManagedSection(original, section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Content before the markers must be preserved.
	if !strings.HasPrefix(got, "127.0.0.1\tlocalhost\n") {
		t.Fatalf("content before markers should be preserved, got:\n%s", got)
	}

	// Content after the markers must be preserved.
	if !strings.HasSuffix(got, "192.168.1.1\trouter\n") {
		t.Fatalf("content after markers should be preserved, got:\n%s", got)
	}

	// Old entry must be gone; new entries must be present.
	if strings.Contains(got, "old-entry.domain") {
		t.Fatalf("old entry should be removed, got:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.1\tpod-a.domain1.domain") {
		t.Fatalf("new entry pod-a should be present, got:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.2\tpod-b.domain1.domain") {
		t.Fatalf("new entry pod-b should be present, got:\n%s", got)
	}
}

func TestReplaceManagedSection_ClearToEmpty(t *testing.T) {
	original := "127.0.0.1\tlocalhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.99\told-entry.domain\n" +
		hostsEndMarker + "\n"

	section := buildManagedSection(map[string]string{})

	got, err := replaceManagedSection(original, section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(got, "old-entry.domain") {
		t.Fatalf("old entry should be removed, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "127.0.0.1\tlocalhost\n") {
		t.Fatalf("content before markers should be preserved, got:\n%s", got)
	}
}

func TestReplaceManagedSection_MissingEndMarker(t *testing.T) {
	original := "127.0.0.1\tlocalhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.99\told-entry.domain\n" +
		"192.168.1.1\trouter\n"

	section := buildManagedSection(map[string]string{
		"pod-a.domain1.domain": "10.0.0.1",
	})

	got, err := replaceManagedSection(original, section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(got, "old-entry.domain") {
		t.Fatalf("old entry should be removed, got:\n%s", got)
	}
	if strings.Contains(got, "192.168.1.1\trouter") {
		t.Fatalf("content after begin marker should be replaced, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "127.0.0.1\tlocalhost\n") {
		t.Fatalf("content before begin marker should be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.1\tpod-a.domain1.domain") {
		t.Fatalf("new entry should be present, got:\n%s", got)
	}
}

func TestUpdateHostsFileRejectsInvalidBaseFile(t *testing.T) {
	hosts := map[string]string{"pod-a.domain": "10.0.0.1"}
	for _, content := range []string{"", "10.0.0.9\tpod-only\n"} {
		hostsFile := filepath.Join(t.TempDir(), "hosts")
		if err := os.WriteFile(hostsFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		hs := newHostsSyncer(nil, hostsFile, 0)

		if err := hs.updateHostsFile(hosts); err == nil {
			t.Fatalf("expected invalid base hosts %q to be rejected", content)
		}
		got, err := os.ReadFile(hostsFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Fatalf("invalid hosts file was modified: got %q, want %q", got, content)
		}
	}
}

func TestApplyHostsDoesNotAdvanceStateWhenBaseFileIsInvalid(t *testing.T) {
	hostsFile := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFile, nil, 0644); err != nil {
		t.Fatal(err)
	}
	hs := newHostsSyncer(nil, hostsFile, 0)
	hosts := map[string]string{"pod-a.domain": "10.0.0.1"}

	if err := hs.applyHosts(hosts, "new-version"); err == nil {
		t.Fatal("expected invalid base hosts to reject update")
	}
	if hs.version != "" || hs.hosts != nil {
		t.Fatalf("state advanced after failed update: version=%q hosts=%v", hs.version, hs.hosts)
	}

	if err := os.WriteFile(hostsFile, []byte("127.0.0.1\tlocalhost\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := hs.applyHosts(hosts, "new-version"); err != nil {
		t.Fatalf("retry after base hosts recovered: %v", err)
	}
}

func TestUpdateHostsFile_PreservesOutsideSection(t *testing.T) {
	dir := t.TempDir()
	hostsFile := filepath.Join(dir, "hosts")

	original := "127.0.0.1\tlocalhost\n" +
		"::1\tlocalhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.99\told.domain\n" +
		hostsEndMarker + "\n" +
		"192.168.1.1\trouter\n"
	if err := os.WriteFile(hostsFile, []byte(original), 0644); err != nil {
		t.Fatalf("write initial hosts file: %v", err)
	}

	hs := newHostsSyncer(nil, hostsFile, 0)

	hosts := map[string]string{
		"pod-a.domain1.domain": "10.0.0.1",
	}
	if err := hs.updateHostsFile(hosts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	got := string(content)

	if strings.Contains(got, "old.domain") {
		t.Fatalf("old entry should be removed, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "127.0.0.1\tlocalhost\n::1\tlocalhost\n") {
		t.Fatalf("content before section should be preserved, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "192.168.1.1\trouter\n") {
		t.Fatalf("content after section should be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.1\tpod-a.domain1.domain") {
		t.Fatalf("new entry should be present, got:\n%s", got)
	}
}

func TestUpdateHostsFile_Idempotent(t *testing.T) {
	dir := t.TempDir()
	hostsFile := filepath.Join(dir, "hosts")

	original := "127.0.0.1\tlocalhost\n"
	if err := os.WriteFile(hostsFile, []byte(original), 0644); err != nil {
		t.Fatalf("write initial hosts file: %v", err)
	}

	hs := newHostsSyncer(nil, hostsFile, 0)

	hosts := map[string]string{
		"pod-a.domain1.domain": "10.0.0.1",
	}

	if err := hs.updateHostsFile(hosts); err != nil {
		t.Fatalf("first update: %v", err)
	}

	first, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	if err := hs.updateHostsFile(hosts); err != nil {
		t.Fatalf("second update: %v", err)
	}

	second, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("second update should not change content\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
