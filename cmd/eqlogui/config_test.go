package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_MissingConfigDefaults(t *testing.T) {
	t.Setenv("DPSLOGS_CONFIG", "")
	cfg, path, err := LoadConfig()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if path != "" {
		t.Fatalf("path=%q want empty", path)
	}
	if cfg.Hub.URL != "https://sync.dpslogs.com" {
		t.Fatalf("hub.url=%q", cfg.Hub.URL)
	}
	if cfg.Hub.RoomID != "" || cfg.Hub.Token != "" {
		t.Fatalf("room/token want empty")
	}
}

func TestLoadConfig_ValidOverridesURL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dpslogs.yaml")
	if err := os.WriteFile(p, []byte("hub:\n  url: http://127.0.0.1:8787\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DPSLOGS_CONFIG", p)

	cfg, path, err := LoadConfig()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if path != p {
		t.Fatalf("path=%q want %q", path, p)
	}
	if cfg.Hub.URL != "http://127.0.0.1:8787" {
		t.Fatalf("hub.url=%q", cfg.Hub.URL)
	}
}

func TestLoadConfig_InvalidYAMLDefaultsWithErr(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dpslogs.yaml")
	if err := os.WriteFile(p, []byte("hub: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DPSLOGS_CONFIG", p)

	cfg, path, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected err")
	}
	if path != p {
		t.Fatalf("path=%q want %q", path, p)
	}
	if cfg.Hub.URL != "https://sync.dpslogs.com" {
		t.Fatalf("hub.url=%q", cfg.Hub.URL)
	}
}

func TestLoadConfig_EnvVarPrecedence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dpslogs.yaml")
	if err := os.WriteFile(p, []byte("hub:\n  url: http://example.invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DPSLOGS_CONFIG", p)

	cfg, path, err := LoadConfig()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if path != p {
		t.Fatalf("path=%q want %q", path, p)
	}
	if cfg.Hub.URL != "http://example.invalid" {
		t.Fatalf("hub.url=%q", cfg.Hub.URL)
	}
}
