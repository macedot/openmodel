package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server"
	"github.com/macedot/openmodel/internal/state"
)

// initProviders creates and initializes all configured providers.
func initProviders(cfg *config.Config) map[string]provider.Provider {
	providers := make(map[string]provider.Provider)

	httpConfig := provider.HTTPConfig{
		TimeoutSeconds:               cfg.HTTP.TimeoutSeconds,
		MaxIdleConns:                 cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost:          cfg.HTTP.MaxIdleConnsPerHost,
		IdleConnTimeoutSeconds:       cfg.HTTP.IdleConnTimeoutSeconds,
		DialTimeoutSeconds:           cfg.HTTP.DialTimeoutSeconds,
		TLSHandshakeTimeoutSeconds:   cfg.HTTP.TLSHandshakeTimeoutSeconds,
		ResponseHeaderTimeoutSeconds: cfg.HTTP.ResponseHeaderTimeoutSeconds,
	}

	for name, pc := range cfg.Providers {
		providers[name] = provider.NewOpenAIProviderWithConfig(name, pc.URL, pc.APIKey, pc.ApiMode, httpConfig)
		logger.Info("Provider initialized", "name", name, "url", pc.URL, "api_mode", pc.ApiMode)
	}
	return providers
}

// loadAndValidateConfig loads config, initializes logger, validates, and returns cfg.
func loadAndValidateConfig(configPath string) (*config.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := logger.Init(cfg.LogLevel, ""); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Info("Config loaded", "config_path", cfg.GetConfigPath())
	if cfg.LogLevel != "" && cfg.LogLevel != "info" {
		logger.Debug("Log level set", "level", cfg.LogLevel)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration error:\n%w", err)
	}

	return cfg, nil
}

func mustLoadAndValidateConfig(configPath string) *config.Config {
	cfg, err := loadAndValidateConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	return cfg
}

func reloadServerConfig(srv *server.Server, configPath string) error {
	newCfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := newCfg.Validate(); err != nil {
		return err
	}
	return srv.ReloadConfig(newCfg)
}

func startConfigWatcher(configPath string, srv *server.Server) *config.Watcher {
	if configPath == "" {
		return nil
	}

	watcher := config.NewWatcher(configPath, func(newCfg *config.Config, err error) {
		if err != nil {
			logger.Error("config_reload_failed", "error", err)
			return
		}
		if err := srv.ReloadConfig(newCfg); err != nil {
			logger.Error("config_reload_failed", "error", err)
			return
		}
		logger.Info("config_reloaded_successfully")
	})

	if err := watcher.Start(); err != nil {
		logger.Warn("config_watcher_failed", "error", err)
		return nil
	}

	logger.Info("config_watcher_started", "path", configPath)
	return watcher
}

func startSignalHandler(ctx context.Context, cancel context.CancelFunc, srv *server.Server, configPath string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		defer signal.Stop(sigCh)

		for {
			sig := <-sigCh
			switch sig {
			case syscall.SIGHUP:
				logger.Info("SIGHUP_received_reloading_config")
				if configPath == "" {
					continue
				}
				if err := reloadServerConfig(srv, configPath); err != nil {
					logger.Error("config_reload_failed", "error", err)
					continue
				}
				logger.Info("config_reloaded_successfully")
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("Shutting_down")
				srv.Stop(ctx)
				cancel()
				return
			}
		}
	}()
}

// runServer starts the HTTP server with the given config.
func runServer(cfg *config.Config) {
	providers := initProviders(cfg)
	stateMgr := state.New(10000)
	srv := server.New(cfg, providers, stateMgr, Version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configPath := cfg.GetConfigPath()
	if watcher := startConfigWatcher(configPath, srv); watcher != nil {
		defer watcher.Stop()
	}

	startSignalHandler(ctx, cancel, srv, configPath)

	logger.Info("Starting_openmodel", "host", cfg.Server.Host, "port", cfg.Server.Port)
	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server_error", "error", err)
	}
}
