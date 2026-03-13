package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKnownSchemaChecksum_IsPinned(t *testing.T) {
	const schemaURL = "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json"

	checksum, ok := knownSchemaChecksums[schemaURL]
	assert.True(t, ok)
	assert.Len(t, checksum, 64)
	assert.NotEqual(t, "known-sha256-to-be-verified", checksum)
}

func TestConfigValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{
				"openai": {URL: "http://localhost:8080", ApiMode: "openai", Models: []string{"gpt-4"}},
			},
			Models: map[string]ModelConfig{
				"gpt-4": {
					Default:   true,
					Providers: []ModelProvider{{Provider: "openai", Model: "gpt-4"}},
				},
			},
		}

		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid provider reference", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{},
			Models: map[string]ModelConfig{
				"gpt-4": {Providers: []ModelProvider{{Provider: "missing", Model: "gpt-4"}}},
			},
		}

		assert.Error(t, cfg.Validate())
	})
}

// TestDefaultConfig tests the DefaultConfig function
func TestDefaultConfig(t *testing.T) {
	t.Run("returns valid config with defaults", func(t *testing.T) {
		cfg := DefaultConfig()

		// Verify server defaults
		if cfg.Server.Port != 12345 {
			t.Errorf("expected port 12345, got %d", cfg.Server.Port)
		}
		if cfg.Server.Host != "localhost" {
			t.Errorf("expected host localhost, got %s", cfg.Server.Host)
		}

		// Verify providers map is initialized
		if cfg.Providers == nil {
			t.Error("expected providers map to be initialized")
		}

		// Verify default provider
		local, ok := cfg.Providers["local"]
		if !ok {
			t.Error("expected 'local' provider to exist")
		}
		if local.URL != "http://localhost:11434/v1" {
			t.Errorf("expected local URL http://localhost:11434/v1, got %s", local.URL)
		}
		if local.APIKey != "" {
			t.Errorf("expected empty API key, got %s", local.APIKey)
		}

		// Verify thresholds defaults
		if cfg.Thresholds.FailuresBeforeSwitch != 3 {
			t.Errorf("expected failures_before_switch 3, got %d", cfg.Thresholds.FailuresBeforeSwitch)
		}
		if cfg.Thresholds.InitialTimeout != 10000 {
			t.Errorf("expected initial_timeout 10000, got %d", cfg.Thresholds.InitialTimeout)
		}
		if cfg.Thresholds.MaxTimeout != 300000 {
			t.Errorf("expected max_timeout 300000, got %d", cfg.Thresholds.MaxTimeout)
		}
	})

	t.Run("log level from environment", func(t *testing.T) {
		orig := os.Getenv("OPENMODEL_LOG_LEVEL")
		defer os.Setenv("OPENMODEL_LOG_LEVEL", orig)

		os.Setenv("OPENMODEL_LOG_LEVEL", "debug")
		cfg := DefaultConfig()
		if cfg.LogLevel != "debug" {
			t.Errorf("expected log level debug, got %s", cfg.LogLevel)
		}
	})
}

