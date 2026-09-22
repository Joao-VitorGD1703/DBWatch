package postgres

import (
	"context"
	"dbx-ray/pkg/models"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresCollector struct {
	instance models.Instance
	pool     *pgxpool.Pool
}

func NewPostgresCollector(instance models.Instance) (*PostgresCollector, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(instance.DSN)
	if err != nil {
		return nil, fmt.Errorf("unable to parse dsn: %w", err)
	}
	// Force simple protocol to avoid prepared statement issues with connection poolers (pgBouncer/Supavisor)
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to postgres: %w", err)
	}

	return &PostgresCollector{
		instance: instance,
		pool:     pool,
	}, nil
}

func (p *PostgresCollector) Collect(ctx context.Context, ch chan<- models.Metric) error {
	now := time.Now()

	// 1. Collect Active Connections
	var activeConns float64
	err := p.pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE state = 'active'").Scan(&activeConns)
	if err != nil {
		return fmt.Errorf("failed to query active connections: %w", err)
	}
	ch <- models.Metric{
		InstanceID: p.instance.ID,
		DBType:     "postgres",
		Timestamp:  now,
		Name:       "active_connections",
		Value:      activeConns,
	}

	// 2. Collect Cache Hit Ratio
	var cacheHitRatio float64
	err = p.pool.QueryRow(ctx, `
		SELECT 
			sum(blks_hit) / nullif(sum(blks_hit) + sum(blks_read), 0) * 100
		FROM pg_stat_database;
	`).Scan(&cacheHitRatio)
	if err != nil {
		return fmt.Errorf("failed to query cache hit ratio: %w", err)
	}
	ch <- models.Metric{
		InstanceID: p.instance.ID,
		DBType:     "postgres",
		Timestamp:  now,
		Name:       "cache_hit_ratio",
		Value:      cacheHitRatio,
	}

	// 2.1 Connections Pct
	var maxConns float64
	err = p.pool.QueryRow(ctx, "SELECT setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&maxConns)
	if err == nil && maxConns > 0 {
		ch <- models.Metric{
			InstanceID: p.instance.ID,
			DBType:     "postgres",
			Timestamp:  now,
			Name:       "connections_pct",
			Value:      (activeConns / maxConns) * 100,
		}
	}

	// 2.2 Disk Usage Pct
	var dbSizeBytes float64
	err = p.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&dbSizeBytes)
	if err == nil {
		maxSize := float64(500 * 1024 * 1024) // 500 MB default
		if dbSizeBytes > maxSize {
			maxSize = dbSizeBytes // Prevent > 100% if over limit
		}
		ch <- models.Metric{
			InstanceID: p.instance.ID,
			DBType:     "postgres",
			Timestamp:  now,
			Name:       "disk_usage_pct",
			Value:      (dbSizeBytes / maxSize) * 100,
		}
	}

	// 2.3 Replication Lag
	var lagSeconds *float64
	err = p.pool.QueryRow(ctx, "SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))").Scan(&lagSeconds)
	if err == nil && lagSeconds != nil {
		ch <- models.Metric{
			InstanceID: p.instance.ID,
			DBType:     "postgres",
			Timestamp:  now,
			Name:       "replication_lag_seconds",
			Value:      *lagSeconds,
		}
	}

	// 2.4 Locks Waiting & Deadlocks
	var locksWaiting float64
	err = p.pool.QueryRow(ctx, "SELECT count(*) FROM pg_locks WHERE NOT granted").Scan(&locksWaiting)
	if err == nil {
		ch <- models.Metric{
			InstanceID: p.instance.ID,
			DBType:     "postgres",
			Timestamp:  now,
			Name:       "locks_waiting",
			Value:      locksWaiting,
		}
	}

	var deadlocks float64
	err = p.pool.QueryRow(ctx, "SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()").Scan(&deadlocks)
	if err == nil {
		ch <- models.Metric{
			InstanceID: p.instance.ID,
			DBType:     "postgres",
			Timestamp:  now,
			Name:       "deadlocks_count",
			Value:      deadlocks,
		}
	}

	// 2.5 Total Transactions (for TPS)
	var totalXact float64
	err = p.pool.QueryRow(ctx, "SELECT xact_commit + xact_rollback FROM pg_stat_database WHERE datname = current_database()").Scan(&totalXact)
	if err == nil {
		ch <- models.Metric{
			InstanceID: p.instance.ID,
			DBType:     "postgres",
			Timestamp:  now,
			Name:       "total_transactions",
			Value:      totalXact,
		}
	}

	// 3. Top 5 Queries (requires pg_stat_statements)
	rows, err := p.pool.Query(ctx, `
		SELECT 
			r.rolname,
			r.rolsuper,
			s.query, 
			s.total_exec_time 
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		ORDER BY s.total_exec_time DESC 
		LIMIT 5;
	`)
	if err != nil {
		return fmt.Errorf("failed to query top queries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rolname *string
		var rolsuper *bool
		var query string
		var totalTime float64
		if err := rows.Scan(&rolname, &rolsuper, &query, &totalTime); err == nil {
			user := "unknown"
			if rolname != nil {
				user = *rolname
			}
			isAdmin := "false"
			if rolsuper != nil && *rolsuper {
				isAdmin = "true"
			}
			ch <- models.Metric{
				InstanceID: p.instance.ID,
				DBType:     "postgres",
				Timestamp:  now,
				Name:       "top_query_time",
				Value:      totalTime,
				Labels: map[string]string{
					"query":    truncate(query, 100),
					"username": user,
					"is_admin": isAdmin,
				},
			}
		} else {
			log.Printf("Error scanning top queries: %v", err)
		}
	}

	// 4. Latest Active Queries (pg_stat_activity)
	activeRows, err := p.pool.Query(ctx, `
		SELECT 
			usename, 
			state, 
			query, 
			extract(epoch from now() - query_start) as duration
		FROM pg_stat_activity 
		WHERE state = 'active' AND pid != pg_backend_pid()
		ORDER BY duration DESC
		LIMIT 10;
	`)
	if err == nil {
		defer activeRows.Close()
		for activeRows.Next() {
			var usename *string
			var state *string
			var query *string
			var duration *float64
			if err := activeRows.Scan(&usename, &state, &query, &duration); err == nil {
				if query != nil && duration != nil {
					user := "-"
					if usename != nil {
						user = *usename
					}
					st := "active"
					if state != nil {
						st = *state
					}
					ch <- models.Metric{
						InstanceID: p.instance.ID,
						DBType:     "postgres",
						Timestamp:  now,
						Name:       "active_query",
						Value:      *duration,
						Labels: map[string]string{
							"query":    truncate(*query, 100),
							"username": user,
							"state":    st,
						},
					}
				}
			}
		}
	} else {
		log.Printf("Error scanning active queries: %v", err)
	}

	return nil
}

func (p *PostgresCollector) Close() {
	p.pool.Close()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
