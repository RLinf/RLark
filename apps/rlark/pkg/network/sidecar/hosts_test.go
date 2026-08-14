package sidecar

import (
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

func TestUpdateHostsFile_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	hostsFile := filepath.Join(dir, "hosts")

	hs := newHostsSyncer(nil, hostsFile, 0)

	hosts := map[string]string{
		"pod-a.domain1.domain": "10.0.0.1",
		"pod-b.domain1.domain": "10.0.0.2",
	}
	if err := hs.updateHostsFile(hosts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	if !strings.Contains(string(content), hostsBeginMarker) {
		t.Fatalf("begin marker should be present, got:\n%s", string(content))
	}
	if !strings.Contains(string(content), hostsEndMarker) {
		t.Fatalf("end marker should be present, got:\n%s", string(content))
	}
	if !strings.Contains(string(content), "10.0.0.1\tpod-a.domain1.domain") {
		t.Fatalf("entry should be present, got:\n%s", string(content))
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