// TestExpandEnvVars tests the expandEnvVars function
func TestExpandEnvVars(t *testing.T) {
	origEnv := map[string]string{
		"TEST_VAR":      "testvalue",
		"ANOTHER_VAR":   "anothervalue",
		"EMPTY_VAR":     "",
		"SPECIAL_CHARS": "hello world!",
		"PATH_VAR":      "/usr/local/bin",
	}
	for k, v := range origEnv {
		os.Setenv(k, v)
	}
	defer func() {
		for k := range origEnv {
			os.Unsetenv(k)
		}
	}()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "no env vars", input: "hello world", expected: "hello world"},
		{name: "single env var", input: "prefix${TEST_VAR}suffix", expected: "prefixtestvaluesuffix"},
		{name: "multiple env vars", input: "${TEST_VAR}-${ANOTHER_VAR}", expected: "testvalue-anothervalue"},
		{name: "env var at start", input: "${TEST_VAR}suffix", expected: "testvaluesuffix"},
		{name: "env var at end", input: "prefix${TEST_VAR}", expected: "prefixtestvalue"},
		{name: "empty env var", input: "prefix${EMPTY_VAR}suffix", expected: "prefixsuffix"},
		{name: "env var with special chars", input: "value is ${SPECIAL_CHARS}", expected: "value is hello world!"},
		{name: "env var in path", input: "${PATH_VAR}/file", expected: "/usr/local/bin/file"},
		{name: "unclosed env var stays as is", input: "prefix${UNCLOSED", expected: "prefix${UNCLOSED"},
		{name: "empty braces expands to empty", input: "prefix${}suffix", expected: "prefixsuffix"},
		{name: "undefined env var expands to empty", input: "prefix${UNDEFINED_VAR}suffix", expected: "prefixsuffix"},
		{name: "first match expands var only", input: "prefix${TEST}VAR}suffix", expected: "prefixVAR}suffix"},
		{name: "multiple same env var", input: "${TEST_VAR} and ${TEST_VAR}", expected: "testvalue and testvalue"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := expandEnvVars(tc.input)
			if result != tc.expected {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestExpandProviderEnvVars tests the expandProviderEnvVars function
func TestExpandProviderEnvVars(t *testing.T) {
	defer os.Unsetenv("TEST_API_KEY")
	os.Setenv("TEST_API_KEY", "secret123")

	t.Run("expands API key and URL", func(t *testing.T) {
		pc := &ProviderConfig{
			URL:    "http://localhost:${PORT}/v1",
			APIKey: "${TEST_API_KEY}",
		}
		os.Setenv("PORT", "8080")
		defer os.Unsetenv("PORT")

		expandProviderEnvVars(pc)

		if pc.APIKey != "secret123" {
			t.Errorf("APIKey = %q, want %q", pc.APIKey, "secret123")
		}
		if pc.URL != "http://localhost:8080/v1" {
			t.Errorf("URL = %q, want %q", pc.URL, "http://localhost:8080/v1")
		}
	})
}

// TestLoadFromPath tests the LoadFromPath function
func TestLoadFromPath(t *testing.T) {
	t.Run("valid config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"server": {"port": 8080, "host": "0.0.0.0"},
			"providers": {"test": {"url": "http://localhost:8080/v1", "apiKey": ""}},
			"models": {},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 5, "initial_timeout_ms": 5000, "max_timeout_ms": 60000}
		}`
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		if err != nil {
			t.Fatalf("LoadFromPath() error = %v", err)
		}

		if cfg.Server.Port != 8080 {
			t.Errorf("Port = %d, want 8080", cfg.Server.Port)
		}
		if cfg.Server.Host != "0.0.0.0" {
			t.Errorf("Host = %q, want 0.0.0.0", cfg.Server.Host)
		}
		if cfg.Thresholds.FailuresBeforeSwitch != 5 {
			t.Errorf("FailuresBeforeSwitch = %d, want 5", cfg.Thresholds.FailuresBeforeSwitch)
		}
	})

	t.Run("expands env vars in provider config", func(t *testing.T) {
		os.Setenv("TEST_PROVIDER_URL", "http://expanded:9090/v1")
		defer os.Unsetenv("TEST_PROVIDER_URL")

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"server": {"port": 12345, "host": "localhost"},
			"providers": {"test": {"url": "${TEST_PROVIDER_URL}", "apiKey": ""}},
			"models": {},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		if err != nil {
			t.Fatalf("LoadFromPath() error = %v", err)
		}

		if cfg.Providers["test"].URL != "http://expanded:9090/v1" {
			t.Errorf("Provider URL = %q, want http://expanded:9090/v1", cfg.Providers["test"].URL)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadFromPath("/nonexistent/path/config.json")
		if err == nil {
			t.Error("LoadFromPath() expected error for missing file")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		if err := os.WriteFile(configPath, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromPath(configPath)
		if err == nil {
			t.Error("LoadFromPath() expected error for invalid JSON")
		}
	})

	t.Run("uses defaults when fields missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		if err != nil {
			t.Fatalf("LoadFromPath() error = %v", err)
		}

		if cfg.Server.Port != 12345 {
			t.Errorf("Port = %d, want default 12345", cfg.Server.Port)
		}
	})
}

// TestLoad tests the Load function with various scenarios
func TestLoad(t *testing.T) {
	origConfig := os.Getenv("OPENMODEL_CONFIG")
	origLogLevel := os.Getenv("OPENMODEL_LOG_LEVEL")
	defer func() {
		if origConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", origConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
		if origLogLevel != "" {
			os.Setenv("OPENMODEL_LOG_LEVEL", origLogLevel)
		} else {
			os.Unsetenv("OPENMODEL_LOG_LEVEL")
		}
	}()

	os.Unsetenv("OPENMODEL_CONFIG")
	os.Unsetenv("OPENMODEL_LOG_LEVEL")

	t.Run("no config file returns defaults", func(t *testing.T) {
		// Force nonexistent config path to test defaults
		os.Setenv("OPENMODEL_CONFIG", "/nonexistent/.config/openmodel/config.json")
		defer os.Unsetenv("OPENMODEL_CONFIG")

		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Server.Port != 12345 {
			t.Errorf("Port = %d, want default 12345", cfg.Server.Port)
		}
	})

	t.Run("config file not found returns defaults", func(t *testing.T) {
		os.Setenv("OPENMODEL_CONFIG", "/nonexistent/config.json")

		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Server.Port != 12345 {
			t.Errorf("Port = %d, want default 12345", cfg.Server.Port)
		}
	})

	t.Run("missing schema field", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{"server": {"port": 9000}, "providers": {}, "models": {}}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		os.Setenv("OPENMODEL_CONFIG", configPath)

		_, err := Load("")
		if err == nil {
			t.Error("Load() expected error for missing $schema field")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		if err := os.WriteFile(configPath, []byte("not valid json{"), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		os.Setenv("OPENMODEL_CONFIG", configPath)

		_, err := Load("")
		if err == nil {
			t.Error("Load() expected error for invalid JSON")
		}
	})
}

// TestGetConfigPath tests the GetConfigPath function
func TestGetConfigPath(t *testing.T) {
	orig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if orig != "" {
			os.Setenv("OPENMODEL_CONFIG", orig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	t.Run("explicit config path from env", func(t *testing.T) {
		os.Setenv("OPENMODEL_CONFIG", "/custom/path/config.json")
		path := GetConfigPath()
		if path != "/custom/path/config.json" {
			t.Errorf("GetConfigPath() = %q, want /custom/path/config.json", path)
		}
	})

	t.Run("default path when no env set", func(t *testing.T) {
		os.Unsetenv("OPENMODEL_CONFIG")
		path := GetConfigPath()
		if path == "" {
			t.Error("GetConfigPath() returned empty path")
		}
		if !filepath.IsAbs(path) {
			t.Errorf("GetConfigPath() returned relative path: %s", path)
		}
	})
}

// TestGetLogLevel tests the getLogLevel function
func TestGetLogLevel(t *testing.T) {
	orig := os.Getenv("OPENMODEL_LOG_LEVEL")
	defer func() {
		if orig != "" {
			os.Setenv("OPENMODEL_LOG_LEVEL", orig)
		} else {
			os.Unsetenv("OPENMODEL_LOG_LEVEL")
		}
	}()

	t.Run("from environment", func(t *testing.T) {
		os.Setenv("OPENMODEL_LOG_LEVEL", "debug")
		level := getLogLevel()
		if level != "debug" {
			t.Errorf("getLogLevel() = %q, want debug", level)
		}
	})

	t.Run("default when not set", func(t *testing.T) {
		os.Unsetenv("OPENMODEL_LOG_LEVEL")
		level := getLogLevel()
		if level != "info" {
			t.Errorf("getLogLevel() = %q, want info", level)
		}
	})
}

// TestGetSchemaCompiler tests the getSchemaCompiler function
func TestGetSchemaCompiler(t *testing.T) {
	t.Run("local schema file", func(t *testing.T) {
		// Create a minimal valid JSON schema
		tmpDir := t.TempDir()
		schemaPath := filepath.Join(tmpDir, "schema.json")
		schemaContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object"
		}`
		if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		_, err := getSchemaCompiler(schemaPath)
		if err != nil {
			t.Fatalf("getSchemaCompiler() error = %v", err)
		}
	})

	t.Run("nonexistent schema file", func(t *testing.T) {
		_, err := getSchemaCompiler("/nonexistent/schema.json")
		if err == nil {
			t.Error("getSchemaCompiler() expected error for nonexistent file")
		}
	})

	t.Run("cache hit returns cached compiler", func(t *testing.T) {
		// Create a minimal valid JSON schema
		tmpDir := t.TempDir()
		schemaPath := filepath.Join(tmpDir, "schema_cache.json")
		schemaContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object"
		}`
		if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		// First call - populates cache
		compiler1, err := getSchemaCompiler(schemaPath)
		if err != nil {
			t.Fatalf("first getSchemaCompiler() error = %v", err)
		}

		// Second call - should return cached compiler
		compiler2, err := getSchemaCompiler(schemaPath)
		if err != nil {
			t.Fatalf("second getSchemaCompiler() error = %v", err)
		}

		// Both should be the same instance (cache hit)
		if compiler1 != compiler2 {
			t.Error("expected same compiler instance on cache hit")
		}
	})

	t.Run("different schemas produce different compilers", func(t *testing.T) {
		// Create two minimal valid JSON schemas
		tmpDir := t.TempDir()
		schemaPath1 := filepath.Join(tmpDir, "schema1.json")
		schemaPath2 := filepath.Join(tmpDir, "schema2.json")
		schemaContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object"
		}`

		if err := os.WriteFile(schemaPath1, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to write schema1: %v", err)
		}
		if err := os.WriteFile(schemaPath2, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to write schema2: %v", err)
		}

		// Get compiler for first schema
		compiler1, err := getSchemaCompiler(schemaPath1)
		if err != nil {
			t.Fatalf("getSchemaCompiler(schema1) error = %v", err)
		}

		// Get compiler for second schema - should be different
		compiler2, err := getSchemaCompiler(schemaPath2)
		if err != nil {
			t.Fatalf("getSchemaCompiler(schema2) error = %v", err)
		}

		// Different schemas should produce different compilers
		if compiler1 == compiler2 {
			t.Error("expected different compiler instances for different schemas")
		}
	})
}

