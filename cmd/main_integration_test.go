package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/config"
)

// These tests rely on a real config file in the developer's environment.
// They are deliberately skipped in CI unless the requisite file is present.

func TestRunModels_WithRealConfig(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Skip("config file not found, skipping test")
	}
	configPath := cfg.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("config file not found, skipping test")
	}

	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	os.Setenv("OPENMODEL_CONFIG", configPath)

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runModels(false)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Available models:") {
		t.Fatalf("expected available models output, got: %s", output)
	}
}

func TestRunModels_JSONWithRealConfig(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Skip("config file not found, skipping test")
	}
	configPath := cfg.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("config file not found, skipping test")
	}

	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	os.Setenv("OPENMODEL_CONFIG", configPath)

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runModels(true)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "provider:") {
		t.Fatalf("expected provider info in JSON output, got: %s", output)
	}
}

func TestRunConfig_WithRealConfig(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Skip("config file not found, skipping test")
	}
	configPath := cfg.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("config file not found, skipping test")
	}

	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	os.Setenv("OPENMODEL_CONFIG", configPath)

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfig()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if !strings.HasSuffix(output, "openmodel.json") {
		t.Fatalf("expected config path ending with openmodel.json, got: %s", output)
	}
}
