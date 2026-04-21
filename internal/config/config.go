package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddr     = ":8080"
	defaultSyncInterval = 5 * time.Second
)

type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	SyncInterval time.Duration
}

func Load() (Config, error) {
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = addrFromPort(os.Getenv("PORT"))
	}
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}

	syncInterval := defaultSyncInterval
	if raw := os.Getenv("SYNC_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse SYNC_INTERVAL: %w", err)
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("SYNC_INTERVAL must be positive")
		}
		syncInterval = parsed
	}

	return Config{
		HTTPAddr:     httpAddr,
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		SyncInterval: syncInterval,
	}, nil
}

func addrFromPort(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return ""
	}
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}
