package config

import (
	"fmt"
	"os"
	"strconv"
)

type EngineType string

const (
	EngineClamAV     EngineType = "clamav"
	EngineTrendMicro EngineType = "trendmicro"
)

type DriverConfig struct {
	Engine     EngineType
	RTSLogPath string
	Timeout    int // milliseconds
}

type Config struct {
	Port         int
	UploadDir    string
	MaxFileSize  int64
	ActiveEngine EngineType
	LogLevel     string
	Drivers      map[EngineType]DriverConfig
}

func Load() (*Config, error) {
	activeEngine := EngineType(getEnv("AV_ENGINE", "clamav"))

	cfg := &Config{
		Port:         getEnvInt("PORT", 3000),
		UploadDir:    getEnv("UPLOAD_DIR", "/tmp/av-scanner"),
		MaxFileSize:  getEnvInt64("MAX_FILE_SIZE", 104857600), // 100MB
		ActiveEngine: activeEngine,
		LogLevel:     getEnv("LOG_LEVEL", "info"),
		Drivers: map[EngineType]DriverConfig{
			EngineClamAV: {
				Engine:     EngineClamAV,
				RTSLogPath: getEnv("CLAMAV_RTS_LOG_PATH", "/var/log/clamav/clamonacc.log"),
				Timeout:    getEnvInt("CLAMAV_TIMEOUT", 15000),
			},
			EngineTrendMicro: {
				Engine:     EngineTrendMicro,
				RTSLogPath: getEnv("TM_RTS_LOG_PATH", "/var/log/ds_agent/ds_agent.log"),
				Timeout:    getEnvInt("TM_TIMEOUT", 15000),
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.ActiveEngine != EngineClamAV && c.ActiveEngine != EngineTrendMicro {
		return fmt.Errorf("invalid active engine: %s", c.ActiveEngine)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.MaxFileSize < 1 {
		return fmt.Errorf("invalid max file size: %d", c.MaxFileSize)
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}
