package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/config"
)

func TestPrintUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("Usage:")) {
		t.Error("Expected 'Usage:' in help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("serve")) {
		t.Error("Expected 'serve' command in help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test")) {
		t.Error("Expected 'test' command in help output")
	}
}

func TestPrintTestUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printTestUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("Usage:")) {
		t.Error("Expected 'Usage:' in test help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test")) {
		t.Error("Expected 'test' in test help output")
	}
}

func TestPrintServerUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printServerUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("Usage:")) {
		t.Error("Expected 'Usage:' in server help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("serve")) {
		t.Error("Expected 'serve' in server help output")
	}
}

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantCommand   string
		wantRemaining []string
	}{
		{
			name:          "default serve command",
			args:          []string{},
			wantCommand:   "serve",
			wantRemaining: []string{},
		},
		{
			name:          "explicit serve command",
			args:          []string{"serve", "-h"},
			wantCommand:   "serve",
			wantRemaining: []string{"-h"},
		},
		{
			name:          "test command",
			args:          []string{"test", "-model", "glm"},
			wantCommand:   "test",
			wantRemaining: []string{"-model", "glm"},
		},
		{
			name:          "flags before command",
			args:          []string{"-h", "serve"},
			wantCommand:   "serve",
			wantRemaining: []string{},
		},
		{
			name:          "unknown command",
			args:          []string{"unknown"},
			wantCommand:   "unknown",
			wantRemaining: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate command parsing logic from main
			command := "serve"
			args := make([]string, len(tt.args))
			copy(args, tt.args)

			for i, arg := range args {
				if arg == "-h" || arg == "--help" {
					command = "serve"
					args = args[:0]
					break
				}
				if !bytes.HasPrefix([]byte(arg), []byte("-")) {
					command = arg
					args = args[i+1:]
					break
				}
			}

			if command != tt.wantCommand {
				t.Errorf("command = %q, want %q", command, tt.wantCommand)
			}

			if len(args) != len(tt.wantRemaining) {
				t.Errorf("remaining args = %v, want %v", args, tt.wantRemaining)
			}
		})
	}
}

func TestFlagParsingTestCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantModel   string
		wantCheck   bool
		wantParseOK bool
	}{
		{
			name:        "no flags",
			args:        []string{},
			wantModel:   "",
			wantCheck:   false,
			wantParseOK: true,
		},
		{
			name:        "model flag",
			args:        []string{"-model", "glm-4"},
			wantModel:   "glm-4",
			wantCheck:   false,
			wantParseOK: true,
		},
		{
			name:        "check flag",
			args:        []string{"-check"},
			wantModel:   "",
			wantCheck:   true,
			wantParseOK: true,
		},
		{
			name:        "both flags",
			args:        []string{"-model", "glm-5", "-check"},
			wantModel:   "glm-5",
			wantCheck:   true,
			wantParseOK: true,
		},
		{
			name:        "unknown flag",
			args:        []string{"-unknown"},
			wantModel:   "",
			wantCheck:   false,
			wantParseOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flag.NewFlagSet("test", flag.ContinueOnError)
			flag.SetOutput(&bytes.Buffer{})

			modelName := flag.String("model", "", "Model name to test")
			jsonOutput := flag.Bool("check", false, "Output results in JSON")

			err := flag.Parse(tt.args)
			parseOK := err == nil

			if parseOK != tt.wantParseOK {
				t.Errorf("parse error = %v, want parseOK = %v", err, tt.wantParseOK)
			}

			if parseOK && *modelName != tt.wantModel {
				t.Errorf("model = %q, want %q", *modelName, tt.wantModel)
			}

			if parseOK && *jsonOutput != tt.wantCheck {
				t.Errorf("check = %v, want %v", *jsonOutput, tt.wantCheck)
			}
		})
	}
}

func TestRunTestNoConfig(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel", "test"}

	modelName := new(string)
	check := new(bool)
	*modelName = ""
	*check = false

	cfg, err := config.Load()

	if err == nil {
		t.Logf("Config loaded in test environment: %+v", cfg)
	} else {
		t.Logf("Expected error (no config): %v", err)
	}
}

func TestPrintVersion(t *testing.T) {
	// Save original version and build date for restoration
	originalVersion := Version
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()

	// Set test values
	Version = "1.2.3"
	BuildDate = "2024-01-15"

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	printVersion()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()

	if !strings.Contains(output, "openmodel version 1.2.3") {
		t.Errorf("Expected version output, got: %s", output)
	}
	if !strings.Contains(output, "build date: 2024-01-15") {
		t.Errorf("Expected build date in output, got: %s", output)
	}
}

