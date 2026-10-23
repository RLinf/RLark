package sshd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerReloadsAuthorizedKeysFileForEveryAuthentication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authorized_key")
	first := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKuK8cXf4I01vK1RZP9h9eJgo7iShVQ/x1fV6pG3Yf9n first"
	second := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEzKnV5gB4QrgM5rRz6uWfVUGn8YqA3Fq4pN7C8oD2yA second"
	if err := os.WriteFile(path, []byte(first+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{AuthorizedKeysFiles: []string{path}}
	keys, err := server.loadAuthorizedKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("first load keys=%d err=%v", len(keys), err)
	}
	firstWire := string(keys[0].Marshal())
	if err := os.WriteFile(path, []byte(second+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err = server.loadAuthorizedKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("second load keys=%d err=%v", len(keys), err)
	}
	if string(keys[0].Marshal()) == firstWire {
		t.Fatal("authorized key file update was not reloaded")
	}
}

func TestServerTreatsEmptyAuthorizedKeysFileAsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authorized_key")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (&Server{AuthorizedKeysFiles: []string{path}}).loadAuthorizedKeys()
	if err == nil {
		t.Fatal("empty authorized keys file unexpectedly enabled authentication")
	}
}

func TestServerLoadsAuthorizedKeysFromMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first")
	secondPath := filepath.Join(dir, "second")
	first := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKuK8cXf4I01vK1RZP9h9eJgo7iShVQ/x1fV6pG3Yf9n first"
	second := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEzKnV5gB4QrgM5rRz6uWfVUGn8YqA3Fq4pN7C8oD2yA second"
	if err := os.WriteFile(firstPath, []byte(first+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte(second+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := (&Server{AuthorizedKeysFiles: []string{firstPath, secondPath}}).loadAuthorizedKeys()
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys=%d err=%v, want two keys", len(keys), err)
	}
}

func TestServerFailsClosedWhenAnyAuthorizedKeysFileCannotBeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authorized_key")
	key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKuK8cXf4I01vK1RZP9h9eJgo7iShVQ/x1fV6pG3Yf9n first"
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (&Server{AuthorizedKeysFiles: []string{path, filepath.Join(t.TempDir(), "missing")}}).loadAuthorizedKeys()
	if err == nil {
		t.Fatal("unreadable authorized keys file did not fail authentication closed")
	}
}