// TestGetSchemaCompilerConcurrent tests thread safety of the schema compiler cache
func TestGetSchemaCompilerConcurrent(t *testing.T) {
	// Create a minimal valid JSON schema
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema_concurrent.json")
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object"
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("failed to write schema: %v", err)
	}

	// Run multiple concurrent calls - should not panic
	// This tests the cache is thread-safe
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() {
				done <- true
			}()
			_, _ = getSchemaCompiler(schemaPath)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestLoad_SchemaValidationFailure tests that schema validation failures are detected
func TestLoad_SchemaValidationFailure(t *testing.T) {
	// Save original env
	origConfig := os.Getenv("OPENMODEL_CONFIG")
	defer func() {
		if origConfig != "" {
			os.Setenv("OPENMODEL_CONFIG", origConfig)
		} else {
			os.Unsetenv("OPENMODEL_CONFIG")
		}
	}()

	// Create temp directory with custom schema that validates port minimum
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.json")
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"properties": {
			"server": {
				"type": "object",
				"properties": {
					"port": {"type": "integer", "minimum": 1},
					"host": {"type": "string"}
				},
				"required": ["port", "host"]
			}
		},
		"required": ["server"]
	}`
	err := os.WriteFile(schemaPath, []byte(schemaContent), 0644)
	assert.NoError(t, err)

	// Create config that violates schema (port = 0 is below minimum of 1)
	configPath := filepath.Join(tmpDir, "config.json")
	configContent := fmt.Sprintf(`{
		"$schema": "%s",
		"server": {"port": 0, "host": "localhost"}
	}`, schemaPath)

	err = os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err)

	os.Setenv("OPENMODEL_CONFIG", configPath)

	_, err = Load("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

// TestGetSchemaCompiler_InvalidSchemaURL tests error handling for invalid schema URLs
func TestGetSchemaCompiler_InvalidSchemaURL(t *testing.T) {
	// Test with malformed schema URL (invalid host that will fail to connect)
	_, err := getSchemaCompiler("http://invalid:9999/schema.json")
	assert.Error(t, err)
}

// BenchmarkDefaultConfig benchmarks the DefaultConfig function - this is the hot path
// as it's called on every server startup
func BenchmarkDefaultConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}

// TestModelValidation tests model reference validation
func TestModelValidation(t *testing.T) {
	t.Run("valid provider/model format", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1", "model2"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
				"providers": ["test/model1"]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		assert.NoError(t, err)
		assert.Len(t, cfg.Models["my-model"].Providers, 1)
		assert.Equal(t, "test", cfg.Models["my-model"].Providers[0].Provider)
		assert.Equal(t, "model1", cfg.Models["my-model"].Providers[0].Model)
	})

	t.Run("invalid provider reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
				"providers": ["unknown/model1"]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromPath(configPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider")
	})

	t.Run("invalid model reference in provider list", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1", "model2"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
				"providers": ["test/nonexistent"]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromPath(configPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in provider")
	})

	t.Run("valid own model reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1", "model2"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
					"providers": ["model1"]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		assert.NoError(t, err)
		assert.Len(t, cfg.Models["my-model"].Providers, 1)
		assert.Equal(t, "test", cfg.Models["my-model"].Providers[0].Provider)
		assert.Equal(t, "model1", cfg.Models["my-model"].Providers[0].Model)
	})

	t.Run("invalid own model reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1", "model2"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
					"providers": ["nonexistent"]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromPath(configPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in any provider's models list")
	})

	t.Run("provider/model without models list definition passes", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1"
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
				"providers": ["test/any-model"]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		assert.NoError(t, err)
		assert.Len(t, cfg.Models["my-model"].Providers, 1)
		assert.Equal(t, "test", cfg.Models["my-model"].Providers[0].Provider)
		assert.Equal(t, "any-model", cfg.Models["my-model"].Providers[0].Model)
	})

	t.Run("valid object format model reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
					"providers": [{"provider": "test", "model": "model1"}]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		assert.NoError(t, err)
		assert.Len(t, cfg.Models["my-model"].Providers, 1)
		assert.Equal(t, "test", cfg.Models["my-model"].Providers[0].Provider)
		assert.Equal(t, "model1", cfg.Models["my-model"].Providers[0].Model)
	})

	t.Run("invalid object format model reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")
		configContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"server": {"port": 12345, "host": "localhost"},
			"providers": {
				"test": {
					"url": "http://localhost:8080/v1",
					"models": ["model1"]
				}
			},
			"models": {
				"my-model": {
					"strategy": "fallback",
					"providers": [{"provider": "test", "model": "nonexistent"}]
				}
			},
			"log_level": "info",
			"thresholds": {"failures_before_switch": 3, "initial_timeout_ms": 10000, "max_timeout_ms": 300000}
		}`

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadFromPath(configPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in provider")
	})
}

