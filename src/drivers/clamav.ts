import { spawn } from 'child_process';
import path from 'path';
import { Tail } from 'tail';
import { AntivirusDriver } from './base';
import {
  ClamAVConfig,
  EngineHealth,
  EngineInfo,
  LogEntry,
  RawEngineResult,
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

    if (!this.clamConfig.rtsEnabled) {
      this.logger.info('RTS not enabled for ClamAV, skipping RTS watch');
      return this.createUnifiedResult(
        filePath,
        fileId,
        'clean',
        'rts',
        startTime,
        undefined,
        { skipped: true, reason: 'RTS not enabled' }
      );
    }

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
      // clamdscan --fdpass --stdout --no-summary <file>
      const args = ['--fdpass', '--stdout', '--no-summary', filePath];
      const proc = spawn(this.clamConfig.scanBinaryPath, args);

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
          engine: 'clamav',
          exitCode: exitCode ?? -1,
          stdout,
          stderr,
        });
      });

      proc.on('error', (error) => {
        resolve({
          engine: 'clamav',
          exitCode: -1,
          stdout,
          stderr: error.message,
        });
      });
    });
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

  protected parseManualScanOutput(result: RawEngineResult): UnifiedResult {
    const startTime = Date.now();
    const lines = result.stdout.trim().split('\n');

    for (const line of lines) {
      const entry = this.parseLogEntry(line);
      if (entry) {
        return this.createUnifiedResult(
          entry.filePath,
          path.basename(entry.filePath),
          entry.status,
          'manual',
          startTime,
          entry.signature,
          result
        );
      }
    }

    // Exit codes: 0 = clean, 1 = virus found, 2+ = error
    const status = result.exitCode === 0 ? 'clean' : result.exitCode === 1 ? 'infected' : 'error';

    return this.createUnifiedResult(
      '',
      '',
      status,
      'manual',
      startTime,
      undefined,
      result
    );
  }

  async checkHealth(): Promise<EngineHealth> {
    try {
      // Execute scan binary with --version
      const result = await new Promise<RawEngineResult>((resolve) => {
        const proc = spawn(this.clamConfig.scanBinaryPath, ['--version']);
        let stdout = '';
        let stderr = '';

        proc.stdout.on('data', (data) => { stdout += data.toString(); });
        proc.stderr.on('data', (data) => { stderr += data.toString(); });

        proc.on('close', (exitCode) => {
          resolve({ engine: 'clamav', exitCode: exitCode ?? -1, stdout, stderr });
        });

        proc.on('error', (error) => {
          resolve({ engine: 'clamav', exitCode: -1, stdout, stderr: error.message });
        });
      });

      const healthy = result.exitCode === 0 || result.stdout.includes('ClamAV');
      const versionMatch = result.stdout.match(/ClamAV\s+([\d.]+)/);

      return {
        engine: 'clamav',
        healthy,
        version: versionMatch?.[1],
        lastCheck: new Date(),
        error: healthy ? undefined : result.stderr,
      };
    } catch (error) {
      return {
        engine: 'clamav',
        healthy: false,
        lastCheck: new Date(),
        error: error instanceof Error ? error.message : 'Unknown error',
      };
    }
  }

  getInfo(): EngineInfo {
    return {
      engine: 'clamav',
      available: true,
      rtsEnabled: this.clamConfig.rtsEnabled,
      manualScanAvailable: true,
    };
  }

  private pathMatches(logPath: string, targetPath: string): boolean {
    return path.resolve(logPath) === path.resolve(targetPath);
  }
}
