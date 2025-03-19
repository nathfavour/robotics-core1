package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nathfavour/robotics-core1/go-layer/internal/api"
	"github.com/nathfavour/robotics-core1/go-layer/internal/cloud"
	"github.com/nathfavour/robotics-core1/go-layer/internal/config"
	"github.com/nathfavour/robotics-core1/go-layer/internal/core"
	"github.com/nathfavour/robotics-core1/go-layer/internal/messaging"
	"github.com/sirupsen/logrus"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	logLevel := flag.String("log-level", "info", "Logging level (debug, info, warn, error)")
	flag.Parse()

	// Set up logging
	setupLogging(*logLevel)
	logrus.Info("Starting Robotics-Core1 Network Backend")

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load configuration")
	}

	// Create context that can be cancelled for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	messageBroker, err := messaging.NewBroker(ctx, cfg.Messaging)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize message broker")
	}

	cloudConnector, err := cloud.NewConnector(ctx, cfg.Cloud, messageBroker)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize cloud connector")
	}

	coreSystem, err := core.NewSystem(ctx, cfg.Core, messageBroker)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize core system")
	}

	apiServer, err := api.NewServer(cfg.API, messageBroker, coreSystem, cloudConnector)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize API server")
	}

	// Start services
	go startServices(ctx, apiServer, messageBroker, cloudConnector, coreSystem)

	// Wait for termination signal
	sig := waitForSignal()
	logrus.WithField("signal", sig).Info("Received termination signal")

	// Perform graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	logrus.Info("Shutting down services")
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logrus.WithError(err).Error("Error shutting down API server")
	}

	// Trigger context cancellation to stop all services
	cancel()

	// Wait a moment for goroutines to clean up
	time.Sleep(250 * time.Millisecond)
	logrus.Info("Shutdown complete")
}

func setupLogging(level string) {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
	})

	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.WithError(err).Warn("Invalid log level, using info")
		logLevel = logrus.InfoLevel
	}
	logrus.SetLevel(logLevel)
}

func startServices(ctx context.Context,
	apiServer *api.Server,
	messageBroker *messaging.Broker,
	cloudConnector *cloud.Connector,
	coreSystem *core.System) {

	// Start API server
	go func() {
		logrus.Info("Starting API server")
		if err := apiServer.Start(ctx); err != nil {
			logrus.WithError(err).Error("API server failed")
		}
	}()

	// Start messaging broker
	go func() {
		logrus.Info("Starting message broker")
		messageBroker.Start(ctx)
	}()

	// Start cloud connector
	go func() {
		logrus.Info("Starting cloud connector")
		if err := cloudConnector.Connect(ctx); err != nil {
			logrus.WithError(err).Error("Cloud connector failed")
		}
	}()

	// Start core system
	go func() {
		logrus.Info("Starting core system")
		if err := coreSystem.Start(ctx); err != nil {
			logrus.WithError(err).Error("Core system failed")
		}
	}()

	logrus.Info("All services started")
}

func waitForSignal() os.Signal {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	return <-signalChan
}
