package mongo

import (
	"context"
	"dbx-ray/pkg/models"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

type MongoCollector struct {
	cfg    models.Instance
	client *mongo.Client
	dbName string
}

func NewMongoCollector(cfg models.Instance) (*MongoCollector, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.DSN)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}

	cs, err := connstring.ParseAndValidate(cfg.DSN)
	dbName := "admin"
	if err == nil && cs.Database != "" {
		dbName = cs.Database
	}

	return &MongoCollector{
		cfg:    cfg,
		client: client,
		dbName: dbName,
	}, nil
}

func (c *MongoCollector) Collect(ctx context.Context, ch chan<- models.Metric) error {
	now := time.Now()

	// 1. serverStatus (Requires admin db)
	var serverStatus bson.M
	err := c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&serverStatus)
	if err != nil {
		log.Printf("Error running serverStatus on mongodb %s: %v", c.cfg.ID, err)
		return err
	}

	// Active connections
	if connections, ok := serverStatus["connections"].(bson.M); ok {
		if current, ok := connections["current"].(int32); ok {
			ch <- models.Metric{
				Timestamp:  now,
				InstanceID: c.cfg.ID,
				DBType:     c.cfg.Type,
				Name:       "active_connections",
				Value:      float64(current),
			}
		}
	}

	// Opcounters (Total Transactions for TPS)
	if opcounters, ok := serverStatus["opcounters"].(bson.M); ok {
		var totalTransactions float64 = 0
		for _, v := range opcounters {
			switch val := v.(type) {
			case int32:
				totalTransactions += float64(val)
			case int64:
				totalTransactions += float64(val)
			case float64:
				totalTransactions += val
			}
		}
		ch <- models.Metric{
			Timestamp:  now,
			InstanceID: c.cfg.ID,
			DBType:     c.cfg.Type,
			Name:       "total_transactions",
			Value:      totalTransactions,
		}
	}

	// Disk Usage (dbStats)
	var dbStats bson.M
	err = c.client.Database(c.dbName).RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&dbStats)
	if err == nil {
		if fsUsedSize, ok := dbStats["fsUsedSize"].(float64); ok {
			if fsTotalSize, ok := dbStats["fsTotalSize"].(float64); ok && fsTotalSize > 0 {
				pct := (fsUsedSize / fsTotalSize) * 100
				ch <- models.Metric{
					Timestamp:  now,
					InstanceID: c.cfg.ID,
					DBType:     c.cfg.Type,
					Name:       "disk_usage_pct",
					Value:      pct,
				}
			}
		} else if dataSize, ok := dbStats["dataSize"].(float64); ok {
			// Fallback: compare dataSize against an arbitrary 500MB free tier if fsSizes are missing
			pct := (dataSize / (500 * 1024 * 1024)) * 100
			if pct > 100 {
				pct = 100
			}
			ch <- models.Metric{
				Timestamp:  now,
				InstanceID: c.cfg.ID,
				DBType:     c.cfg.Type,
				Name:       "disk_usage_pct",
				Value:      pct,
			}
		}
	}

	// Top Queries (system.profile)
	profileColl := c.client.Database(c.dbName).Collection("system.profile")
	opts := options.Find().SetSort(bson.D{{Key: "millis", Value: -1}}).SetLimit(5)
	cursor, err := profileColl.Find(ctx, bson.D{}, opts)
	if err == nil {
		var results []bson.M
		cursor.All(ctx, &results)
		for _, res := range results {
			if millis, ok := res["millis"].(int32); ok {
				query := "Unknown"
				if command, ok := res["command"].(bson.M); ok {
					queryBytes, _ := bson.MarshalExtJSON(command, false, false)
					query = string(queryBytes)
					if len(query) > 100 {
						query = query[:97] + "..."
					}
				}
				user := "-"
				if userStr, ok := res["user"].(string); ok {
					user = userStr
				}
				ch <- models.Metric{
					Timestamp:  now,
					InstanceID: c.cfg.ID,
					DBType:     c.cfg.Type,
					Name:       "top_query_time",
					Value:      float64(millis),
					Labels: map[string]string{
						"query":    query,
						"username": user,
						"is_admin": "false",
					},
				}
			}
		}
	}

	// Latest Active Queries (currentOp)
	var currentOp bson.M
	err = c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "currentOp", Value: 1}, {Key: "active", Value: true}}).Decode(&currentOp)
	if err == nil {
		if inprog, ok := currentOp["inprog"].(bson.A); ok {
			for _, opRaw := range inprog {
				if op, ok := opRaw.(bson.M); ok {
					ns, _ := op["ns"].(string)
					// Ignore internal commands and queries on the admin/local dbs that aren't user driven
					if ns == "" || ns == "admin.$cmd" || ns == "local.oplog.rs" {
						continue
					}
					
					opType, _ := op["op"].(string)
					secsRunning, _ := op["secs_running"].(int32)
					
					query := "Unknown"
					if command, ok := op["command"].(bson.M); ok {
						queryBytes, _ := bson.MarshalExtJSON(command, false, false)
						query = string(queryBytes)
						if len(query) > 100 {
							query = query[:97] + "..."
						}
					}
					
					ch <- models.Metric{
						Timestamp:  now,
						InstanceID: c.cfg.ID,
						DBType:     c.cfg.Type,
						Name:       "active_query",
						Value:      float64(secsRunning),
						Labels: map[string]string{
							"query":    query,
							"username": "-",
							"state":    opType,
						},
					}
				}
			}
		}
	} else {
		log.Printf("Failed to collect active queries for mongodb %s: %v", c.cfg.ID, err)
	}

	return nil
}

func (c *MongoCollector) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.client.Disconnect(ctx)
}
