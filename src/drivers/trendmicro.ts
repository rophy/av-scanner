import { spawn } from 'child_process';
import path from 'path';
import { Tail } from 'tail';
import { AntivirusDriver } from './base';
import {
  EngineHealth,
  EngineInfo,
  LogEntry,
  RawEngineResult,
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
    const startTime = Date.now();
    const fileId = path.basename(filePath);

    const result = await this.executeScan(filePath);
    const parsed = this.parseManualScanOutput(result);

    return {
      ...parsed,
      filePath,
      fileId,
      duration: Date.now() - startTime,
    };
  }

  private executeScan(filePath: string): Promise<RawEngineResult> {
    return new Promise((resolve) => {
      // dsa_scan --target <file> --json
      const args = ['--target', filePath, '--json'];
      const proc = spawn(this.tmConfig.scanBinaryPath, args);

      let stdout = '';
      let stderr = '';

      proc.stdout.on('data', (data) => {
        stdout += data.toString();
      });

      proc.stderr.on('data', (data) => {
        stderr += data.toString();
      });

      proc.on('close', (exitCode) => {
        resolve({
          engine: 'trendmicro',
          exitCode: exitCode ?? -1,
          stdout,
          stderr,
        });
      });

      proc.on('error', (error) => {
        resolve({
          engine: 'trendmicro',
          exitCode: -1,
          stdout,
          stderr: error.message,
        });
      });
    });
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

  protected parseManualScanOutput(result: RawEngineResult): UnifiedResult {
    const startTime = Date.now();

    // Try to parse JSON output from dsa_scan --json
    try {
      const jsonResult = JSON.parse(result.stdout);

      const infected = jsonResult.infected === true ||
                      jsonResult.status === 'infected' ||
                      (jsonResult.threats && jsonResult.threats.length > 0);

      const signature = jsonResult.threats?.[0]?.name ||
                       jsonResult.signature ||
                       jsonResult.malware_name;

      return this.createUnifiedResult(
        jsonResult.file || '',
        '',
        infected ? 'infected' : 'clean',
        'manual',
        startTime,
        signature,
        jsonResult
      );
    } catch {
      this.logger.debug('Failed to parse JSON output, falling back to text parsing');
    }

    // Text-based parsing fallback
    const infected = result.stdout.toLowerCase().includes('infected') ||
                    result.stdout.toLowerCase().includes('virus') ||
                    result.stdout.toLowerCase().includes('malware');

    const signatureMatch = result.stdout.match(/(?:virus|malware|threat):\s*(.+)/i);

    return this.createUnifiedResult(
      '',
      '',
      infected ? 'infected' : result.exitCode === 0 ? 'clean' : 'error',
      'manual',
      startTime,
      signatureMatch?.[1]?.trim(),
      result
    );
  }

  async checkHealth(): Promise<EngineHealth> {
    try {
      const result = await new Promise<RawEngineResult>((resolve) => {
        const proc = spawn(this.tmConfig.scanBinaryPath, ['--version']);
        let stdout = '';
        let stderr = '';

        proc.stdout.on('data', (data) => { stdout += data.toString(); });
        proc.stderr.on('data', (data) => { stderr += data.toString(); });

        proc.on('close', (exitCode) => {
          resolve({ engine: 'trendmicro', exitCode: exitCode ?? -1, stdout, stderr });
        });

        proc.on('error', (error) => {
          resolve({ engine: 'trendmicro', exitCode: -1, stdout, stderr: error.message });
        });
      });

      const healthy = result.exitCode === 0 || result.stdout.length > 0;
      const versionMatch = result.stdout.match(/(?:version|v)[\s:]*([^\s]+)/i);

      return {
        engine: 'trendmicro',
        healthy,
        version: versionMatch?.[1],
        lastCheck: new Date(),
        error: healthy ? undefined : result.stderr,
      };
    } catch (error) {
      return {
        engine: 'trendmicro',
        healthy: false,
        lastCheck: new Date(),
        error: error instanceof Error ? error.message : 'Unknown error',
      };
    }
  }

  getInfo(): EngineInfo {
    return {
      engine: 'trendmicro',
      available: true,
      rtsEnabled: true, // DS Agent RTS is always on
      manualScanAvailable: true,
    };
  }

  private pathMatches(logPath: string, targetPath: string): boolean {
    return path.resolve(logPath) === path.resolve(targetPath);
  }
}
