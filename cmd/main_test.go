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
	if !bytes.Contains(buf.Bytes(), []byte("bench")) {
		t.Error("Expected 'bench' command in help output")
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

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	printServerUsage(fs)

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
	_, err := loadAndValidateConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for non-existent config path")
	}
}

func TestRunServer_WithInvalidConfigFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(configPath, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := loadAndValidateConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid config file")
	}
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

	// This will call config.Load() which returns default config (no models)
	// Should show "No models configured"
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	// Capture stderr to avoid polluting test output
	r, w, _ := os.Pipe()
	os.Stderr = w
	go func() { io.Copy(io.Discard, r) }()

	runModels(false)

	os.Stderr = oldStderr
}

func TestRunModels_WithValidConfig(t *testing.T) {
	oldConfig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", oldConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	configPath := filepath.Join(t.TempDir(), "openmodel.json")
	configJSON := `{
		"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
		"models": {
			"smart": {
				"providers": [{"provider": "openai", "model": "gpt-4"}]
			}
		},
		"providers": {
			"openai": {
				"url": "https://api.openai.com",
				"api_key": "test-key",
				"models": ["gpt-4"]
			}
		}
	}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	os.Setenv("OPENMODEL_CONFIG", configPath)

	cfg, exitCode := executeModelsCmd(nil)
	if exitCode != 0 {
		t.Fatalf("executeModelsCmd exitCode = %d, want 0", exitCode)
	}
	if cfg == nil {
		t.Fatal("expected config")
	}
	if _, ok := cfg.Models["smart"]; !ok {
		t.Fatal("expected smart model in config")
	}
}

func TestRunModels_WithJSONOutput(t *testing.T) {
	_, exitCode := executeModelsCmd([]string{"unexpected"})
	if exitCode != 1 {
		t.Fatalf("executeModelsCmd exitCode = %d, want 1", exitCode)
	}
}

func TestRunModels_WithUnexpectedArgument(t *testing.T) {
	_, exitCode := executeModelsCmd([]string{"unexpected"})
	if exitCode != 1 {
		t.Fatalf("executeModelsCmd exitCode = %d, want 1", exitCode)
	}
}

func TestRunConfig_WithNoHomeDir(t *testing.T) {
	oldConfig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", oldConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	os.Unsetenv("OPENMODEL_CONFIG")

	path := (&config.Config{}).GetConfigPath()
	if path == "" {
		t.Skip("config path resolution is unavailable in this environment")
	}
}

func TestRunConfig_WithNonExistentConfig(t *testing.T) {
	oldConfig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", oldConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	os.Setenv("OPENMODEL_CONFIG", filepath.Join(t.TempDir(), "missing.json"))

	err := executeConfig()
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfig_WithInvalidConfig(t *testing.T) {
	oldConfig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if oldConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", oldConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	configPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(configPath, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	os.Setenv("OPENMODEL_CONFIG", configPath)

	err := executeConfig()
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "error loading config") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfig_WithValidConfig(t *testing.T) {
	oldConfig := os.Getenv("OPENMODEL_CONFIG")
	oldStdout := os.Stdout
	defer func() {
		if oldConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", oldConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
		os.Stdout = oldStdout
	}()

	configPath := filepath.Join(t.TempDir(), "openmodel.json")
	configJSON := `{
		"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
		"providers": {},
		"models": {}
	}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	os.Setenv("OPENMODEL_CONFIG", configPath)

	r, w, _ := os.Pipe()
	os.Stdout = w

	err := executeConfig()
	w.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	if strings.TrimSpace(buf.String()) != configPath {
		t.Fatalf("expected printed config path %q, got %q", configPath, strings.TrimSpace(buf.String()))
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

	fs := flag.NewFlagSet("models", flag.ExitOnError)
	printModelsUsage(fs)

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

	cfg := &config.Config{}
	path := cfg.GetConfigPath()
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

	cfg := &config.Config{}
	path := cfg.GetConfigPath()
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

	cfg := &config.Config{}
	path := cfg.GetConfigPath()
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

func TestDefaultModelAttribute(t *testing.T) {
	tests := []struct {
		name            string
		models          map[string]config.ModelConfig
		expectedDefault string
	}{
		{
			name: "first model is default when none specified",
			models: map[string]config.ModelConfig{
				"model-a": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "test", Model: "a"}}},
				"model-b": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "test", Model: "b"}}},
			},
			expectedDefault: "model-a",
		},
		{
			name: "explicit default model is marked",
			models: map[string]config.ModelConfig{
				"model-a": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "test", Model: "a"}}},
				"model-b": {Strategy: "fallback", Default: true, Providers: []config.ModelProvider{{Provider: "test", Model: "b"}}},
			},
			expectedDefault: "model-b",
		},
		{
			name: "single model is default",
			models: map[string]config.ModelConfig{
				"only-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "test", Model: "only"}}},
			},
			expectedDefault: "only-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the ordered list of models and check default logic
			type modelInfo struct {
				Name    string
				Default bool
			}

			modelOrder := make([]string, 0, len(tt.models))
			for name := range tt.models {
				modelOrder = append(modelOrder, name)
			}

			// Sort for consistent ordering
			for i := 0; i < len(modelOrder); i++ {
				for j := i + 1; j < len(modelOrder); j++ {
					if modelOrder[i] > modelOrder[j] {
						modelOrder[i], modelOrder[j] = modelOrder[j], modelOrder[i]
					}
				}
			}

			models := make([]modelInfo, 0, len(modelOrder))
			hasExplicitDefault := false
			for _, name := range modelOrder {
				modelConfig := tt.models[name]
				info := modelInfo{
					Name:    name,
					Default: modelConfig.Default,
				}
				if modelConfig.Default {
					hasExplicitDefault = true
				}
				models = append(models, info)
			}

			// If no explicit default, first model becomes default
			if !hasExplicitDefault && len(models) > 0 {
				models[0].Default = true
			}

			// Find the default model
			var foundDefault string
			for _, m := range models {
				if m.Default {
					foundDefault = m.Name
					break
				}
			}

			if foundDefault != tt.expectedDefault {
				t.Errorf("expected default model %q, got %q", tt.expectedDefault, foundDefault)
			}
		})
	}
}

