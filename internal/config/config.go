// Package config handles JSON configuration loading
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config represents the openmodel configuration
type Config struct {
	Server     ServerConfig              `json:"server"`
	Providers  map[string]ProviderConfig `json:"providers"`
	Models     map[string][]ModelBackend `json:"models"`
	LogLevel   string                    `json:"log_level"`
	Thresholds ThresholdsConfig          `json:"thresholds"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// ProviderConfig holds provider connection settings
type ProviderConfig struct {
	Type   string   `json:"type"`   // "ollama" or "opencodezen"
	URL    string   `json:"url"`    // Base URL for the provider
	APIKey string   `json:"apiKey"` // API key (supports ${VAR} expansion)
	Models []string `json:"models"` // Available models for this provider
}

// ModelBackend represents a backend model in the chain
type ModelBackend struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// ThresholdsConfig holds failure threshold settings
type ThresholdsConfig struct {
	FailuresBeforeSwitch int `json:"failures_before_switch"`
	InitialTimeout       int `json:"initial_timeout_ms"`
	MaxTimeout           int `json:"max_timeout_ms"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 11435,
			Host: "localhost",
		},
		Providers: map[string]ProviderConfig{
			"ollama": {
				Type:   "ollama",
				URL:    "http://localhost:11434",
				Models: []string{},
			},
		},
		LogLevel: "info",
		Thresholds: ThresholdsConfig{
			FailuresBeforeSwitch: 3,
			InitialTimeout:       10000,
			MaxTimeout:           300000,
		},
	}
}

// expandEnvVars expands environment variables in ${VAR} format
func expandEnvVars(s string) string {
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start
		varName := s[start+2 : end]
		envValue := os.Getenv(varName)
		s = s[:start] + envValue + s[end+1:]
	}
	return s
}

// expandProviderEnvVars expands environment variables in provider config
func expandProviderEnvVars(pc *ProviderConfig) {
	pc.APIKey = expandEnvVars(pc.APIKey)
	pc.URL = expandEnvVars(pc.URL)
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	// Check for explicit config path in env
	if path := os.Getenv("OPENMODEL_CONFIG"); path != "" {
		return path
	}
	// Default to ~/.config/openmodel/config.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "openmodel", "config.json")
}

// Load loads configuration from file
func Load() (*Config, error) {
	configPath := getConfigPath()
	if configPath == "" {
		return DefaultConfig(), nil
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Start with defaults and merge
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand environment variables in all provider configs
	for name, provider := range cfg.Providers {
		expandProviderEnvVars(&provider)
		cfg.Providers[name] = provider
	}

	return cfg, nil
}

// LoadFromPath loads configuration from a specific path
func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand environment variables in all provider configs
	for name, provider := range cfg.Providers {
		expandProviderEnvVars(&provider)
		cfg.Providers[name] = provider
	}

	return cfg, nil
}
