package scanner

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/rophy/av-scanner/internal/cache"
	"github.com/rophy/av-scanner/internal/config"
	"github.com/rophy/av-scanner/internal/drivers"
	"github.com/rophy/av-scanner/internal/metrics"
)

type ScanResponse struct {
	FileID        string              `json:"fileId"`
	Status        drivers.ScanStatus  `json:"status"`
	Engine        config.EngineType   `json:"engine"`
	Signature     string              `json:"signature,omitempty"`
	ScanResult    *drivers.ScanResult `json:"scanResult,omitempty"`
	TotalDuration int64               `json:"totalDuration"`
}

type Scanner struct {
	drivers        map[config.EngineType]drivers.Driver
	activeEngine   config.EngineType
	config         *config.Config
	logger         *slog.Logger
	detectionCache *cache.DetectionCache
}

func New(cfg *config.Config, logger *slog.Logger) *Scanner {
	// Create shared detection cache
	detectionCache := cache.NewDetectionCache(cache.DefaultTTL)

	s := &Scanner{
		drivers:        make(map[config.EngineType]drivers.Driver),
		activeEngine:   cfg.ActiveEngine,
		config:         cfg,
		logger:         logger,
		detectionCache: detectionCache,
	}

	// Initialize drivers with shared cache
	s.drivers[config.EngineClamAV] = drivers.NewClamAVDriver(
		cfg.Drivers[config.EngineClamAV],
		logger,
		detectionCache,
	)
	s.drivers[config.EngineTrendMicro] = drivers.NewTrendMicroDriver(
		cfg.Drivers[config.EngineTrendMicro],
		logger,
		detectionCache,
	)
	s.drivers[config.EngineMock] = drivers.NewMockDriver(
		config.DriverConfig{Engine: config.EngineMock},
	)

	return s
}

// Start starts all driver background watchers
func (s *Scanner) Start() error {
	for engine, driver := range s.drivers {
		if err := driver.Start(); err != nil {
			s.logger.Error("Failed to start driver", "engine", engine, "error", err)
		}
	}
	return nil
}

// Stop stops all driver background watchers
func (s *Scanner) Stop() {
	for _, driver := range s.drivers {
		driver.Stop()
	}
	s.detectionCache.Stop()
}

func (s *Scanner) Scan(filePath, fileID, originalName string, size int64) (*ScanResponse, error) {
	startTime := time.Now()
	driver := s.drivers[s.activeEngine]

	s.logger.Info("Starting scan",
		"fileId", fileID,
		"engine", driver.Engine(),
		"originalName", originalName,
		"size", size,
	)

	absPath, _ := filepath.Abs(filePath)

	// 1. Run manual scan
	result, err := driver.ManualScan(filePath)

	var finalStatus drivers.ScanStatus
	var signature string

	if err == nil && (result.Status == drivers.StatusClean || result.Status == drivers.StatusInfected) {
		// Manual scan completed successfully - use its result
		finalStatus = result.Status
		signature = result.Signature
	} else {
		// 2. Manual scan failed (file missing = RTS quarantined it)
		// Wait for RTS cache with timeout proportional to file size
		driverCfg := driver.Config()
		s.logger.Debug("Manual scan failed, waiting for RTS cache", "error", err, "fileId", fileID)
		retryDelay := 20 * time.Millisecond
		baseDelay := time.Duration(driverCfg.RTSCacheBaseDelay) * time.Millisecond
		delayPerMB := time.Duration(driverCfg.RTSCacheDelayPerMB) * time.Millisecond
		maxWait := baseDelay + time.Duration(size/1024/1024)*delayPerMB
		waited := time.Duration(0)
		for waited < maxWait {
			if cached, found := s.detectionCache.Get(absPath); found && cached.Status == "infected" {
				s.logger.Info("File detected by RTS",
					"fileId", fileID,
					"signature", cached.Signature,
					"waitedMs", waited.Milliseconds(),
				)
				finalStatus = drivers.StatusInfected
				signature = cached.Signature
				break
			}
			time.Sleep(retryDelay)
			waited += retryDelay
		}
		// If still not found in cache, it's an error
		if finalStatus == "" {
			return nil, fmt.Errorf("scan failed: file not accessible and no RTS detection found")
		}
	}

	// 3. Clean up file (may already be removed by RTS)
	s.deleteFile(filePath, fileID)

	response := &ScanResponse{
		FileID:        fileID,
		Status:        finalStatus,
		Engine:        driver.Engine(),
		Signature:     signature,
		ScanResult:    result,
		TotalDuration: time.Since(startTime).Milliseconds(),
	}

	s.logger.Info("Scan completed",
		"fileId", fileID,
		"status", response.Status,
		"duration", response.TotalDuration,
	)

	// Record metrics
	metrics.RecordScan(string(driver.Engine()), string(response.Status))

	return response, nil
}

func (s *Scanner) deleteFile(filePath, fileID string) error {
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			s.logger.Debug("File already removed (likely by RTS quarantine)",
				"fileId", fileID,
				"filePath", filePath,
			)
			return nil
		}
		return err
	}
	s.logger.Debug("Deleted scanned file", "fileId", fileID, "filePath", filePath)
	return nil
}

func (s *Scanner) CheckHealth() []*drivers.EngineHealth {
	var results []*drivers.EngineHealth
	for _, driver := range s.drivers {
		health, _ := driver.CheckHealth()
		results = append(results, health)
	}
	return results
}

func (s *Scanner) GetActiveEngineHealth() (*drivers.EngineHealth, error) {
	return s.drivers[s.activeEngine].CheckHealth()
}

func (s *Scanner) GetEngineInfo() []drivers.EngineInfo {
	var results []drivers.EngineInfo
	for _, driver := range s.drivers {
		results = append(results, driver.GetInfo())
	}
	return results
}

func (s *Scanner) ActiveEngine() config.EngineType {
	return s.activeEngine
}

func (s *Scanner) GenerateFileID() string {
	return uuid.New().String()
}

func (s *Scanner) GetUploadPath(fileID, originalName string) string {
	ext := filepath.Ext(originalName)
	return filepath.Join(s.config.UploadDir, fileID+ext)
}
