package storage

import (
	"context"
	"dbx-ray/pkg/models"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Storage interface {
	SaveMetric(ctx context.Context, m models.Metric) error
	GetMetrics(ctx context.Context, instanceID, name string, limit int) ([]models.Metric, error)
	Close()
}

type TimescaleStorage struct {
	pool *pgxpool.Pool
}

func NewTimescaleStorage(dsn string) (*TimescaleStorage, error) {
	ctx := context.Background()
	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 10; i++ {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			err = pool.Ping(ctx)
			if err == nil {
				break
			}
		}
		log.Printf("Failed to connect to database, retrying in 2s... (%v)", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database after retries: %w", err)
	}

	// Initialize the schema
	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		time TIMESTAMPTZ NOT NULL,
		instance_id TEXT NOT NULL,
		db_type TEXT NOT NULL,
		name TEXT NOT NULL,
		value DOUBLE PRECISION NOT NULL,
		labels JSONB
	);
	
	-- Create hypertable if it doesn't exist
	DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT * FROM timescaledb_information.hypertables 
			WHERE hypertable_name = 'metrics'
		) THEN
			PERFORM create_hypertable('metrics', 'time', if_not_exists => TRUE);
		END IF;
	END $$;
	
	CREATE INDEX IF NOT EXISTS idx_metrics_instance_name ON metrics (instance_id, name, time DESC);
	`
	_, err = pool.Exec(ctx, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("TimescaleDB storage initialized")
	return &TimescaleStorage{pool: pool}, nil
}

func (s *TimescaleStorage) SaveMetric(ctx context.Context, m models.Metric) error {
	query := `
		INSERT INTO metrics (time, instance_id, db_type, name, value, labels)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.pool.Exec(ctx, query, m.Timestamp, m.InstanceID, m.DBType, m.Name, m.Value, m.Labels)
	return err
}

func (s *TimescaleStorage) GetMetrics(ctx context.Context, instanceID, name string, limit int) ([]models.Metric, error) {
	query := `
		SELECT time, instance_id, db_type, name, value, labels
		FROM metrics
		WHERE instance_id = $1 AND name = $2
		ORDER BY time DESC
		LIMIT $3
	`
	rows, err := s.pool.Query(ctx, query, instanceID, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.Metric
	for rows.Next() {
		var m models.Metric
		err := rows.Scan(&m.Timestamp, &m.InstanceID, &m.DBType, &m.Name, &m.Value, &m.Labels)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

func (s *TimescaleStorage) Close() {
	s.pool.Close()
}
