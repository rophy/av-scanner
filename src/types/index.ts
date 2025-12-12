export type ScanStatus = 'clean' | 'infected' | 'error';
export type EngineType = 'trendmicro' | 'clamav';
export type ScanPhase = 'rts' | 'manual';

export interface UnifiedResult {
  status: ScanStatus;
  engine: EngineType;
  signature?: string;
  phase: ScanPhase;
  filePath: string;
  fileId: string;
  timestamp: Date;
  duration: number;
  raw?: unknown;
}

export interface RawEngineResult {
  engine: EngineType;
  exitCode: number;
  stdout: string;
  stderr: string;
  logEntry?: string;
}

export interface ScanRequest {
  fileId: string;
  filePath: string;
  originalName: string;
  mimeType?: string;
  size: number;
}

export interface ScanResponse {
  fileId: string;
  status: ScanStatus;
  engine: EngineType;
  signature?: string;
  rtsResult?: UnifiedResult;
  manualResult?: UnifiedResult;
  totalDuration: number;
}

export interface EngineHealth {
  engine: EngineType;
  healthy: boolean;
  version?: string;
  lastCheck: Date;
  error?: string;
}

export interface EngineInfo {
  engine: EngineType;
  available: boolean;
  rtsEnabled: boolean;
  manualScanAvailable: boolean;
}

export interface LogEntry {
  timestamp: Date;
  filePath: string;
  status: ScanStatus;
  signature?: string;
  raw: string;
}

export interface WatchOptions {
  timeout: number;
  pollInterval: number;
}

export interface DriverConfig {
  engine: EngineType;
  rtsEnabled: boolean;
  rtsLogPath: string;
  scanBinaryPath: string;
  timeout: number;
}

export interface AppConfig {
  port: number;
  uploadDir: string;
  maxFileSize: number;
  activeEngine: EngineType;
  drivers: {
    clamav: ClamAVConfig;
    trendmicro: TrendMicroConfig;
  };
}

// Both drivers share the same interface: log file + binary
export interface ClamAVConfig extends DriverConfig {
  engine: 'clamav';
}

export interface TrendMicroConfig extends DriverConfig {
  engine: 'trendmicro';
}
