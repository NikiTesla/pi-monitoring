package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxSecretsBytes = 64 << 10

// loadSecrets supplies only credentials referenced by the command's configuration.
// The CLI owns the process environment; an explicitly set variable, even empty,
// takes precedence over the adjacent file. No shell is invoked.
func loadSecrets(configPath string, names []string) error {
	path := filepath.Join(filepath.Dir(configPath), "secrets.env")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect secrets file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("secrets file %q must be a regular file, not a symlink or device", path)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("secrets file %q must be private; run chmod 600 on it", path)
	}
	file, err := openSecretsFile(path)
	if err != nil {
		return fmt.Errorf("open secrets file %q: %w", path, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened secrets file %q: %w", path, err)
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("secrets file %q changed while opening; retry with a private regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSecretsBytes+1))
	if err != nil {
		return fmt.Errorf("read secrets file %q: %w", path, err)
	}
	if len(data) > maxSecretsBytes {
		return fmt.Errorf("secrets file %q exceeds 64 KiB", path)
	}
	values, err := parseSecrets(data)
	if err != nil {
		return fmt.Errorf("secrets file %q: %w", path, err)
	}
	// Parse the entire file before changing any environment variables. Error
	// messages never include assignment names or values from the secrets file.
	for _, name := range names {
		if _, exists := os.LookupEnv(name); exists {
			continue
		}
		if value, exists := values[name]; exists {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("could not load a credential from secrets file %q", path)
			}
		}
	}
	return nil
}

func parseSecrets(data []byte) (map[string]string, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("must contain UTF-8 text")
	}
	values := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// The whole file is already bounded; permit one assignment of that size.
	scanner.Buffer(make([]byte, 1024), maxSecretsBytes+1)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		name, value, found := strings.Cut(line, "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !found || !validEnvironmentName(name) {
			return nil, fmt.Errorf("line %d: expected NAME=value with a valid environment variable name", lineNumber)
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("line %d: duplicate assignment", lineNumber)
		}
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			quote := value[0]
			if len(value) < 2 || value[len(value)-1] != quote || strings.ContainsRune(value[1:len(value)-1], rune(quote)) {
				return nil, fmt.Errorf("line %d: expected a single-line quoted value", lineNumber)
			}
			value = value[1 : len(value)-1]
		} else if strings.ContainsAny(value, " \t'\"") {
			return nil, fmt.Errorf("line %d: quote values containing spaces or quotes", lineNumber)
		}
		for _, c := range value {
			if c < 0x20 || c == 0x7f {
				return nil, fmt.Errorf("line %d: control characters are not allowed in values", lineNumber)
			}
		}
		values[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("could not read assignments")
	}
	return values, nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