// TestValidateProviderReferences tests the ValidateProviderReferences function
func TestValidateProviderReferences(t *testing.T) {
	t.Run("valid references", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{
				"openai": {URL: "https://api.openai.com/v1"},
				"ollama": {URL: "http://localhost:11434/v1"},
			},
			Models: map[string]ModelConfig{
				"gpt-4": {
					Strategy: StrategyFallback,
					Providers: []ModelProvider{
						{Provider: "openai", Model: "gpt-4"},
						{Provider: "ollama", Model: "llama2"},
					},
				},
			},
		}

		err := cfg.ValidateProviderReferences()
		assert.NoError(t, err)
	})

	t.Run("missing provider reference", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{
				"openai": {URL: "https://api.openai.com/v1"},
			},
			Models: map[string]ModelConfig{
				"gpt-4": {
					Strategy: StrategyFallback,
					Providers: []ModelProvider{
						{Provider: "openai", Model: "gpt-4"},
						{Provider: "missing", Model: "claude"}, // This provider doesn't exist
					},
				},
			},
		}

		err := cfg.ValidateProviderReferences()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
		assert.Contains(t, err.Error(), `"gpt-4"`)
	})

	t.Run("multiple missing providers", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{
				"local": {URL: "http://localhost:11434/v1"},
			},
			Models: map[string]ModelConfig{
				"model1": {
					Strategy: StrategyFallback,
					Providers: []ModelProvider{
						{Provider: "missing1", Model: "m1"},
					},
				},
				"model2": {
					Strategy: StrategyFallback,
					Providers: []ModelProvider{
						{Provider: "missing2", Model: "m2"},
					},
				},
			},
		}

		err := cfg.ValidateProviderReferences()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing1")
		assert.Contains(t, err.Error(), "missing2")
	})

	t.Run("empty models", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{
				"openai": {URL: "https://api.openai.com/v1"},
			},
			Models: map[string]ModelConfig{},
		}

		err := cfg.ValidateProviderReferences()
		assert.NoError(t, err)
	})

	t.Run("empty providers", func(t *testing.T) {
		cfg := &Config{
			Providers: map[string]ProviderConfig{},
			Models: map[string]ModelConfig{
				"gpt-4": {
					Strategy: StrategyFallback,
					Providers: []ModelProvider{
						{Provider: "openai", Model: "gpt-4"},
					},
				},
			},
		}

		err := cfg.ValidateProviderReferences()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "openai")
	})
}

