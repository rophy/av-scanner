package drivers

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rophy/av-scanner/internal/config"
)

const (
	EngineMock     config.EngineType = "mock"
	EICARSignature                   = "EICAR-Test-File"
)

// EICAR test pattern as character codes to avoid triggering AV software
// https://en.wikipedia.org/wiki/EICAR_test_file
var eicarCodes = []byte{
	88, 53, 79, 33, 80, 37, 64, 65, 80, 91, 52, 92, 80, 90, 88, 53,
	52, 40, 80, 94, 41, 55, 67, 67, 41, 55, 125, 36, 69, 73, 67, 65,
	82, 45, 83, 84, 65, 78, 68, 65, 82, 68, 45, 65, 78, 84, 73, 86,
	73, 82, 85, 83, 45, 84, 69, 83, 84, 45, 70, 73, 76, 69, 33, 36,
	72, 43, 72, 42,
}

// EICARPattern returns the EICAR test string constructed from character codes
func EICARPattern() string {
	return string(eicarCodes)
}

// MockDriver implements Driver interface for testing purposes.
// It detects files containing the EICAR test pattern.
type MockDriver struct {
	cfg config.DriverConfig
}

func NewMockDriver(cfg config.DriverConfig) *MockDriver {
	return &MockDriver{
		cfg: cfg,
	}
}

func (d *MockDriver) Engine() config.EngineType {
	return EngineMock
}

func (d *MockDriver) Config() config.DriverConfig {
	return d.cfg
}

func (d *MockDriver) Start() error {
	return nil
}

func (d *MockDriver) Stop() {
}

func (d *MockDriver) RTSWatch(filePath string, opts WatchOptions) (*ScanResult, error) {
	return d.scan(filePath, PhaseRTS)
}

func (d *MockDriver) ManualScan(filePath string) (*ScanResult, error) {
	return d.scan(filePath, PhaseManual)
}

func (d *MockDriver) scan(filePath string, phase ScanPhase) (*ScanResult, error) {
	startTime := time.Now()
	fileID := filepath.Base(filePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		Status:    StatusClean,
		Engine:    EngineMock,
		Phase:     phase,
		FilePath:  filePath,
		FileID:    fileID,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime).Milliseconds(),
	}

	if strings.Contains(string(content), EICARPattern()) {
		result.Status = StatusInfected
		result.Signature = EICARSignature
	}

	return result, nil
}

func (d *MockDriver) CheckHealth() (*EngineHealth, error) {
	return &EngineHealth{
		Engine:    EngineMock,
		Healthy:   true,
		Version:   "1.0.0-mock",
		LastCheck: time.Now(),
	}, nil
}

func (d *MockDriver) GetInfo() EngineInfo {
	return EngineInfo{
		Engine:              EngineMock,
		Available:           true,
		RTSEnabled:          false,
		ManualScanAvailable: true,
	}
}
