package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server"
	"github.com/macedot/openmodel/internal/state"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	providers := make(map[string]provider.Provider)
	for name, pc := range cfg.Providers {
		switch pc.Type {
		case "ollama":
			providers[name] = provider.NewOllamaProvider(pc.URL)
		case "opencodezen":
			providers[name] = provider.NewOpenCodeZenProviderWithURL(pc.URL, pc.APIKey)
		}
	}

	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := server.New(cfg, providers, stateMgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")
		srv.Stop(ctx)
		cancel()
	}()

	log.Printf("Starting openmodel on %s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
