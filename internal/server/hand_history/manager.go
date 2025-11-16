package handhistory

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Manager coordinates flushing for multiple monitors.
type Manager struct {
	cfg    ManagerConfig
	logger zerolog.Logger

	mu       sync.RWMutex
	monitors map[string]*Monitor
	flushReq chan struct{}
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewManager creates and starts a hand-history manager.
func NewManager(logger zerolog.Logger, cfg ManagerConfig) *Manager {
	if cfg.BaseDir == "" {
		cfg.BaseDir = "hands"
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 10 * time.Second
	}
	if cfg.FlushHands <= 0 {
		cfg.FlushHands = 100
	}
	if cfg.Variant == "" {
		cfg.Variant = defaultVariant
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}

	m := &Manager{
		cfg:      cfg,
		logger:   logger,
		monitors: make(map[string]*Monitor),
		flushReq: make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}
	m.wg.Add(1)
	go m.run()
	return m
}

// Shutdown stops the ticker and flushes all monitors.
func (m *Manager) Shutdown() {
	close(m.stop)
	m.wg.Wait()
	m.flushAll()
	m.mu.Lock()
	monitors := m.monitors
	m.monitors = make(map[string]*Monitor)
	m.mu.Unlock()
	for _, monitor := range monitors {
		if err := monitor.Close(); err != nil {
			m.logger.Error().Err(err).Msg("hand history flush on shutdown failed")
		}
	}
}

// CreateMonitor instantiates and registers a monitor for the given game.
func (m *Manager) CreateMonitor(gameID string) (*Monitor, error) {
	m.mu.Lock()
	if _, exists := m.monitors[gameID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("handhistory: monitor for %s already exists", gameID)
	}
	m.mu.Unlock()

	monitorCfg := MonitorConfig{
		GameID:           gameID,
		OutputDir:        filepath.Join(m.cfg.BaseDir, fmt.Sprintf("game-%s", gameID)),
		Filename:         defaultFilename,
		FlushHands:       m.cfg.FlushHands,
		IncludeHoleCards: m.cfg.IncludeHoleCards,
		Variant:          m.cfg.Variant,
		Clock:            m.cfg.Clock,
	}

	monitor, err := NewMonitor(monitorCfg, m.logger.With().Str("game_id", gameID).Logger())
	if err != nil {
		return nil, err
	}
	monitor.SetFlushNotifier(func() { m.requestFlush() })

	m.mu.Lock()
	m.monitors[gameID] = monitor
	m.mu.Unlock()

	return monitor, nil
}

// RemoveMonitor flushes and unregisters the monitor for the given game ID.
func (m *Manager) RemoveMonitor(gameID string) {
	m.mu.Lock()
	monitor, ok := m.monitors[gameID]
	if ok {
		delete(m.monitors, gameID)
	}
	m.mu.Unlock()

	if ok {
		if err := monitor.Close(); err != nil {
			m.logger.Error().Err(err).Str("game_id", gameID).Msg("hand history flush on remove failed")
		}
	}
}

func (m *Manager) run() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.flushAll()
		case <-m.flushReq:
			m.flushAll()
		case <-m.stop:
			return
		}
	}
}

func (m *Manager) requestFlush() {
	select {
	case m.flushReq <- struct{}{}:
	default:
	}
}

func (m *Manager) flushAll() {
	m.mu.RLock()
	snapshot := make(map[string]*Monitor, len(m.monitors))
	for k, v := range m.monitors {
		snapshot[k] = v
	}
	m.mu.RUnlock()

	for gameID, monitor := range snapshot {
		err := monitor.Flush()
		if err != nil {
			m.logger.Error().Err(err).Str("game_id", gameID).Msg("hand history flush failed")
		}
		disabled, dropped := monitor.HandleFlushResult(err)
		if disabled {
			m.logger.Error().Str("game_id", gameID).Int("dropped_hands", dropped).
				Msg("hand history recording disabled after repeated failures")
			m.RemoveMonitor(gameID)
		}
	}
}
