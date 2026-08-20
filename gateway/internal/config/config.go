// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package config

import (
	"os"
	"strconv"
)

// Config holds gateway runtime configuration.
type Config struct {
	Port     int
	LogLevel string
	Env      string
}

// Load reads config from environment with sensible defaults.
func Load() Config {
	port := 8080
	if v := os.Getenv("GOSO_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	level := os.Getenv("GOSO_LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	env := os.Getenv("GOSO_ENV")
	if env == "" {
		env = "development"
	}
	return Config{Port: port, LogLevel: level, Env: env}
}
