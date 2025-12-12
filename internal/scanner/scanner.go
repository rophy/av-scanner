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

	// 1. Brief delay proportional to file size to give RTS time to detect
	// Base 50ms + 10ms per MB
	delay := 50*time.Millisecond + time.Duration(size/1024/1024)*10*time.Millisecond
	time.Sleep(delay)

	// 2. Check RTS cache first (fast path)
	if cached, found := s.detectionCache.Get(absPath); found && cached.Status == "infected" {
		s.logger.Info("File detected by RTS (fast path)",
			"fileId", fileID,
			"signature", cached.Signature,
		)
		s.deleteFile(filePath, fileID)
		return &ScanResponse{
			FileID:        fileID,
			Status:        drivers.StatusInfected,
			Engine:        driver.Engine(),
			Signature:     cached.Signature,
			TotalDuration: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 3. RTS didn't catch it, run manual scan
	result, err := driver.ManualScan(filePath)

	var finalStatus drivers.ScanStatus
	var signature string

	if err != nil {
		s.logger.Warn("Manual scan error", "error", err, "fileId", fileID)
		finalStatus = drivers.StatusError
	} else if result.Status == drivers.StatusInfected {
		finalStatus = drivers.StatusInfected
		signature = result.Signature
	} else if result.Status == drivers.StatusError {
		finalStatus = drivers.StatusError
	} else {
		finalStatus = drivers.StatusClean
	}

	// 4. Check RTS cache again (catches race condition where RTS detected during manual scan)
	if finalStatus == drivers.StatusError || finalStatus == drivers.StatusClean {
		if cached, found := s.detectionCache.Get(absPath); found && cached.Status == "infected" {
			s.logger.Info("File detected by RTS (post-scan check)",
				"fileId", fileID,
				"signature", cached.Signature,
			)
			finalStatus = drivers.StatusInfected
			signature = cached.Signature
		}
	}

	// 5. Clean up file (may already be removed by RTS)
	if err := s.deleteFile(filePath, fileID); err != nil {
		s.logger.Debug("File deletion failed", "error", err, "fileId", fileID)
	}

	// If still error after checking cache, treat as scan failure
	if finalStatus == drivers.StatusError {
		return nil, fmt.Errorf("scan failed: file not accessible and no RTS detection found")
	}

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
