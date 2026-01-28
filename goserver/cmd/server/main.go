package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/LevanPro/server/internal/config"
	"github.com/LevanPro/server/internal/services"
)

type application struct {
	cfg              *config.Config
	fileService      *services.FileService
	userService      *services.UserService
	bandwidthService *services.BandwidthService
	pingService      *services.PingService
	logger           *slog.Logger
}

func main() {
	cfg := config.MustLoad()

	logger := slog.New(getLogHandler())
	slog.SetDefault(logger)

	// Parse collection interval
	collectionInterval, err := time.ParseDuration(cfg.BandwidthTracking.CollectionInterval)
	if err != nil {
		logger.Error("Invalid collection interval, using default 60s", "error", err.Error())
		collectionInterval = 60 * time.Second
	}

	// Create bandwidth storage directory
	bandwidthStoragePath := filepath.Join(cfg.StoragePath, cfg.BandwidthTracking.StoragePath)
	if err := os.MkdirAll(bandwidthStoragePath, 0755); err != nil {
		logger.Error("Failed to create bandwidth storage directory", "error", err.Error())
		os.Exit(1)
	}

	bandwidthService, err := services.NewBandwidthService(
		bandwidthStoragePath,
		collectionInterval,
		logger,
	)
	if err != nil {
		logger.Error("Failed to initialize bandwidth service", "error", err.Error())
		os.Exit(1)
	}
	defer bandwidthService.Close()

	// Start background tracking
	if err := bandwidthService.Start(); err != nil {
		logger.Error("Failed to start bandwidth tracking", "error", err.Error())
		os.Exit(1)
	}

	pingService, err := services.NewPingService(cfg.UDPServer.Address, logger)
	if err != nil {
		logger.Error("Failed to initialize ping service", "error", err.Error())
		os.Exit(1)
	}
	defer pingService.Close()

	err = pingService.Start()
	if err != nil {
		logger.Error("Failed to start ping service", "error", err.Error())
		os.Exit(1)
	}

	app := &application{
		cfg:              cfg,
		fileService:      services.NewFileService(cfg.StoragePath),
		userService:      services.NewUserService(),
		bandwidthService: bandwidthService,
		pingService:      pingService,
		logger:           logger,
	}

	err = http.ListenAndServe(app.cfg.HTTPServer.Address, app.routes())
	if err != nil {
		app.logger.Error(err.Error())
		os.Exit(1)
	}
}

func getLogHandler() *slog.JSONHandler {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   slog.TimeKey,
					Value: slog.StringValue(time.Now().UTC().Format(time.RFC3339)),
				}
			}
			return a
		},
	})

	return handler
}
