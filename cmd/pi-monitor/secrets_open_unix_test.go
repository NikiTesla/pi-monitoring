//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSecretsOpeningDoesNotFollowSymlinkOrWaitForFIFOWriter(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.env")
	if err := os.WriteFile(target, []byte("TOKEN=fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if file, err := openSecretsFile(link); err == nil {
		file.Close()
		t.Fatal("followed a symlink")
	}
	fifo := filepath.Join(dir, "secrets.env")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	// With no writer, a blocking open would hang. O_NONBLOCK must return an
	// inspectable descriptor so the regular-file check can reject it.
	file, err := openSecretsFile(fifo)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("expected FIFO descriptor: %v", err)
	}
	if err := loadSecrets(filepath.Join(dir, "config.yaml"), nil); err == nil {
		t.Fatal("accepted FIFO secrets file")
	}
}
