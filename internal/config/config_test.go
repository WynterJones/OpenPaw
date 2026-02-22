package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear env vars that could interfere with default values.
	// Setting to empty string is sufficient: the override checks use != ""
	// so empty values are treated the same as unset for Port, Bind, DataDir,
	// LogLevel, and EncryptionKey. DevMode compares against "true" so empty
	// is effectively false. JWTSecret defaults to "" anyway.
	for _, key := range []string{
		"OPENPAW_PORT",
		"OPENPAW_BIND",
		"OPENPAW_DATA_DIR",
		"OPENPAW_LOG_LEVEL",
		"OPENPAW_DEV",
		"OPENPAW_JWT_SECRET",
		"OPENPAW_ENCRYPTION_KEY",
	} {
		t.Setenv(key, "")
	}

	cfg := Load()

	if cfg.Port != 41295 {
		t.Errorf("expected default port 41295, got %d", cfg.Port)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("expected default bind address 127.0.0.1, got %s", cfg.BindAddress)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level info, got %s", cfg.LogLevel)
	}
	if cfg.DevMode != false {
		t.Errorf("expected default dev mode false, got %v", cfg.DevMode)
	}
	if cfg.DataDir == "" {
		t.Error("expected DataDir to be non-empty")
	}
}

func TestLoadPortOverride(t *testing.T) {
	t.Setenv("OPENPAW_PORT", "9090")

	cfg := Load()

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
}

func TestLoadInvalidPortFallsBackToDefault(t *testing.T) {
	t.Setenv("OPENPAW_PORT", "not-a-number")

	cfg := Load()

	if cfg.Port != 41295 {
		t.Errorf("expected default port 41295 for invalid port, got %d", cfg.Port)
	}
}

func TestLoadBindOverride(t *testing.T) {
	t.Setenv("OPENPAW_BIND", "0.0.0.0")

	cfg := Load()

	if cfg.BindAddress != "0.0.0.0" {
		t.Errorf("expected bind address 0.0.0.0, got %s", cfg.BindAddress)
	}
}

func TestLoadLogLevelOverride(t *testing.T) {
	t.Setenv("OPENPAW_LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.LogLevel)
	}
}

func TestLoadDevModeTrue(t *testing.T) {
	t.Setenv("OPENPAW_DEV", "true")

	cfg := Load()

	if cfg.DevMode != true {
		t.Errorf("expected dev mode true, got %v", cfg.DevMode)
	}
}

func TestLoadDevModeFalse(t *testing.T) {
	t.Setenv("OPENPAW_DEV", "false")

	cfg := Load()

	if cfg.DevMode != false {
		t.Errorf("expected dev mode false, got %v", cfg.DevMode)
	}
}

func TestLoadDevModeInvalidIsFalse(t *testing.T) {
	t.Setenv("OPENPAW_DEV", "yes")

	cfg := Load()

	if cfg.DevMode != false {
		t.Errorf("expected dev mode false for non-'true' value, got %v", cfg.DevMode)
	}
}

func TestLoadJWTSecretOverride(t *testing.T) {
	t.Setenv("OPENPAW_JWT_SECRET", "my-secret-key")

	cfg := Load()

	if cfg.JWTSecret != "my-secret-key" {
		t.Errorf("expected JWT secret my-secret-key, got %s", cfg.JWTSecret)
	}
}

func TestLoadEncryptionKeyOverride(t *testing.T) {
	t.Setenv("OPENPAW_ENCRYPTION_KEY", "enc-key-value")

	cfg := Load()

	if cfg.EncryptionKey != "enc-key-value" {
		t.Errorf("expected encryption key enc-key-value, got %s", cfg.EncryptionKey)
	}
}

func TestLoadDataDirOverride(t *testing.T) {
	t.Setenv("OPENPAW_DATA_DIR", "/tmp/openpaw-test-data")

	cfg := Load()

	if cfg.DataDir != "/tmp/openpaw-test-data" {
		t.Errorf("expected data dir /tmp/openpaw-test-data, got %s", cfg.DataDir)
	}
}

func TestLoadAllOverrides(t *testing.T) {
	t.Setenv("OPENPAW_PORT", "8888")
	t.Setenv("OPENPAW_BIND", "0.0.0.0")
	t.Setenv("OPENPAW_DATA_DIR", "/tmp/test")
	t.Setenv("OPENPAW_LOG_LEVEL", "warn")
	t.Setenv("OPENPAW_DEV", "true")
	t.Setenv("OPENPAW_JWT_SECRET", "secret123")
	t.Setenv("OPENPAW_ENCRYPTION_KEY", "enckey456")

	cfg := Load()

	if cfg.Port != 8888 {
		t.Errorf("expected port 8888, got %d", cfg.Port)
	}
	if cfg.BindAddress != "0.0.0.0" {
		t.Errorf("expected bind 0.0.0.0, got %s", cfg.BindAddress)
	}
	if cfg.DataDir != "/tmp/test" {
		t.Errorf("expected data dir /tmp/test, got %s", cfg.DataDir)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("expected log level warn, got %s", cfg.LogLevel)
	}
	if cfg.DevMode != true {
		t.Errorf("expected dev mode true, got %v", cfg.DevMode)
	}
	if cfg.JWTSecret != "secret123" {
		t.Errorf("expected JWT secret secret123, got %s", cfg.JWTSecret)
	}
	if cfg.EncryptionKey != "enckey456" {
		t.Errorf("expected encryption key enckey456, got %s", cfg.EncryptionKey)
	}
}
