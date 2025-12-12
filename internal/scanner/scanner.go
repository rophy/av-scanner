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
)

type ScanOptions struct {
	RTSTimeout time.Duration
}

type ScanResponse struct {
	FileID        string              `json:"fileId"`
	Status        drivers.ScanStatus  `json:"status"`
	Engine        config.EngineType   `json:"engine"`
	Signature     string              `json:"signature,omitempty"`
	RTSResult     *drivers.ScanResult `json:"rtsResult,omitempty"`
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

func (s *Scanner) Scan(filePath, fileID, originalName string, size int64, opts ScanOptions) (*ScanResponse, error) {
	startTime := time.Now()
	driver := s.drivers[s.activeEngine]

	s.logger.Info("Starting scan",
		"fileId", fileID,
		"engine", driver.Engine(),
		"originalName", originalName,
		"size", size,
	)

	timeout := opts.RTSTimeout
	if timeout == 0 {
		timeout = time.Duration(s.config.Drivers[s.activeEngine].Timeout) * time.Millisecond
	}

	// Watch for RTS result
	rtsResult, err := driver.RTSWatch(filePath, drivers.WatchOptions{
		Timeout:      timeout,
		PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		s.logger.Error("RTS watch error", "error", err, "fileId", fileID)
		return nil, fmt.Errorf("RTS watch failed: %w", err)
	}

	// Always delete file after scan
	if err := s.deleteFile(filePath, fileID); err != nil {
		s.logger.Debug("File deletion failed", "error", err, "fileId", fileID)
	}

	response := &ScanResponse{
		FileID:        fileID,
		Status:        rtsResult.Status,
		Engine:        driver.Engine(),
		Signature:     rtsResult.Signature,
		RTSResult:     rtsResult,
		TotalDuration: time.Since(startTime).Milliseconds(),
	}

	s.logger.Info("Scan completed",
		"fileId", fileID,
		"status", response.Status,
		"duration", response.TotalDuration,
	)

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
