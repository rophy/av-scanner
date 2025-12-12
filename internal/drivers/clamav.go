package drivers

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/nxadm/tail"
	"github.com/rophy/av-scanner/internal/config"
)

var (
	// Regex patterns support optional timestamp prefix: [YYYY-MM-DD HH:MM:SS]
	clamavFoundRegex = regexp.MustCompile(`(?:\[[\d-]+\s[\d:]+\]\s)?(.+):\s+(.+)\s+FOUND$`)
	clamavOKRegex    = regexp.MustCompile(`(?:\[[\d-]+\s[\d:]+\]\s)?(.+):\s+OK$`)
	clamavMovedRegex = regexp.MustCompile(`(?:\[[\d-]+\s[\d:]+\]\s)?(.+):\s+moved to '(.+)'$`)
)

type ClamAVDriver struct {
	config config.DriverConfig
	logger *slog.Logger
}

func NewClamAVDriver(cfg config.DriverConfig, logger *slog.Logger) *ClamAVDriver {
	return &ClamAVDriver{
		config: cfg,
		logger: logger.With("driver", "clamav"),
	}
}

func (d *ClamAVDriver) Engine() config.EngineType {
	return config.EngineClamAV
}

func (d *ClamAVDriver) RTSWatch(filePath string, opts WatchOptions) (*ScanResult, error) {
	startTime := time.Now()
	fileID := filepath.Base(filePath)

	t, err := tail.TailFile(d.config.RTSLogPath, tail.Config{
		Follow:    true,
		ReOpen:    true,
		Poll:      true, // Use polling instead of inotify for shell redirects
		MustExist: true,
		Location:  &tail.SeekInfo{Offset: 0, Whence: os.SEEK_END},
	})
	if err != nil {
		return nil, err
	}
	defer t.Cleanup()

	timeout := time.After(opts.Timeout)

	for {
		select {
		case <-timeout:
			t.Stop()
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

		case line := <-t.Lines:
			if line == nil || line.Err != nil {
				continue
			}

			entry := d.parseLogEntry(line.Text)
			if entry == nil {
				continue
			}

			// Check if this log entry matches our file
			if d.pathMatches(entry.FilePath, filePath) {
				t.Stop()
				return &ScanResult{
					Status:    entry.Status,
					Engine:    d.Engine(),
					Signature: entry.Signature,
					Phase:     PhaseRTS,
					FilePath:  filePath,
					FileID:    fileID,
					Timestamp: time.Now(),
					Duration:  time.Since(startTime).Milliseconds(),
					Raw:       map[string]string{"logEntry": line.Text},
				}, nil
			}
		}
	}
}

func (d *ClamAVDriver) parseLogEntry(line string) *LogEntry {
	// Check for FOUND (infected)
	if matches := clamavFoundRegex.FindStringSubmatch(line); matches != nil {
		return &LogEntry{
			Timestamp: time.Now(),
			FilePath:  matches[1],
			Status:    StatusInfected,
			Signature: matches[2],
			Raw:       line,
		}
	}

	// Check for OK (clean)
	if matches := clamavOKRegex.FindStringSubmatch(line); matches != nil {
		return &LogEntry{
			Timestamp: time.Now(),
			FilePath:  matches[1],
			Status:    StatusClean,
			Raw:       line,
		}
	}

	// Check for moved (infected, after quarantine)
	if matches := clamavMovedRegex.FindStringSubmatch(line); matches != nil {
		return &LogEntry{
			Timestamp: time.Now(),
			FilePath:  matches[1],
			Status:    StatusInfected,
			Raw:       line,
		}
	}

	return nil
}

func (d *ClamAVDriver) pathMatches(logPath, targetPath string) bool {
	absLog, err1 := filepath.Abs(logPath)
	absTarget, err2 := filepath.Abs(targetPath)
	if err1 != nil || err2 != nil {
		return logPath == targetPath
	}
	return absLog == absTarget
}

func (d *ClamAVDriver) CheckHealth() (*EngineHealth, error) {
	health := &EngineHealth{
		Engine:    d.Engine(),
		LastCheck: time.Now(),
	}

	// Check if RTS log file is readable
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