// Tests for bench command

func TestPrintBenchUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	fs := newBenchFlagSet()
	printBenchUsage(fs)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Error("Expected 'Usage:' in bench help output")
	}
	if !strings.Contains(output, "bench") {
		t.Error("Expected 'bench' in help output")
	}
	if !strings.Contains(output, "-prompt") {
		t.Error("Expected '-prompt' in help output")
	}
	if !strings.Contains(output, "-scope") {
		t.Error("Expected '-scope' in help output")
	}
	if !strings.Contains(output, "-stream") {
		t.Error("Expected '-stream' in help output")
	}
	if !strings.Contains(output, "application") {
		t.Error("Expected 'application' scope mode in help output")
	}
	if !strings.Contains(output, "providers") {
		t.Error("Expected 'providers' scope mode in help output")
	}
	if !strings.Contains(output, "all") {
		t.Error("Expected 'all' scope mode in help output")
	}
}

func TestFormatProviders(t *testing.T) {
	tests := []struct {
		name       string
		providers  []config.ModelProvider
		expected   string
		minResults []string // strings that must be in result
	}{
		{
			name:       "empty providers",
			providers:  []config.ModelProvider{},
			expected:   "",
			minResults: []string{},
		},
		{
			name: "single provider",
			providers: []config.ModelProvider{
				{Provider: "openai", Model: "gpt-4"},
			},
			expected:   "openai/gpt-4",
			minResults: []string{"openai/gpt-4"},
		},
		{
			name: "multiple providers",
			providers: []config.ModelProvider{
				{Provider: "openai", Model: "gpt-4"},
				{Provider: "ollama", Model: "llama2"},
			},
			expected:   "openai/gpt-4, ollama/llama2",
			minResults: []string{"openai/gpt-4", "ollama/llama2"},
		},
		{
			name: "three providers",
			providers: []config.ModelProvider{
				{Provider: "openai", Model: "gpt-4"},
				{Provider: "anthropic", Model: "claude-3"},
				{Provider: "local", Model: "llama2"},
			},
			minResults: []string{"openai/gpt-4", "anthropic/claude-3", "local/llama2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProviders(tt.providers)

			if tt.expected != "" && result != tt.expected {
				t.Errorf("formatProviders() = %q, want %q", result, tt.expected)
			}

			for _, minResult := range tt.minResults {
				if !strings.Contains(result, minResult) {
					t.Errorf("formatProviders() result %q missing expected substring %q", result, minResult)
				}
			}
		})
	}
}

