package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListen(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := listen(file); err == nil {
		t.Error("listen replaced a regular file")
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("listen removed a regular file: %v", err)
	}
	sock := filepath.Join(dir, "sock")
	l, err := listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := listen(sock); err == nil {
		t.Error("listen took a socket another listener holds")
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close()
	if l, err = listen(sock); err != nil {
		t.Errorf("listen over a stale socket: %v", err)
	} else {
		l.Close()
	}
}
