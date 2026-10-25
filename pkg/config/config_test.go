package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCorruptConfigNamesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "noscli", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded on a corrupt config, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("Load() error %q does not name %s", err, path)
	}
}
