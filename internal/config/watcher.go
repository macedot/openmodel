// Package config handles configuration loading and hot reload
package config

import (
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches for config file changes
type Watcher struct {
	configPath string
	watcher    *fsnotify.Watcher
	callback   func(*Config)
	mu         sync.RWMutex
	stopCh     chan struct{}
	running    bool
}

// NewWatcher creates a new config watcher
func NewWatcher(configPath string, callback func(*Config)) *Watcher {
	return &Watcher{
		configPath: configPath,
		callback:   callback,
		stopCh:     make(chan struct{}),
	}
}

// Start begins watching for config file changes
func (w *Watcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := watcher.Add(w.configPath); err != nil {
		watcher.Close()
		return err
	}

	w.watcher = watcher
	w.running = true

	go w.watchLoop()

	return nil
}

// watchLoop handles file system events
func (w *Watcher) watchLoop() {
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				w.handleConfigChange()
			}
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// handleConfigChange loads and validates the new config, then calls callback
func (w *Watcher) handleConfigChange() {
	cfg, err := Load(w.configPath)
	if err != nil {
		// Log error but don't stop watching
		return
	}

	// Validate the config
	if err := cfg.ValidateProviderReferences(); err != nil {
		return
	}
	if err := cfg.ValidateDefaultModels(); err != nil {
		return
	}
	if err := cfg.ValidateApiModes(); err != nil {
		return
	}

	// Call the callback with the valid config
	w.mu.RLock()
	callback := w.callback
	w.mu.RUnlock()

	if callback != nil {
		callback(cfg)
	}
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	close(w.stopCh)
	if w.watcher != nil {
		w.watcher.Close()
	}
	w.running = false
}
