import { unlink } from 'fs/promises';
import path from 'path';
import { v4 as uuidv4 } from 'uuid';
import { AntivirusDriver } from '../drivers';
import { DriverFactory } from './driver-factory';
import {
  AppConfig,
  EngineHealth,
  EngineInfo,
  ScanRequest,
  ScanResponse,
  ScanStatus,
  UnifiedResult,
} from '../types';
import { Logger } from '../utils/logger';

export interface ScanOptions {
  skipManualScan?: boolean;
  rtsTimeout?: number;
}

export class ScannerService {
  private driverFactory: DriverFactory;
  private config: AppConfig;
  private logger: Logger;

  constructor(driverFactory: DriverFactory, config: AppConfig, logger: Logger) {
    this.driverFactory = driverFactory;
    this.config = config;
    this.logger = logger;
  }

  async scan(request: ScanRequest, options: ScanOptions = {}): Promise<ScanResponse> {
    const startTime = Date.now();
    const driver = this.driverFactory.getActiveDriver(this.config);

    this.logger.info('Starting scan', {
      fileId: request.fileId,
      engine: driver.engine,
      originalName: request.originalName,
      size: request.size,
    });

    let rtsResult: UnifiedResult | undefined;
    let manualResult: UnifiedResult | undefined;
    let finalStatus: ScanStatus = 'clean';
    let signature: string | undefined;

    try {
      // Phase 1: Wait for RTS to complete
      if (driver.rtsEnabled) {
        this.logger.debug('Waiting for RTS scan', { fileId: request.fileId });
        rtsResult = await driver.rtsWatch(request.filePath, {
          timeout: options.rtsTimeout || this.config.drivers[driver.engine]?.timeout || 60000,
          pollInterval: 100,
        });

        if (rtsResult.status === 'infected') {
          finalStatus = 'infected';
          signature = rtsResult.signature;
          this.logger.warn('RTS detected infection', {
            fileId: request.fileId,
            signature,
          });
        } else if (rtsResult.status === 'error') {
          this.logger.warn('RTS scan error', { fileId: request.fileId, raw: rtsResult.raw });
        }
      }

      // Phase 2: Manual scan (if RTS was clean and not skipped)
      if (finalStatus === 'clean' && !options.skipManualScan) {
        this.logger.debug('Running manual scan', { fileId: request.fileId });
        manualResult = await driver.manualScan(request.filePath);

        if (manualResult.status === 'infected') {
          finalStatus = 'infected';
          signature = manualResult.signature;
          this.logger.warn('Manual scan detected infection', {
            fileId: request.fileId,
            signature,
          });
        } else if (manualResult.status === 'error') {
          finalStatus = 'error';
          this.logger.error('Manual scan error', { fileId: request.fileId, raw: manualResult.raw });
        }
      }
    } finally {
      // Always delete the file after scanning - security requirement
      await this.deleteFile(request.filePath, request.fileId);
    }

    const response: ScanResponse = {
      fileId: request.fileId,
      status: finalStatus,
      engine: driver.engine,
      signature,
      rtsResult,
      manualResult,
      totalDuration: Date.now() - startTime,
    };

    this.logger.info('Scan completed', {
      fileId: request.fileId,
      status: finalStatus,
      duration: response.totalDuration,
    });

    return response;
  }

  private async deleteFile(filePath: string, fileId: string): Promise<void> {
    try {
      await unlink(filePath);
      this.logger.debug('Deleted scanned file', { fileId, filePath });
    } catch (error) {
      // File may have been quarantined/deleted by RTS
      this.logger.debug('File already removed (likely by RTS quarantine)', {
        fileId,
        filePath,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  }

  async checkHealth(): Promise<EngineHealth[]> {
    const drivers = this.driverFactory.getAllDrivers();
    const healthChecks = await Promise.all(
      drivers.map((driver) => driver.checkHealth())
    );
    return healthChecks;
  }

  async getActiveEngineHealth(): Promise<EngineHealth> {
    const driver = this.driverFactory.getActiveDriver(this.config);
    return driver.checkHealth();
  }

  getEngineInfo(): EngineInfo[] {
    return this.driverFactory.getAllDrivers().map((driver) => driver.getInfo());
  }

  getActiveEngine(): string {
    return this.config.activeEngine;
  }

  generateFileId(): string {
    return uuidv4();
  }

  getUploadPath(fileId: string, originalName: string): string {
    const ext = path.extname(originalName);
    return path.join(this.config.uploadDir, `${fileId}${ext}`);
  }
}
