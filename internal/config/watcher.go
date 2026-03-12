// Package config handles configuration loading and hot reload
package config

import (
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches for config file changes
type Watcher struct {
	configPath string
	watcher    *fsnotify.Watcher
	callback   func(*Config, error)
	mu         sync.RWMutex
	stopCh     chan struct{}
	running    atomic.Bool
}

// NewWatcher creates a new config watcher
// The callback receives the new config and any validation error
func NewWatcher(configPath string, callback func(*Config, error)) *Watcher {
	return &Watcher{
		configPath: configPath,
		callback:   callback,
		stopCh:     make(chan struct{}),
	}
}

// Start begins watching for config file changes
func (w *Watcher) Start() error {
	if w.running.Load() {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running.Load() {
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
	w.running.Store(true)

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
		w.callCallback(nil, err)
		return
	}

	// Validate the config
	if err := cfg.ValidateProviderReferences(); err != nil {
		w.callCallback(nil, err)
		return
	}
	if err := cfg.ValidateDefaultModels(); err != nil {
		w.callCallback(nil, err)
		return
	}
	if err := cfg.ValidateApiModes(); err != nil {
		w.callCallback(nil, err)
		return
	}

	w.callCallback(cfg, nil)
}

// callCallback safely invokes the callback
func (w *Watcher) callCallback(cfg *Config, err error) {
	w.mu.RLock()
	callback := w.callback
	w.mu.RUnlock()

	if callback != nil {
		callback(cfg, err)
	}
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	if !w.running.Load() {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running.Load() {
		return
	}

	close(w.stopCh)
	if w.watcher != nil {
		w.watcher.Close()
	}
	w.running.Store(false)
}

// IsRunning returns whether the watcher is currently running
func (w *Watcher) IsRunning() bool {
	return w.running.Load()
}
