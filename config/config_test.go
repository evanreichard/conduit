package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestLoadConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server":"https://example.com","api_key":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// Load Config File
	values, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Values
	if values["server"] != "https://example.com" {
		t.Fatalf("expected server from config file, got %q", values["server"])
	}
	if values["api_key"] != "secret" {
		t.Fatalf("expected api_key from config file, got %q", values["api_key"])
	}
}

func TestFindConfigFile(t *testing.T) {
	workDir := t.TempDir()
	configDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configDir)

	// Missing Config File
	path, err := findConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("expected no config file, got %q", path)
	}

	// User Config File
	userPath := filepath.Join(configDir, "conduit", "config.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err = findConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if path != userPath {
		t.Fatalf("expected user config file %q, got %q", userPath, path)
	}

	// Local Config File Precedence
	localPath := "conduit.json"
	if err := os.WriteFile(localPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err = findConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if path != localPath {
		t.Fatalf("expected local config file %q, got %q", localPath, path)
	}
}

func TestGetConfigValuePriority(t *testing.T) {
	def := ConfigDef{Key: "server", Env: "CONDUIT_SERVER", Default: "default"}
	fileValues := map[string]string{"server": "file"}

	// Config File Beats Default
	if value := getConfigValue(nil, fileValues, def); value != "file" {
		t.Fatalf("expected file value, got %q", value)
	}

	// Environment Beats Config File
	t.Setenv("CONDUIT_SERVER", "env")
	if value := getConfigValue(nil, fileValues, def); value != "env" {
		t.Fatalf("expected env value, got %q", value)
	}

	// Flags Beat Environment
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("server", "default", "server")
	if err := flags.Set("server", "flag"); err != nil {
		t.Fatal(err)
	}
	if value := getConfigValue(flags, fileValues, def); value != "flag" {
		t.Fatalf("expected flag value, got %q", value)
	}
}

func TestGetClientConfigFileValuesIgnoresTunnelSettings(t *testing.T) {
	workDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}

	// Write Local Config File
	if err := os.WriteFile("conduit.json", []byte(`{"server":"https://example.com","api_key":"secret","name":"saved","target":"localhost:3000"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	values, err := getClientConfigFileValues()
	if err != nil {
		t.Fatal(err)
	}
	if values["server"] != "https://example.com" {
		t.Fatalf("expected server from config file, got %q", values["server"])
	}
	if values["name"] != "" || values["target"] != "" {
		t.Fatalf("expected tunnel settings to be ignored, got name=%q target=%q", values["name"], values["target"])
	}
}
