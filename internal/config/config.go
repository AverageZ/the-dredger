package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	DBPath      string
	LogPath     string
	OllamaURL   string
	OllamaModel string
	Workers     int
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
		OllamaModel: "gemma3:4b",
		Workers:     3,
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

	return cfg, nil
}
