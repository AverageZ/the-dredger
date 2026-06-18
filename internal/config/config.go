package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	DBPath          string
	LogPath         string
	OllamaURL       string
	OllamaModel     string
	Workers         int
	DredgeBatchSize int
	HostDelay       time.Duration
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("find home directory: %w", err)
	}

	cfg := Config{
		DBPath:      filepath.Join(home, ".dredger", "dredger.db"),
		LogPath:     filepath.Join(home, ".dredger", "dredger.log"),
		OllamaURL:   "http://localhost:11434",
		OllamaModel: "gemma4:e4b",
		Workers:     3,
		// Keep bulk enrichment intentionally bounded and polite by default.
		DredgeBatchSize: 50,
		HostDelay:       2 * time.Second,
	}

	if v := os.Getenv("DREDGER_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DREDGER_LOG_PATH"); v != "" {
		cfg.LogPath = v
	}
	if v := os.Getenv("DREDGER_OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	}
	if v := os.Getenv("DREDGER_OLLAMA_MODEL"); v != "" {
		cfg.OllamaModel = v
	}
	if v := os.Getenv("DREDGER_WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 1 && n <= 16 {
			cfg.Workers = n
		}
	}
	if v := os.Getenv("DREDGER_DREDGE_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 1 && n <= 1000 {
			cfg.DredgeBatchSize = n
		}
	}
	if v := os.Getenv("DREDGER_HOST_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil && d >= 0 && d <= time.Minute {
			cfg.HostDelay = d
		}
	}

	return cfg, nil
}
