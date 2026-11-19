package main

import (
	"reflect"
	"testing"
)

func TestNewSSHDServerCopiesMultipleAuthorizedKeyFiles(t *testing.T) {
	files := []string{"/keys/workload", "/keys/admin"}
	server := newSSHDServer("2222", "/bin/sh", files)
	files[0] = "/changed"
	if server.Port != "2222" || server.Shell != "/bin/sh" || !reflect.DeepEqual(server.AuthorizedKeysFiles, []string{"/keys/workload", "/keys/admin"}) {
		t.Fatalf("server=%#v", server)
	}
}
