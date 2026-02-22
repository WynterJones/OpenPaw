package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Port          int
	BindAddress   string
	DataDir       string
	LogLevel      string
	JWTSecret     string
	EncryptionKey string
	DevMode       bool
}

func Load() *Config {
	cfg := &Config{
		Port:        41295,
		BindAddress: "127.0.0.1",
		DataDir:     resolveDataDir(),
		LogLevel:    "info",
		JWTSecret:   getEnv("OPENPAW_JWT_SECRET", ""),
		DevMode:     getEnv("OPENPAW_DEV", "false") == "true",
	}

	if p := getEnv("OPENPAW_PORT", ""); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			cfg.Port = port
		}
	}
	if b := getEnv("OPENPAW_BIND", ""); b != "" {
		cfg.BindAddress = b
	}
	if d := getEnv("OPENPAW_DATA_DIR", ""); d != "" {
		cfg.DataDir = d
	}
	if l := getEnv("OPENPAW_LOG_LEVEL", ""); l != "" {
		cfg.LogLevel = l
	}
	if ek := getEnv("OPENPAW_ENCRYPTION_KEY", ""); ek != "" {
		cfg.EncryptionKey = ek
	}

	return cfg
}

func resolveDataDir() string {
	// Resolve data dir relative to the executable, not the CWD
	exe, err := os.Executable()
	if err != nil {
		return "./data"
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "./data"
	}
	return filepath.Join(filepath.Dir(exe), "data")
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
