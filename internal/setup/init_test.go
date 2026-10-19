package setup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesPrivateConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	var out bytes.Buffer
	if err := Init(InitOptions{OutputPath: path, EnableDocker: true, DisableDashboard: true}, &out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("configuration exposed to group/others: %v", info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		t.Fatalf("missing configuration: %v", err)
	}
	if !strings.Contains(out.String(), "No service installed or started") {
		t.Fatalf("misleading output: %s", &out)
	}
}

func TestInitPreservesExistingFileAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.yaml")
	if err := os.WriteFile(target, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(dir, "dangling.yaml")
	missing := filepath.Join(dir, "missing.yaml")
	if err := os.Symlink(missing, dangling); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, link, dangling} {
		if err := Init(InitOptions{OutputPath: path}, &bytes.Buffer{}); err == nil {
			t.Errorf("overwrote existing path %s", path)
		}
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("followed dangling symlink: %v", err)
	}
}

func TestInitRejectsUnsupportedActions(t *testing.T) {
	for _, options := range []InitOptions{
		{Start: true},
		{Start: true, DryRun: true},
		{Install: true, OutputPath: "config.yaml", DryRun: true},
		{Install: true},
		{Install: true, Start: true},
	} {
		var out bytes.Buffer
		if err := Init(options, &out); err == nil {
			t.Errorf("accepted unsupported options: %+v", options)
		}
		if out.Len() != 0 {
			t.Errorf("reported actions for rejected options: %s", &out)
		}
	}
}

func TestInitDryRunsDoNotCreateConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	var out bytes.Buffer
	if err := Init(InitOptions{OutputPath: path, DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("dry run mutated directory: %v, %v", entries, err)
	}
	out.Reset()
	if err := Init(InitOptions{Install: true, Start: true, EnableDocker: true, DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"service installation is not implemented", "/usr/local/bin/pi-monitor", "pi-monitor.service", "substantial host privileges", "unverified"} {
		// A missing Docker socket explicitly reports that the plan cannot run.
		if want == "unverified" && strings.Contains(out.String(), "this plan is not runnable") {
			continue
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("plan missing %q: %s", want, &out)
		}
	}
}
