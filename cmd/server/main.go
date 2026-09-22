package main

import (
	"context"
	"dbx-ray/internal/api"
	"dbx-ray/internal/collector"
	"dbx-ray/internal/config"
	"dbx-ray/internal/storage"
	"dbx-ray/pkg/models"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	dsn := os.Getenv("TIMESCALE_DSN")
	if dsn == "" {
		log.Fatal("TIMESCALE_DSN environment variable is required")
	}

	store, err := storage.NewTimescaleStorage(dsn)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	metricsChan := make(chan models.Metric, 1000)

	manager := collector.NewManager(cfg.Instances, metricsChan)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Start(ctx)

	app := fiber.New()
	app.Use(logger.New())
	app.Use(cors.New())

	apiHandler := api.NewAPI(store, manager, metricsChan)
	apiHandler.SetupRoutes(app)

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Shutting down server...")
		cancel()
		manager.Stop()
		app.Shutdown()
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server listening on port %s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
