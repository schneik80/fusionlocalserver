package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv unsets all three APS_* environment variables for the duration of the test.
// Load() treats empty strings as unset (it checks `id != ""`), so this works correctly.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APS_CLIENT_ID", "")
	t.Setenv("APS_CLIENT_SECRET", "")
	t.Setenv("APS_REGION", "")
}

// writeConfigFile creates ~/.config/fusionlocalserver/config.json under the given home dir.
func writeConfigFile(t *testing.T, home, contents string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "fusionlocalserver")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// saveDefaults snapshots the package-level ldflags vars and restores them via t.Cleanup.
func saveDefaults(t *testing.T) {
	t.Helper()
	prevID := DefaultClientID
	prevRegion := DefaultRegion
	t.Cleanup(func() {
		DefaultClientID = prevID
		DefaultRegion = prevRegion
	})
}

func TestLoad_EnvVarsTakePrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = "ld-id"
	DefaultRegion = "EMEA"

	// File present with a different client_id — env should still win.
	writeConfigFile(t, home, `{"client_id":"file-id","region":"AUS"}`)

	t.Setenv("APS_CLIENT_ID", "env-id")
	t.Setenv("APS_CLIENT_SECRET", "env-secret")
	t.Setenv("APS_REGION", "EMEA")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "env-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "env-id")
	}
	if cfg.ClientSecret != "env-secret" {
		t.Errorf("ClientSecret = %q, want %q", cfg.ClientSecret, "env-secret")
	}
	if cfg.Region != "EMEA" {
		t.Errorf("Region = %q, want %q", cfg.Region, "EMEA")
	}
}

func TestLoad_FileFallback(t *testing.T) {
	clearEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = ""
	DefaultRegion = ""

	writeConfigFile(t, home, `{"client_id":"file-id","region":"EMEA"}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "file-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "file-id")
	}
	if cfg.Region != "EMEA" {
		t.Errorf("Region = %q, want %q", cfg.Region, "EMEA")
	}
}

func TestLoad_FileFallback_RegionEnvOverride(t *testing.T) {
	clearEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = ""
	DefaultRegion = ""

	writeConfigFile(t, home, `{"client_id":"file-id","region":"EMEA"}`)
	t.Setenv("APS_REGION", "AUS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "file-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "file-id")
	}
	if cfg.Region != "AUS" {
		t.Errorf("Region = %q, want %q (env override)", cfg.Region, "AUS")
	}
}

func TestLoad_LdflagsFallback(t *testing.T) {
	clearEnv(t)
	home := t.TempDir() // empty — no config file
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = "ld-id"
	DefaultRegion = "EMEA"

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "ld-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "ld-id")
	}
	if cfg.Region != "EMEA" {
		t.Errorf("Region = %q, want %q", cfg.Region, "EMEA")
	}
}

func TestLoad_NoneConfigured_Errors(t *testing.T) {
	clearEnv(t)
	home := t.TempDir() // empty — no config file
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = ""
	DefaultRegion = ""

	cfg, err := Load()
	if err == nil {
		t.Fatalf("Load: got nil error, want error; cfg = %+v", cfg)
	}
	if !strings.Contains(err.Error(), "no APS client_id") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no APS client_id")
	}
}

func TestLoad_MalformedFile_Errors(t *testing.T) {
	clearEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = ""
	DefaultRegion = ""

	writeConfigFile(t, home, `{garbage`)

	cfg, err := Load()
	if err == nil {
		t.Fatalf("Load: got nil error, want error; cfg = %+v", cfg)
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "parsing")
	}
}

func TestLoad_EmptyClientIDInFile_Errors(t *testing.T) {
	clearEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveDefaults(t)
	DefaultClientID = ""
	DefaultRegion = ""

	writeConfigFile(t, home, `{"client_id":""}`)

	cfg, err := Load()
	if err == nil {
		t.Fatalf("Load: got nil error, want error; cfg = %+v", cfg)
	}
	if !strings.Contains(err.Error(), "client_id is empty") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "client_id is empty")
	}
}

func TestDir_CreatesWithMode0700(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(home, ".config", "fusionlocalserver")
	if dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("perm = %o, want %o", perm, 0700)
	}
}

func TestPath_ReturnsExpected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := Path()
	want := filepath.Join(home, ".config", "fusionlocalserver", "config.json")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