func TestValidateDefaultModels(t *testing.T) {
	t.Run("no models with default", func(t *testing.T) {
		cfg := &Config{
			Models: map[string]ModelConfig{
				"model-a": {Strategy: "fallback"},
				"model-b": {Strategy: "fallback"},
			},
		}
		err := cfg.ValidateDefaultModels()
		assert.NoError(t, err)
	})

	t.Run("single model with default", func(t *testing.T) {
		cfg := &Config{
			Models: map[string]ModelConfig{
				"model-a": {Strategy: "fallback", Default: true},
				"model-b": {Strategy: "fallback"},
			},
		}
		err := cfg.ValidateDefaultModels()
		assert.NoError(t, err)
	})

	t.Run("multiple models with default", func(t *testing.T) {
		cfg := &Config{
			Models: map[string]ModelConfig{
				"model-a": {Strategy: "fallback", Default: true},
				"model-b": {Strategy: "fallback", Default: true},
			},
		}
		err := cfg.ValidateDefaultModels()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "multiple models marked as default")
		assert.Contains(t, err.Error(), "model-a")
		assert.Contains(t, err.Error(), "model-b")
	})

	t.Run("three models with default", func(t *testing.T) {
		cfg := &Config{
			Models: map[string]ModelConfig{
				"model-a": {Strategy: "fallback", Default: true},
				"model-b": {Strategy: "fallback", Default: true},
				"model-c": {Strategy: "fallback", Default: true},
			},
		}
		err := cfg.ValidateDefaultModels()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "multiple models marked as default")
	})

	t.Run("empty models", func(t *testing.T) {
		cfg := &Config{
			Models: map[string]ModelConfig{},
		}
		err := cfg.ValidateDefaultModels()
		assert.NoError(t, err)
	})
}