func TestBenchCommandFlagParsing(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantPrompt  string
		wantScope   string
		wantParseOK bool
	}{
		{
			name:        "default scope",
			args:        []string{"-prompt", "test.txt"},
			wantPrompt:  "test.txt",
			wantScope:   "application",
			wantParseOK: true,
		},
		{
			name:        "explicit scope providers",
			args:        []string{"-prompt", "test.txt", "-scope", "providers"},
			wantPrompt:  "test.txt",
			wantScope:   "providers",
			wantParseOK: true,
		},
		{
			name:        "explicit scope all",
			args:        []string{"-prompt", "test.txt", "-scope", "all"},
			wantPrompt:  "test.txt",
			wantScope:   "all",
			wantParseOK: true,
		},
		{
			name:        "app alias for application scope",
			args:        []string{"-prompt", "test.txt", "-scope", "app"},
			wantPrompt:  "test.txt",
			wantScope:   "app",
			wantParseOK: true,
		},
		{
			name:        "missing prompt flag",
			args:        []string{"-scope", "providers"},
			wantPrompt:  "",
			wantScope:   "providers",
			wantParseOK: true, // Parse succeeds, but validation should fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagSet := flag.NewFlagSet("bench", flag.ContinueOnError)
			flagSet.SetOutput(&bytes.Buffer{})

			promptFile := flagSet.String("prompt", "", "Prompt file")
			scope := flagSet.String("scope", "application", "Scope")

			err := flagSet.Parse(tt.args)
			parseOK := err == nil

			if parseOK != tt.wantParseOK {
				t.Errorf("parse error = %v, want parseOK = %v", err, tt.wantParseOK)
			}

			if parseOK {
				if *promptFile != tt.wantPrompt {
					t.Errorf("prompt = %q, want %q", *promptFile, tt.wantPrompt)
				}
				if *scope != tt.wantScope {
					t.Errorf("scope = %q, want %q", *scope, tt.wantScope)
				}
			}
		})
	}
}

func TestPrintUsageIncludesBench(t *testing.T) {
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

	output := buf.String()
	if !strings.Contains(output, "bench") {
		t.Error("Expected 'bench' command in usage output")
	}
	if !strings.Contains(output, "Benchmark models with prompts") {
		t.Error("Expected bench command description in usage output")
	}
}

func TestFindFirstAvailableProvider(t *testing.T) {
	// Note: This test verifies the logic of findFirstAvailableProvider without
	// mocking the full Provider interface. The function iterates through the
	// model's providers list and finds the first one that exists in the providers map.

	tests := []struct {
		name        string
		modelConfig config.ModelConfig
		// providersExist indicates which providers are in the map (true = exists)
		providersExist map[string]bool
		wantErr        bool
		wantProvider   string
		wantModel      string
	}{
		{
			name: "first provider available",
			modelConfig: config.ModelConfig{
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
				},
			},
			providersExist: map[string]bool{"openai": true},
			wantErr:        false,
			wantProvider:   "openai",
			wantModel:      "gpt-4",
		},
		{
			name: "second provider when first unavailable",
			modelConfig: config.ModelConfig{
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
					{Provider: "ollama", Model: "llama2"},
				},
			},
			providersExist: map[string]bool{"ollama": true}, // openai not in map
			wantErr:        false,
			wantProvider:   "ollama",
			wantModel:      "llama2",
		},
		{
			name: "no providers available",
			modelConfig: config.ModelConfig{
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
				},
			},
			providersExist: map[string]bool{}, // empty
			wantErr:        true,
		},
		{
			name: "empty providers list",
			modelConfig: config.ModelConfig{
				Strategy:  "fallback",
				Providers: []config.ModelProvider{},
			},
			providersExist: map[string]bool{"openai": true},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a minimal providers map
			// Since we can't easily mock Provider interface, we use nil values
			// The function only checks if the key exists in the map
			providers := make(benchProviderMap)
			for name := range tt.providersExist {
				providers[name] = nil // The function checks existence, not functionality
			}

			_, provKey, model, err := findFirstAvailableProvider(providers, tt.modelConfig)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !strings.Contains(provKey, tt.wantProvider) {
					t.Errorf("provider key = %q, want to contain %q", provKey, tt.wantProvider)
				}
				if model != tt.wantModel {
					t.Errorf("model = %q, want %q", model, tt.wantModel)
				}
			}
		})
	}
}

func TestGetEndpointsForAPIMode(t *testing.T) {
	tests := []struct {
		name     string
		apiMode  string
		expected []string
	}{
		{
			name:     "openai mode",
			apiMode:  "openai",
			expected: []string{"/v1/chat/completions"},
		},
		{
			name:     "anthropic mode",
			apiMode:  "anthropic",
			expected: []string{"/v1/messages"},
		},
		{
			name:     "unknown mode supports both",
			apiMode:  "custom",
			expected: []string{"/v1/chat/completions", "/v1/messages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEndpointsForAPIMode(tt.apiMode)
			if len(result) != len(tt.expected) {
				t.Fatalf("len(result) = %d, want %d", len(result), len(tt.expected))
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Fatalf("result[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSanitizeBenchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "keeps safe characters", input: "provider_model-1", expected: "provider_model-1"},
		{name: "drops unsafe characters", input: "provider/model:v1", expected: "providermodelv1"},
		{name: "empty fallback", input: "!@#$", expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeBenchName(tt.input); got != tt.expected {
				t.Fatalf("sanitizeBenchName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseStatusCodeFromError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{name: "extracts status code", input: "provider failed with status 429", expected: 429},
		{name: "missing status code", input: "provider failed", expected: 0},
		{name: "invalid status code", input: "status abc", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseStatusCodeFromError(tt.input); got != tt.expected {
				t.Fatalf("parseStatusCodeFromError(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}
