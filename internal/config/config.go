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
	EngineMock       EngineType = "mock"
)

type DriverConfig struct {
	Engine            EngineType
	RTSLogPath        string
	ScanBinaryPath    string
	Timeout           int // milliseconds
	RTSCacheBaseDelay int // milliseconds - base delay when waiting for RTS cache
	RTSCacheDelayPerMB int // milliseconds - additional delay per MB of file size
}

type AuthConfig struct {
	Enabled       bool
	ServiceURL    string // kube-federated-auth service URL
	ClusterName   string // cluster name for token validation
	Timeout       int    // milliseconds
	AllowlistFile string // path to allowlist YAML file
}

type Config struct {
	Port         int
	UploadDir    string
	MaxFileSize  int64
	ActiveEngine EngineType
	LogLevel     string
	Drivers      map[EngineType]DriverConfig
	Auth         AuthConfig
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
				Engine:             EngineClamAV,
				RTSLogPath:         getEnv("CLAMAV_RTS_LOG_PATH", "/var/log/clamav/clamonacc.log"),
				ScanBinaryPath:     getEnv("CLAMAV_SCAN_BINARY", "/usr/bin/clamdscan"),
				Timeout:            getEnvInt("CLAMAV_TIMEOUT", 15000),
				RTSCacheBaseDelay:  getEnvInt("CLAMAV_RTS_CACHE_BASE_DELAY", 500),
				RTSCacheDelayPerMB: getEnvInt("CLAMAV_RTS_CACHE_DELAY_PER_MB", 10),
			},
			EngineTrendMicro: {
				Engine:             EngineTrendMicro,
				RTSLogPath:         getEnv("TM_RTS_LOG_PATH", "/var/log/ds_agent/ds_agent.log"),
				ScanBinaryPath:     getEnv("TM_SCAN_BINARY", "/opt/ds_agent/dsa_scan"),
				Timeout:            getEnvInt("TM_TIMEOUT", 15000),
				RTSCacheBaseDelay:  getEnvInt("TM_RTS_CACHE_BASE_DELAY", 500),
				RTSCacheDelayPerMB: getEnvInt("TM_RTS_CACHE_DELAY_PER_MB", 10),
			},
		},
		Auth: AuthConfig{
			Enabled:       getEnvBool("AUTH_ENABLED", false),
			ServiceURL:    getEnv("AUTH_SERVICE_URL", ""),
			ClusterName:   getEnv("AUTH_CLUSTER_NAME", "default"),
			Timeout:       getEnvInt("AUTH_TIMEOUT", 5000),
			AllowlistFile: getEnv("AUTH_ALLOWLIST_FILE", "/etc/av-scanner/allowlist.yaml"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.ActiveEngine != EngineClamAV && c.ActiveEngine != EngineTrendMicro && c.ActiveEngine != EngineMock {
		return fmt.Errorf("invalid active engine: %s", c.ActiveEngine)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.MaxFileSize < 1 {
		return fmt.Errorf("invalid max file size: %d", c.MaxFileSize)
	}
	if c.Auth.Enabled {
		if c.Auth.ServiceURL == "" {
			return fmt.Errorf("AUTH_SERVICE_URL is required when AUTH_ENABLED=true")
		}
		if c.Auth.Timeout < 1 {
			return fmt.Errorf("invalid auth timeout: %d", c.Auth.Timeout)
		}
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

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
