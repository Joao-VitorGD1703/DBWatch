package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"dbx-ray/pkg/models"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLCollector struct {
	db       *sql.DB
	instance models.Instance
}

func NewMySQLCollector(cfg models.Instance) (*MySQLCollector, error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping mysql: %w", err)
	}
	return &MySQLCollector{db: db, instance: cfg}, nil
}

func (c *MySQLCollector) Collect(ctx context.Context, ch chan<- models.Metric) error {
	// 1. Active Connections
	var activeConnsStr string
	err := c.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Threads_connected'").Scan(new(string), &activeConnsStr)
	if err == nil {
		activeConns, _ := strconv.ParseFloat(activeConnsStr, 64)
		ch <- models.Metric{
			InstanceID: c.instance.ID,
			Name:       "active_connections",
			Value:      activeConns,
			Timestamp:  time.Now(),
		}
	} else {
		log.Printf("Failed to collect active connections for %s: %v", c.instance.ID, err)
	}

	// 2. Cache Hit Ratio
	var readsStr, requestsStr string
	err1 := c.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_reads'").Scan(new(string), &readsStr)
	err2 := c.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_read_requests'").Scan(new(string), &requestsStr)
	
	if err1 == nil && err2 == nil {
		reads, _ := strconv.ParseFloat(readsStr, 64)
		requests, _ := strconv.ParseFloat(requestsStr, 64)
		
		var hitRatio float64 = 0
		if requests > 0 {
			hitRatio = 1.0 - (reads / requests)
		}

		ch <- models.Metric{
			InstanceID: c.instance.ID,
			Name:       "cache_hit_ratio",
			Value:      hitRatio * 100, // percentage
			Timestamp:  time.Now(),
		}
	} else {
		log.Printf("Failed to collect cache hit ratio for %s: %v %v", c.instance.ID, err1, err2)
	}

	// 3. Slow Queries
	rows, err := c.db.QueryContext(ctx, `
		SELECT DIGEST_TEXT as query, SUM_TIMER_WAIT / 1000000000000.0 as total_exec_time 
		FROM performance_schema.events_statements_summary_by_digest 
		ORDER BY SUM_TIMER_WAIT DESC 
		LIMIT 5;
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var query *string
			var totalExecTime float64
			if err := rows.Scan(&query, &totalExecTime); err == nil {
				qText := ""
				if query != nil {
					qText = *query
				}
				ch <- models.Metric{
					InstanceID: c.instance.ID,
					Name:       "slow_queries",
					Value:      totalExecTime,
					Labels:     map[string]string{"query": qText},
					Timestamp:  time.Now(),
				}
			}
		}
	} else {
		log.Printf("Failed to collect slow queries for %s: %v", c.instance.ID, err)
	}

	// 4. Latest Active Queries (processlist)
	activeRows, err := c.db.QueryContext(ctx, `
		SELECT 
			USER, 
			COMMAND as state, 
			INFO as query, 
			TIME as duration 
		FROM information_schema.processlist 
		WHERE COMMAND != 'Sleep' AND INFO IS NOT NULL
		ORDER BY TIME DESC
		LIMIT 10;
	`)
	if err == nil {
		defer activeRows.Close()
		now := time.Now()
		for activeRows.Next() {
			var user string
			var state string
			var query string
			var duration float64
			if err := activeRows.Scan(&user, &state, &query, &duration); err == nil {
				if len(query) > 100 {
					query = query[:97] + "..."
				}
				ch <- models.Metric{
					InstanceID: c.instance.ID,
					DBType:     "mysql",
					Timestamp:  now,
					Name:       "active_query",
					Value:      duration,
					Labels: map[string]string{
						"query":    query,
						"username": user,
						"state":    state,
					},
				}
			}
		}
	} else {
		log.Printf("Failed to collect active queries for %s: %v", c.instance.ID, err)
	}

	return nil
}

func (c *MySQLCollector) Close() {
	if c.db != nil {
		c.db.Close()
	}
}