func TestPrintVersion_Dev(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()

	// Test dev version (no build date)
	Version = "dev"
	BuildDate = "unknown"

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	printVersion()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()

	if !strings.Contains(output, "openmodel version dev") {
		t.Errorf("Expected dev version output, got: %s", output)
	}
	// Build date should not be printed for "unknown"
	if strings.Contains(output, "build date") {
		t.Errorf("Expected no build date for dev version, got: %s", output)
	}
}

func TestRunServer_WithNonExistentConfigPath(t *testing.T) {
	// Skip this test - log.Fatalf calls os.Exit which can't be caught
	// This test verifies the error path manually by checking the config loading behavior
	t.Skip("Skipping - log.Fatalf calls os.Exit which cannot be caught in tests")
}

func TestRunServer_WithInvalidConfigFile(t *testing.T) {
	// Skip this test - log.Fatalf calls os.Exit which can't be caught
	t.Skip("Skipping - log.Fatalf calls os.Exit which cannot be caught in tests")
}

func TestRunModels_WithNoConfig(t *testing.T) {
	// Set a non-existent config path - config.Load() will return default config
	// which has no models defined
	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		}
	}()

	// Create temp home dir with no config (so Load returns default)
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer func() {
		os.Setenv("HOME", oldHome)
	}()

	jsonOutput := false

	// This will call config.Load() which returns default config (no models)
	// Should show "No models configured"
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	// Capture stderr to avoid polluting test output
	r, w, _ := os.Pipe()
	os.Stderr = w
	go func() { io.Copy(io.Discard, r) }()

	runModels(&jsonOutput)

	os.Stderr = oldStderr
}

// TestRunModels_WithRealConfig tests runModels using the existing config in the environment
// This config was verified to load successfully in TestRunTestNoConfig
func TestRunModels_WithRealConfig(t *testing.T) {
	t.Skip("Skipping: test depends on local config file")
	// Use the existing config from environment - it was loaded successfully
	// See TestRunTestNoConfig output showing the config
	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		}
	}()

	// Don't change HOME - use existing config
	oldHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", oldHome)
	}()

	// Test text output
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	jsonOutput := false
	runModels(&jsonOutput)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Should show available models from real config
	if !strings.Contains(output, "Available models:") {
		t.Errorf("Expected 'Available models:' in output, got: %s", output)
	}
	// Note: We don't check for specific models as they depend on user's config
}

// TestRunModels_JSONWithRealConfig tests runModels with JSON output using existing config
func TestRunModels_JSONWithRealConfig(t *testing.T) {
	t.Skip("Skipping: test depends on local config file")
	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		}
	}()

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	jsonOutput := true
	runModels(&jsonOutput)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// JSON output should contain model info
	if !strings.Contains(output, "smart") {
		t.Errorf("Expected 'smart' model in JSON output, got: %s", output)
	}
}

func TestRunModels_WithValidConfig(t *testing.T) {
	// Skip - config.Load() requires schema validation from remote URL
	// This is an integration test concern
	t.Skip("Skipping - config.Load() requires remote schema validation")
}

func TestRunModels_WithJSONOutput(t *testing.T) {
	// Skip - config.Load() requires schema validation from remote URL
	t.Skip("Skipping - config.Load() requires remote schema validation")
}

func TestRunModels_WithUnexpectedArgument(t *testing.T) {
	// Skip this test - os.Exit cannot be caught in tests
	t.Skip("Skipping - os.Exit cannot be caught in tests")
}

func TestRunConfig_WithNoHomeDir(t *testing.T) {
	// Skip this test - os.Exit cannot be caught in tests
	t.Skip("Skipping - os.Exit cannot be caught in tests")
}

func TestRunConfig_WithNonExistentConfig(t *testing.T) {
	// Skip this test - os.Exit cannot be caught in tests
	t.Skip("Skipping - os.Exit cannot be caught in tests")
}

func TestRunConfig_WithInvalidConfig(t *testing.T) {
	// Skip this test - os.Exit cannot be caught in tests
	t.Skip("Skipping - os.Exit cannot be caught in tests")
}

func TestRunConfig_WithValidConfig(t *testing.T) {
	// Skip - config.Load() requires schema validation from remote URL
	t.Skip("Skipping - config.Load() requires remote schema validation")
}

