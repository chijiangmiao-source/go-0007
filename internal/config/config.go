package config

import (
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Addr            string
	StorePath       string
	EngineMode      string
	ShutdownTimeout time.Duration
}

func Load() Config {
	addr := getenv("LEO_LOOP_ADDR", "127.0.0.1:8080")
	store := getenv("LEO_LOOP_STORE", filepath.Join(".", "data", "leo-loop-state.json"))
	return Config{Addr: addr, StorePath: store, EngineMode: getenv("LEO_LOOP_ENGINE_MODE", "normal"), ShutdownTimeout: 5 * time.Second}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
