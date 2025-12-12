import {
  DriverConfig,
  EngineHealth,
  EngineInfo,
  EngineType,
  LogEntry,
  ScanStatus,
  UnifiedResult,
  WatchOptions,
} from '../types';
import { Logger } from '../utils/logger';

/**
 * Base driver for AV engines - RTS-only mode
 * Watches log files for scan results, no direct binary execution
 */
export abstract class AntivirusDriver {
  protected config: DriverConfig;
  protected logger: Logger;

  constructor(config: DriverConfig, logger: Logger) {
    this.config = config;
    this.logger = logger;
  }

  get engine(): EngineType {
    return this.config.engine;
  }

  abstract rtsWatch(filePath: string, options?: WatchOptions): Promise<UnifiedResult>;

  abstract manualScan(filePath: string): Promise<UnifiedResult>;

  abstract checkHealth(): Promise<EngineHealth>;

  abstract getInfo(): EngineInfo;

  protected abstract parseLogEntry(logLine: string): LogEntry | null;

  protected normalizeStatus(
    infected: boolean,
    error: boolean
  ): ScanStatus {
    if (error) return 'error';
    return infected ? 'infected' : 'clean';
  }

  protected createUnifiedResult(
    filePath: string,
    fileId: string,
    status: ScanStatus,
    phase: 'rts' | 'manual',
    startTime: number,
    signature?: string,
    raw?: unknown
  ): UnifiedResult {
    return {
      status,
      engine: this.engine,
      signature,
      phase,
      filePath,
      fileId,
      timestamp: new Date(),
      duration: Date.now() - startTime,
      raw,
    };
  }
}