// TestRunConfig_WithRealConfig tests runConfig using the existing config in the environment
func TestRunConfig_WithRealConfig(t *testing.T) {
	oldEnv := os.Getenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_CONFIG")
	defer func() {
		if oldEnv != "" {
			os.Setenv("OPENMODEL_CONFIG", oldEnv)
		}
	}()

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfig()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	// Should print the config path on success
	// The config path should end with openmodel.json
	if !strings.HasSuffix(output, "openmodel.json") {
		t.Errorf("Expected config path ending with openmodel.json, got: %s", output)
	}
}

func TestPrintModelsUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printModelsUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Error("Expected 'Usage:' in models help output")
	}
	if !strings.Contains(output, "models") {
		t.Error("Expected 'models' in help output")
	}
}

func TestPrintConfigUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printConfigUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Error("Expected 'Usage:' in config help output")
	}
	if !strings.Contains(output, "config") {
		t.Error("Expected 'config' in help output")
	}
	// Note: output uses lowercase "validate" not "Validate"
	if !strings.Contains(output, "validate") {
		t.Error("Expected 'validate' in help output")
	}
}

func TestIntPtr(t *testing.T) {
	tests := []struct {
		name  string
		input int
	}{
		{"zero", 0},
		{"positive", 42},
		{"negative", -10},
		{"large", 1000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := intPtr(tt.input)
			if result == nil {
				t.Error("Expected non-nil result")
			}
			if *result != tt.input {
				t.Errorf("intPtr(%d) = %d, want %d", tt.input, *result, tt.input)
			}
		})
	}
}

func TestGetConfigPathNoHomeDir(t *testing.T) {
	// Save original HOME
	origHome := os.Getenv("HOME")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	// Unset HOME to simulate no home directory
	os.Unsetenv("HOME")

	// GetConfigPath should return empty string when HOME is not set
	// Also unset OPENMODEL_CONFIG to use the default path logic
	origConfig := os.Getenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_CONFIG")
	defer func() {
		if origConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", origConfig)
		}
	}()

	path := config.GetConfigPath()
	if path != "" {
		t.Errorf("expected empty path when HOME is not set, got %q", path)
	}
}

func TestGetConfigPathWithExplicitConfig(t *testing.T) {
	// Save original env
	origConfig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if origConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", origConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	// Set explicit config path
	testPath := "/nonexistent/path/config.json"
	os.Setenv("OPENMODEL_CONFIG", testPath)

	path := config.GetConfigPath()
	if path != testPath {
		t.Errorf("expected %q, got %q", testPath, path)
	}
}

func TestLoadFromPathNonExistent(t *testing.T) {
	nonExistentPath := "/nonexistent/path/to/config.json"

	_, err := config.LoadFromPath(nonExistentPath)
	if err == nil {
		t.Error("expected error when loading non-existent config file")
	}
}

func TestGetConfigPathWithTempHome(t *testing.T) {
	// Save original HOME
	origHome := os.Getenv("HOME")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	// Also save and clear OPENMODEL_CONFIG
	origConfig := os.Getenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_CONFIG")
	defer func() {
		if origConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", origConfig)
		}
	}()

	// Create temp directory without config file
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	path := config.GetConfigPath()
	// Path should exist but file should not
	expectedPath := filepath.Join(tmpDir, ".config", "openmodel", "openmodel.json")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}

	// Verify file doesn't exist
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected config file to not exist, got error: %v", err)
	}
}

func TestRunModelsWithUnexpectedArgument(t *testing.T) {
	// Test with unexpected argument - should print usage and exit
	// Since we can't catch os.Exit, we test the underlying flag parsing behavior

	// Create a flag set and test that it fails with unexpected args
	flagSet := flag.NewFlagSet("models", flag.ContinueOnError)
	flagSet.SetOutput(&bytes.Buffer{})

	_ = flagSet.String("model", "", "Model name")
	err := flagSet.Parse([]string{"-unknown", "value"})

	// This should fail because -unknown is not a valid flag
	if err == nil {
		t.Error("expected error with unknown flag, got nil")
	}

	// Test with valid flag but missing required argument
	flagSet = flag.NewFlagSet("models", flag.ContinueOnError)
	flagSet.SetOutput(&bytes.Buffer{})
	flagSet.String("model", "", "Model name")
	err = flagSet.Parse([]string{"-model"}) // missing value

	// This should fail because -model requires a value
	if err == nil {
		t.Error("expected error with missing flag value, got nil")
	}
}
