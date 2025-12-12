package drivers

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/nxadm/tail"
	"github.com/rophy/av-scanner/internal/cache"
	"github.com/rophy/av-scanner/internal/config"
)

var (
	// Regex patterns support optional timestamp prefix: [YYYY-MM-DD HH:MM:SS]
	clamavFoundRegex = regexp.MustCompile(`(?:\[[\d-]+\s[\d:]+\]\s)?(.+):\s+(.+)\s+FOUND$`)
	clamavMovedRegex = regexp.MustCompile(`(?:\[[\d-]+\s[\d:]+\]\s)?(.+):\s+moved to '(.+)'$`)
)

type ClamAVDriver struct {
	config config.DriverConfig
	logger *slog.Logger
	cache  *cache.DetectionCache
	ctx    context.Context
	cancel context.CancelFunc
}

func NewClamAVDriver(cfg config.DriverConfig, logger *slog.Logger, detectionCache *cache.DetectionCache) *ClamAVDriver {
	ctx, cancel := context.WithCancel(context.Background())
	return &ClamAVDriver{
		config: cfg,
		logger: logger.With("driver", "clamav"),
		cache:  detectionCache,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the background log watcher
func (d *ClamAVDriver) Start() error {
	if _, err := os.Stat(d.config.RTSLogPath); err != nil {
		d.logger.Warn("RTS log file not accessible, background watcher not started", "path", d.config.RTSLogPath)
		return nil
	}

	go d.watchLog()
	d.logger.Info("Background log watcher started", "path", d.config.RTSLogPath)
	return nil
}

// Stop stops the background log watcher
func (d *ClamAVDriver) Stop() {
	d.cancel()
}

func (d *ClamAVDriver) watchLog() {
	t, err := tail.TailFile(d.config.RTSLogPath, tail.Config{
		Follow:    true,
		ReOpen:    true,
		MustExist: true,
		Location:  &tail.SeekInfo{Offset: 0, Whence: os.SEEK_END},
	})
	if err != nil {
		d.logger.Error("Failed to start log tailer", "error", err)
		return
	}
	defer t.Cleanup()

	for {
		select {
		case <-d.ctx.Done():
			t.Stop()
			return
		case line := <-t.Lines:
			if line == nil || line.Err != nil {
				continue
			}
			d.processLogLine(line.Text)
		}
	}
}

func (d *ClamAVDriver) processLogLine(line string) {
	// Check for FOUND (infected)
	if matches := clamavFoundRegex.FindStringSubmatch(line); matches != nil {
		absPath, err := filepath.Abs(matches[1])
		if err != nil {
			absPath = matches[1]
		}

		d.cache.Add(absPath, &cache.Detection{
			FilePath:  matches[1],
			Status:    "infected",
			Signature: matches[2],
			Raw:       line,
		})
		d.logger.Debug("Cached detection", "path", absPath, "signature", matches[2])
		return
	}

	// Check for moved (infected, after quarantine)
	if matches := clamavMovedRegex.FindStringSubmatch(line); matches != nil {
		absPath, err := filepath.Abs(matches[1])
		if err != nil {
			absPath = matches[1]
		}

		d.cache.Add(absPath, &cache.Detection{
			FilePath:  matches[1],
			Status:    "infected",
			Signature: "",
			Raw:       line,
		})
		d.logger.Debug("Cached quarantine", "path", absPath)
	}
}

func (d *ClamAVDriver) Engine() config.EngineType {
	return config.EngineClamAV
}

func (d *ClamAVDriver) RTSWatch(filePath string, opts WatchOptions) (*ScanResult, error) {
	startTime := time.Now()
	fileID := filepath.Base(filePath)

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	timeout := time.After(opts.Timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			d.logger.Warn("RTS watch timeout", "filePath", filePath)
			return &ScanResult{
				Status:    StatusClean,
				Engine:    d.Engine(),
				Phase:     PhaseRTS,
				FilePath:  filePath,
				FileID:    fileID,
				Timestamp: time.Now(),
				Duration:  time.Since(startTime).Milliseconds(),
				Raw:       map[string]bool{"timeout": true},
			}, nil

		case <-ticker.C:
			if cached, found := d.cache.Get(absPath); found {
				status := StatusClean
				if cached.Status == "infected" {
					status = StatusInfected
				}

				return &ScanResult{
					Status:    status,
					Engine:    d.Engine(),
					Signature: cached.Signature,
					Phase:     PhaseRTS,
					FilePath:  filePath,
					FileID:    fileID,
					Timestamp: time.Now(),
					Duration:  time.Since(startTime).Milliseconds(),
					Raw:       map[string]string{"logEntry": cached.Raw},
				}, nil
			}
		}
	}
}

func (d *ClamAVDriver) CheckHealth() (*EngineHealth, error) {
	health := &EngineHealth{
		Engine:    d.Engine(),
		LastCheck: time.Now(),
	}

	if _, err := os.Stat(d.config.RTSLogPath); err != nil {
		health.Healthy = false
		health.Error = "RTS log not accessible: " + d.config.RTSLogPath
		return health, nil
	}

	health.Healthy = true
	return health, nil
}

func (d *ClamAVDriver) GetInfo() EngineInfo {
	return EngineInfo{
		Engine:              d.Engine(),
		Available:           true,
		RTSEnabled:          true,
		ManualScanAvailable: false,
	}
}
