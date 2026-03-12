package config

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testConfigSchema = `"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",`

func TestNewWatcher(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create a minimal valid config
	configContent := `{
		"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
		"server": {"port": 12345, "host": "localhost"},
		"providers": {
			"test": {"url": "http://localhost:8080/v1", "api_mode": "openai", "api_key": "test-key"}
		},
		"models": {}
	}`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	var callbackCalled atomic.Bool
	var callbackMu sync.Mutex
	var callbackCfg *Config

	watcher := NewWatcher(configPath, func(cfg *Config, err error) {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		callbackCalled.Store(true)
		callbackCfg = cfg
	})

	assert.NotNil(t, watcher)
	assert.False(t, watcher.IsRunning())

	// Test Start
	err = watcher.Start()
	require.NoError(t, err)
	assert.True(t, watcher.IsRunning())

	// Test double start (should be idempotent)
	err = watcher.Start()
	require.NoError(t, err)
	assert.True(t, watcher.IsRunning())

	// Test Stop
	watcher.Stop()
	assert.False(t, watcher.IsRunning())

	// Test double stop (should be idempotent)
	watcher.Stop()
	assert.False(t, watcher.IsRunning())

	// Suppress unused warnings
	_ = callbackCalled.Load()
	_ = callbackCfg
}

func TestWatcherHandleConfigChange(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial valid config
	validConfig := `{
		"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
		"server": {"port": 12345, "host": "localhost"},
		"providers": {
			"test": {"url": "http://localhost:8080/v1", "api_mode": "openai", "api_key": "test-key"}
		},
		"models": {}
	}`
	err := os.WriteFile(configPath, []byte(validConfig), 0644)
	require.NoError(t, err)

	var callbackMu sync.Mutex
	var callbacks []struct {
		cfg *Config
		err error
	}

	watcher := NewWatcher(configPath, func(cfg *Config, err error) {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		callbacks = append(callbacks, struct {
			cfg *Config
			err error
		}{cfg: cfg, err: err})
	})

	// Test valid config change
	watcher.handleConfigChange()

	callbackMu.Lock()
	require.Len(t, callbacks, 1)
	assert.NotNil(t, callbacks[0].cfg)
	assert.NoError(t, callbacks[0].err)
	assert.Contains(t, callbacks[0].cfg.Providers, "test")
	callbackMu.Unlock()

	// Test invalid config
	invalidConfig := `{invalid json}`
	err = os.WriteFile(configPath, []byte(invalidConfig), 0644)
	require.NoError(t, err)

	// Clear previous callbacks
	callbackMu.Lock()
	callbacks = nil
	callbackMu.Unlock()

	watcher.handleConfigChange()

	callbackMu.Lock()
	require.Len(t, callbacks, 1)
	assert.Nil(t, callbacks[0].cfg)
	assert.Error(t, callbacks[0].err)
	callbackMu.Unlock()

	// Test config with invalid provider reference
	configWithInvalidRef := `{
		"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
		"server": {"port": 12345, "host": "localhost"},
		"providers": {
			"test": {"url": "http://localhost:8080/v1", "api_mode": "openai", "api_key": "test-key"}
		},
		"models": {
			"test-model": {
				"strategy": "fallback",
				"providers": ["nonexistent-provider/model"]
			}
		}
	}`
	err = os.WriteFile(configPath, []byte(configWithInvalidRef), 0644)
	require.NoError(t, err)

	callbackMu.Lock()
	callbacks = nil
	callbackMu.Unlock()

	watcher.handleConfigChange()

	callbackMu.Lock()
	require.Len(t, callbacks, 1)
	assert.Nil(t, callbacks[0].cfg)
	assert.Error(t, callbacks[0].err)
	callbackMu.Unlock()
}

func TestWatcherConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	validConfig := `{
		"$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
		"server": {"port": 12345, "host": "localhost"},
		"providers": {
			"test": {"url": "http://localhost:8080/v1", "api_mode": "openai", "api_key": "test-key"}
		},
		"models": {}
	}`
	err := os.WriteFile(configPath, []byte(validConfig), 0644)
	require.NoError(t, err)

	var callbackCount atomic.Int32
	watcher := NewWatcher(configPath, func(cfg *Config, err error) {
		callbackCount.Add(1)
	})

	// Start the watcher
	err = watcher.Start()
	require.NoError(t, err)

	// Concurrent Start/Stop operations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			watcher.Start()
			watcher.IsRunning()
			watcher.Stop()
		}()
	}
	wg.Wait()

	// Verify clean state
	assert.False(t, watcher.IsRunning())
}

func TestWatcherNonExistentFile(t *testing.T) {
	var callbackMu sync.Mutex
	var callbackErr error

	watcher := NewWatcher("/nonexistent/path/config.json", func(cfg *Config, err error) {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		callbackErr = err
	})

	// Should fail to start because file doesn't exist
	err := watcher.Start()
	assert.Error(t, err)
	assert.False(t, watcher.IsRunning())

	// Suppress unused warning
	_ = callbackErr
}
