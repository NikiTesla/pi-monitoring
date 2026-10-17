//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package main

import (
	"errors"
	"os"
)

func openSecretsFile(path string) (*os.File, error) {
	return nil, errors.New("automatic secrets file loading is unavailable on this platform; supply credentials through the process environment")
}
