package api

import (
	"context"
	"log"
	"os"
	"strconv"

	"dbx-ray/internal/collector"
	"dbx-ray/internal/config"
	"dbx-ray/internal/storage"
	"dbx-ray/pkg/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/contrib/websocket"
)

type API struct {
	store       *storage.TimescaleStorage
	manager     *collector.Manager
	metricsChan chan models.Metric
	hub         *Hub
}

func NewAPI(store *storage.TimescaleStorage, manager *collector.Manager, metricsChan chan models.Metric) *API {
	api := &API{
		store:       store,
		manager:     manager,
		metricsChan: metricsChan,
		hub:         NewHub(),
	}
	return api
}

func (a *API) SetupRoutes(app *fiber.App) {
	api := app.Group("/api")

	api.Get("/instances", a.getInstances)
	api.Post("/instances", a.addInstance)
	api.Delete("/instances/:id", a.deleteInstance)
	api.Get("/metrics/:instanceId", a.GetMetrics)

	// WebSocket endpoint
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/metrics", websocket.New(a.wsHandler))

	// Background worker to broadcast metrics to all WS clients
	go a.broadcastWorker()
}

func (a *API) getInstances(c *fiber.Ctx) error {
	return c.JSON(a.manager.GetInstances())
}

func (a *API) addInstance(c *fiber.Ctx) error {
	var instance models.Instance
	if err := c.BodyParser(&instance); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	
	if instance.ID == "" || instance.Type == "" || instance.DSN == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id, type and dsn are required"})
	}

	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		cfg = &models.Config{}
	}

	cfg.Instances = append(cfg.Instances, instance)
	if err := config.SaveConfig(configPath, cfg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save config"})
	}

	a.manager.AddInstance(instance)
	return c.JSON(instance)
}

func (a *API) deleteInstance(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id is required"})
	}

	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load config"})
	}

	// Remove from config
	found := false
	for i, instance := range cfg.Instances {
		if instance.ID == id {
			cfg.Instances = append(cfg.Instances[:i], cfg.Instances[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "instance not found"})
	}

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save config"})
	}

	// Remove from manager to stop collector
	a.manager.RemoveInstance(id)

	return c.SendStatus(fiber.StatusNoContent)
}

func (a *API) GetMetrics(c *fiber.Ctx) error {
	instanceID := c.Params("instanceId")
	name := c.Query("name", "active_connections")
	limitStr := c.Query("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	metrics, err := a.store.GetMetrics(context.Background(), instanceID, name, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(metrics)
}

func (a *API) wsHandler(c *websocket.Conn) {
	client := &Client{conn: c, send: make(chan models.Metric, 256)}
	a.hub.register <- client

	defer func() {
		a.hub.unregister <- client
		c.Close()
	}()

	// Write pump
	for {
		metric, ok := <-client.send
		if !ok {
			c.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
		if err := c.WriteJSON(metric); err != nil {
			return
		}
	}
}

func (a *API) broadcastWorker() {
	go a.hub.run()
	for metric := range a.metricsChan {
		// Save to storage
		if err := a.store.SaveMetric(context.Background(), metric); err != nil {
			log.Printf("Error saving metric: %v", err)
		}

		// Broadcast to all WS clients
		a.hub.broadcast <- metric
	}
}

// Simple Hub for managing WebSocket clients
type Client struct {
	conn *websocket.Conn
	send chan models.Metric
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan models.Metric
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan models.Metric),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case metric := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- metric:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}
