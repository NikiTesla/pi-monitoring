//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"os"
	"syscall"
)

func openSecretsFile(path string) (*os.File, error) {
	// A concurrent replacement must neither follow a final symlink nor block
	// while opening a FIFO. The caller validates the opened descriptor too.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
