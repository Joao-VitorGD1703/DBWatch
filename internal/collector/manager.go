package collector

import (
	"context"
	"dbx-ray/internal/collector/mongo"
	"dbx-ray/internal/collector/mysql"
	"dbx-ray/internal/collector/postgres"
	"dbx-ray/pkg/models"
	"log"
	"sync"
	"time"
)

type Collector interface {
	Collect(ctx context.Context, ch chan<- models.Metric) error
	Close()
}

type Manager struct {
	mu      sync.RWMutex
	configs []models.Instance
	metrics chan models.Metric
	wg      sync.WaitGroup
	ctx     context.Context
	cancels map[string]context.CancelFunc
}

func NewManager(configs []models.Instance, metrics chan models.Metric) *Manager {
	return &Manager{
		configs: configs,
		metrics: metrics,
		cancels: make(map[string]context.CancelFunc),
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()

	m.mu.RLock()
	configs := m.configs
	m.mu.RUnlock()

	for _, cfg := range configs {
		m.wg.Add(1)
		childCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancels[cfg.ID] = cancel
		m.mu.Unlock()
		go m.runCollector(childCtx, cfg)
	}
}

func (m *Manager) AddInstance(cfg models.Instance) error {
	m.mu.Lock()
	m.configs = append(m.configs, cfg)
	ctx := m.ctx
	m.mu.Unlock()

	if ctx != nil {
		m.wg.Add(1)
		childCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancels[cfg.ID] = cancel
		m.mu.Unlock()
		go m.runCollector(childCtx, cfg)
	}
	return nil
}

func (m *Manager) RemoveInstance(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find and remove config
	for i, cfg := range m.configs {
		if cfg.ID == id {
			m.configs = append(m.configs[:i], m.configs[i+1:]...)
			break
		}
	}

	// Cancel running collector
	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
	}

	return nil
}

func (m *Manager) GetInstances() []models.Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configs
}

func (m *Manager) Stop() {
	m.wg.Wait()
	close(m.metrics)
}

func (m *Manager) runCollector(ctx context.Context, cfg models.Instance) {
	defer m.wg.Done()

	var c Collector
	var err error

	switch cfg.Type {
	case "postgres":
		c, err = postgres.NewPostgresCollector(cfg)
	case "mysql":
		c, err = mysql.NewMySQLCollector(cfg)
	case "mongodb":
		c, err = mongo.NewMongoCollector(cfg)
	default:
		log.Printf("Unsupported DB type %s for instance %s", cfg.Type, cfg.ID)
		return
	}

	if err != nil {
		log.Printf("Failed to initialize collector for instance %s: %v", cfg.ID, err)
		return
	}
	defer c.Close()

	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		log.Printf("Invalid interval %s for instance %s, defaulting to 15s", cfg.Interval, cfg.ID)
		interval = 15 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial collection
	if err := c.Collect(ctx, m.metrics); err != nil {
		log.Printf("Error collecting metrics for %s: %v", cfg.ID, err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping collector for instance %s", cfg.ID)
			return
		case <-ticker.C:
			if err := c.Collect(ctx, m.metrics); err != nil {
				log.Printf("Error collecting metrics for %s: %v", cfg.ID, err)
			}
		}
	}
}
