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
	// TrendMicro DS Agent log patterns
	// Pattern: "Malware detected: /path/to/file, Malware: EICAR_TEST_FILE"
	tmMalwareRegex = regexp.MustCompile(`Malware detected:\s+(.+),\s+Malware:\s+(.+)`)
	// Pattern: "Quarantined: /path/to/file"
	tmQuarantineRegex = regexp.MustCompile(`Quarantined:\s+(.+)`)
	// Pattern for clean scan (if logged)
	tmCleanRegex = regexp.MustCompile(`Scan completed:\s+(.+),\s+Result:\s+Clean`)
)

type TrendMicroDriver struct {
	config config.DriverConfig
	logger *slog.Logger
}

func NewTrendMicroDriver(cfg config.DriverConfig, logger *slog.Logger) *TrendMicroDriver {
	return &TrendMicroDriver{
		config: cfg,
		logger: logger.With("driver", "trendmicro"),
	}
}

func (d *TrendMicroDriver) Engine() config.EngineType {
	return config.EngineTrendMicro
}

func (d *TrendMicroDriver) RTSWatch(filePath string, opts WatchOptions) (*ScanResult, error) {
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

func (d *TrendMicroDriver) parseLogEntry(line string) *LogEntry {
	// Check for malware detection
	if matches := tmMalwareRegex.FindStringSubmatch(line); matches != nil {
		return &LogEntry{
			Timestamp: time.Now(),
			FilePath:  matches[1],
			Status:    StatusInfected,
			Signature: matches[2],
			Raw:       line,
		}
	}

	// Check for quarantine (also indicates infection)
	if matches := tmQuarantineRegex.FindStringSubmatch(line); matches != nil {
		return &LogEntry{
			Timestamp: time.Now(),
			FilePath:  matches[1],
			Status:    StatusInfected,
			Raw:       line,
		}
	}

	// Check for clean result
	if matches := tmCleanRegex.FindStringSubmatch(line); matches != nil {
		return &LogEntry{
			Timestamp: time.Now(),
			FilePath:  matches[1],
			Status:    StatusClean,
			Raw:       line,
		}
	}

	return nil
}

func (d *TrendMicroDriver) pathMatches(logPath, targetPath string) bool {
	absLog, err1 := filepath.Abs(logPath)
	absTarget, err2 := filepath.Abs(targetPath)
	if err1 != nil || err2 != nil {
		return logPath == targetPath
	}
	return absLog == absTarget
}

func (d *TrendMicroDriver) CheckHealth() (*EngineHealth, error) {
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

func (d *TrendMicroDriver) GetInfo() EngineInfo {
	return EngineInfo{
		Engine:              d.Engine(),
		Available:           true,
		RTSEnabled:          true,
		ManualScanAvailable: false,
	}
}
