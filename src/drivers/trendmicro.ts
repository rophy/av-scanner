import fs from 'fs';
import path from 'path';
import { Tail } from 'tail';
import { AntivirusDriver } from './base';
import {
  EngineHealth,
  EngineInfo,
  LogEntry,
  TrendMicroConfig,
  UnifiedResult,
  WatchOptions,
} from '../types';
import { Logger } from '../utils/logger';

const DEFAULT_TIMEOUT = 60000;
const DEFAULT_POLL_INTERVAL = 100;

/**
 * Trend Micro DS Agent Driver
 *
 * Interface:
 * - RTS Log: watches log file for SCTRL entries
 * - Manual Scan: executes dsa_scan binary
 */
export class TrendMicroDriver extends AntivirusDriver {
  private tmConfig: TrendMicroConfig;

  constructor(config: TrendMicroConfig, logger: Logger) {
    super(config, logger);
    this.tmConfig = config;
  }

  async rtsWatch(
    filePath: string,
    options: WatchOptions = { timeout: DEFAULT_TIMEOUT, pollInterval: DEFAULT_POLL_INTERVAL }
  ): Promise<UnifiedResult> {
    const startTime = Date.now();
    const fileId = path.basename(filePath);

    // DS Agent RTS is always on - we must watch the log for SCTRL entries
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
            { timeout: true, reason: 'No SCTRL entry found within timeout' }
          )
        );
      }, options.timeout);

      const tail = new Tail(this.tmConfig.rtsLogPath, {
        fromBeginning: false,
        follow: true,
      });

      tail.on('line', (line: string) => {
        const entry = this.parseLogEntry(line);
        if (entry && this.pathMatches(entry.filePath, filePath)) {
          clearTimeout(timeout);
          tail.unwatch();

          this.logger.info('RTS detected file scan', {
            filePath,
            status: entry.status,
            signature: entry.signature,
          });

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
        this.logger.error('Tail error on DS Agent log', { error });
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
    // DS Agent SCTRL log format:
    // 2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, /path/to/file) virus found: 2 ...
    // 2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, /path/to/file) failed: 3
    // 2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, /path/to/file) clean

    const sctrlMatch = logLine.match(
      /^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+):\s+\[ds_am\/\d+\]\s+\|\s+\[SCTRL\]\s+\([^,]+,\s*([^)]+)\)\s+(.+)$/
    );

    if (!sctrlMatch) {
      return null;
    }

    const [, timestampStr, filePath, result] = sctrlMatch;
    const timestamp = new Date(timestampStr.replace(' ', 'T'));

    // Check for virus found
    const virusMatch = result.match(/virus found:\s*(\d+)/i);
    if (virusMatch) {
      const signatureMatch = result.match(/virus found:\s*\d+\s*(.+)?/i);
      return {
        timestamp,
        filePath: filePath.trim(),
        status: 'infected',
        signature: signatureMatch?.[1]?.trim() || 'Unknown',
        raw: logLine,
      };
    }

    // Check for failure (failed: 3 is not infection, it's scan failure)
    const failedMatch = result.match(/failed:\s*(\d+)/i);
    if (failedMatch) {
      this.logger.warn('SCTRL scan failed', { filePath, errorCode: failedMatch[1] });
      return {
        timestamp,
        filePath: filePath.trim(),
        status: 'error',
        raw: logLine,
      };
    }

    // Check for clean
    if (result.toLowerCase().includes('clean') || result.toLowerCase().includes('ok')) {
      return {
        timestamp,
        filePath: filePath.trim(),
        status: 'clean',
        raw: logLine,
      };
    }

    // Any other SCTRL entry for this file - assume it was processed
    return {
      timestamp,
      filePath: filePath.trim(),
      status: 'clean',
      raw: logLine,
    };
  }

  async checkHealth(): Promise<EngineHealth> {
    try {
      // RTS-only mode - check if log file exists and is readable
      await fs.promises.access(this.tmConfig.rtsLogPath, fs.constants.R_OK);

      return {
        engine: 'trendmicro',
        healthy: true,
        lastCheck: new Date(),
      };
    } catch (error) {
      return {
        engine: 'trendmicro',
        healthy: false,
        lastCheck: new Date(),
        error: `RTS log not accessible: ${this.tmConfig.rtsLogPath}`,
      };
    }
  }

  getInfo(): EngineInfo {
    return {
      engine: 'trendmicro',
      available: true,
      rtsEnabled: true,
      manualScanAvailable: false,
    };
  }

  private pathMatches(logPath: string, targetPath: string): boolean {
    return path.resolve(logPath) === path.resolve(targetPath);
  }
}
