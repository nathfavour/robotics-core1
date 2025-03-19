package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nathfavour/robotics-core1/go-layer/internal/cloud"
	"github.com/nathfavour/robotics-core1/go-layer/internal/config"
	"github.com/nathfavour/robotics-core1/go-layer/internal/core"
	"github.com/nathfavour/robotics-core1/go-layer/internal/messaging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Server provides the HTTP and WebSocket interfaces for the robotics system
type Server struct {
	httpServer     *http.Server
	cfg            config.APIConfig
	messageBroker  *messaging.Broker
	coreSystem     *core.System
	cloudConnector *cloud.Connector
	upgrader       websocket.Upgrader
	logger         *logrus.Entry
}

// NewServer creates a new API server
func NewServer(cfg config.APIConfig, messageBroker *messaging.Broker, coreSystem *core.System, cloudConnector *cloud.Connector) (*Server, error) {
	s := &Server{
		cfg:            cfg,
		messageBroker:  messageBroker,
		coreSystem:     coreSystem,
		cloudConnector: cloudConnector,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// In production, implement proper origin checks
				return true
			},
		},
		logger: logrus.WithField("component", "api-server"),
	}

	mux := http.NewServeMux()

	// Register API endpoints
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/command", s.handleCommand)
	mux.HandleFunc("/api/v1/ws", s.handleWebSocket)
	mux.HandleFunc("/api/v1/algorithms", s.handleAlgorithms)
	mux.HandleFunc("/api/v1/sensors", s.handleSensors)

	// Cloud sync endpoints
	mux.HandleFunc("/api/v1/cloud/sync", s.handleCloudSync)
	mux.HandleFunc("/api/v1/cloud/status", s.handleCloudStatus)

	// Metrics endpoint for Prometheus
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	return s, nil
}

// Start the API server
func (s *Server) Start(ctx context.Context) error {
	s.logger.WithField("port", s.cfg.Port).Info("Starting API server")

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.WithError(err).Error("HTTP server failed")
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// Shutdown the API server gracefully
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down API server")
	return s.httpServer.Shutdown(ctx)
}

// API endpoint handlers
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"status":    "operational",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "0.1.0",
		"components": map[string]string{
			"api":     "online",
			"core":    s.coreSystem.Status(),
			"cloud":   s.cloudConnector.Status(),
			"message": s.messageBroker.Status(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cmd struct {
		Action string          `json:"action"`
		Target string          `json:"target"`
		Params json.RawMessage `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Process command through core system
	result, err := s.coreSystem.ExecuteCommand(r.Context(), cmd.Action, cmd.Target, cmd.Params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Command execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.WithError(err).Error("WebSocket upgrade failed")
		return
	}
	defer conn.Close()

	// Create client handler
	client := NewWSClient(conn, s.messageBroker)
	client.Handle()
}

func (s *Server) handleAlgorithms(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List algorithms
		algorithms, err := s.coreSystem.GetAlgorithms(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get algorithms: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(algorithms)

	case http.MethodPost:
		// Register new algorithm
		var algo json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&algo); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		id, err := s.coreSystem.RegisterAlgorithm(r.Context(), algo)
		if err != nil {
			http.Error(w, fmt.Sprintf("Algorithm registration failed: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSensors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get sensor data
		sensors, err := s.coreSystem.GetSensorData(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get sensor data: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sensors)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCloudSync(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// Trigger cloud sync
		var params struct {
			Mode string `json:"mode"` // "full" or "incremental"
		}

		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		syncID, err := s.cloudConnector.TriggerSync(r.Context(), params.Mode)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to start sync: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"sync_id": syncID})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCloudStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status, err := s.cloudConnector.GetSyncStatus(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get cloud status: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
