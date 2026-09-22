package models

import "time"

// Metric represents a single collected data point from a database instance.
type Metric struct {
	InstanceID string            `json:"instance_id"`
	DBType     string            `json:"db_type"` // "postgres", "mysql", "mongodb"
	Timestamp  time.Time         `json:"timestamp"`
	Name       string            `json:"name"`
	Value      float64           `json:"value"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// Instance represents a database connection configuration
type Instance struct {
	ID       string `yaml:"id"`
	Type     string `yaml:"type"` // "postgres", "mysql", "mongodb"
	DSN      string `yaml:"dsn"`
	Interval string `yaml:"interval"` // e.g., "15s"
}

// Config represents the application configuration
type Config struct {
	Instances []Instance `yaml:"instances"`
}
