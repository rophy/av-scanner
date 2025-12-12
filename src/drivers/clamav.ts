import fs from 'fs';
import path from 'path';
import { Tail } from 'tail';
import { AntivirusDriver } from './base';
import {
  ClamAVConfig,
  EngineHealth,
  EngineInfo,
  LogEntry,
  UnifiedResult,
  WatchOptions,
} from '../types';
import { Logger } from '../utils/logger';

const DEFAULT_TIMEOUT = 60000;
const DEFAULT_POLL_INTERVAL = 100;

/**
 * ClamAV Driver
 *
 * Interface:
 * - RTS Log: watches log file for on-access scan results (clamonacc output)
 * - Manual Scan: executes clamdscan binary
 */
export class ClamAVDriver extends AntivirusDriver {
  private clamConfig: ClamAVConfig;

  constructor(config: ClamAVConfig, logger: Logger) {
    super(config, logger);
    this.clamConfig = config;
  }

  async rtsWatch(
    filePath: string,
    options: WatchOptions = { timeout: DEFAULT_TIMEOUT, pollInterval: DEFAULT_POLL_INTERVAL }
  ): Promise<UnifiedResult> {
    const startTime = Date.now();
    const fileId = path.basename(filePath);

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        tail.unwatch();
        this.logger.warn(`RTS watch timeout for ${filePath}`);
        resolve(
          this.createUnifiedResult(
            filePath,
            fileId,
            'clean',
            'rts',
            startTime,
            undefined,
            { timeout: true }
          )
        );
      }, options.timeout);

      const tail = new Tail(this.clamConfig.rtsLogPath, {
        fromBeginning: false,
        follow: true,
      });

      tail.on('line', (line: string) => {
        const entry = this.parseLogEntry(line);
        if (entry && this.pathMatches(entry.filePath, filePath)) {
          clearTimeout(timeout);
          tail.unwatch();
          resolve(
            this.createUnifiedResult(
              filePath,
              fileId,
              entry.status,
              'rts',
              startTime,
              entry.signature,
              { logEntry: line }
            )
          );
        }
      });

      tail.on('error', (error: Error) => {
        clearTimeout(timeout);
        tail.unwatch();
        this.logger.error('Tail error', { error });
        reject(error);
      });
    });
  }

  async manualScan(filePath: string): Promise<UnifiedResult> {
    // RTS-only mode - manual scan not supported
    // The file should be scanned by host RTS when written to scan directory
    const startTime = Date.now();
    const fileId = path.basename(filePath);

    return this.createUnifiedResult(
      filePath,
      fileId,
      'error',
      'manual',
      startTime,
      undefined,
      { error: 'Manual scan not supported in RTS-only mode' }
    );
  }

  protected parseLogEntry(logLine: string): LogEntry | null {
    // ClamAV on-access log format (clamonacc):
    // /path/to/file: Eicar-Test-Signature FOUND
    // /path/to/file: OK
    const foundMatch = logLine.match(/^(.+):\s+(.+)\s+FOUND$/);
    if (foundMatch) {
      return {
        timestamp: new Date(),
        filePath: foundMatch[1],
        status: 'infected',
        signature: foundMatch[2],
        raw: logLine,
      };
    }

    const okMatch = logLine.match(/^(.+):\s+OK$/);
    if (okMatch) {
      return {
        timestamp: new Date(),
        filePath: okMatch[1],
        status: 'clean',
        raw: logLine,
      };
    }

    return null;
  }

  async checkHealth(): Promise<EngineHealth> {
    try {
      // RTS-only mode - check if log file exists and is readable
      await fs.promises.access(this.clamConfig.rtsLogPath, fs.constants.R_OK);

      return {
        engine: 'clamav',
        healthy: true,
        lastCheck: new Date(),
      };
    } catch (error) {
      return {
        engine: 'clamav',
        healthy: false,
        lastCheck: new Date(),
        error: `RTS log not accessible: ${this.clamConfig.rtsLogPath}`,
      };
    }
  }

  getInfo(): EngineInfo {
    return {
      engine: 'clamav',
      available: true,
      rtsEnabled: true,
      manualScanAvailable: false,
    };
  }

  private pathMatches(logPath: string, targetPath: string): boolean {
    return path.resolve(logPath) === path.resolve(targetPath);
  }
}
