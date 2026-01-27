package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/LevanPro/server/internal/config"
	"github.com/LevanPro/server/internal/services"
)

type application struct {
	cfg              *config.Config
	fileService      *services.FileService
	userService      *services.UserService
	bandwidthService *services.BandwidthService
	logger           *slog.Logger
}

func main() {
	cfg := config.MustLoad()

	logger := slog.New(getLogHandler())
	slog.SetDefault(logger)

	bandwidthService, err := services.NewBandwidthService()
	if err != nil {
		logger.Error("Failed to initialize bandwidth service", "error", err.Error())
		os.Exit(1)
	}
	defer bandwidthService.Close()

	app := &application{
		cfg:              cfg,
		fileService:      services.NewFileService(cfg.StoragePath),
		userService:      services.NewUserService(),
		bandwidthService: bandwidthService,
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
