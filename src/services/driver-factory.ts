import { AntivirusDriver, ClamAVDriver, TrendMicroDriver } from '../drivers';
import {
  AppConfig,
  ClamAVConfig,
  EngineType,
  TrendMicroConfig,
} from '../types';
import { Logger } from '../utils/logger';

export class DriverFactory {
  private logger: Logger;
  private drivers: Map<EngineType, AntivirusDriver> = new Map();

  constructor(logger: Logger) {
    this.logger = logger;
  }

  createDriver(config: ClamAVConfig | TrendMicroConfig): AntivirusDriver {
    switch (config.engine) {
      case 'clamav':
        return new ClamAVDriver(config as ClamAVConfig, this.logger);
      case 'trendmicro':
        return new TrendMicroDriver(config as TrendMicroConfig, this.logger);
      default:
        throw new Error(`Unknown engine type: ${(config as any).engine}`);
    }
  }

  initializeDrivers(appConfig: AppConfig): void {
    // Initialize ClamAV driver
    if (appConfig.drivers.clamav) {
      try {
        const clamavDriver = this.createDriver(appConfig.drivers.clamav);
        this.drivers.set('clamav', clamavDriver);
        this.logger.info('ClamAV driver initialized');
      } catch (error) {
        this.logger.warn('Failed to initialize ClamAV driver', { error });
      }
    }

    // Initialize Trend Micro driver
    if (appConfig.drivers.trendmicro) {
      try {
        const trendmicroDriver = this.createDriver(appConfig.drivers.trendmicro);
        this.drivers.set('trendmicro', trendmicroDriver);
        this.logger.info('Trend Micro driver initialized');
      } catch (error) {
        this.logger.warn('Failed to initialize Trend Micro driver', { error });
      }
    }
  }

  getDriver(engine: EngineType): AntivirusDriver | undefined {
    return this.drivers.get(engine);
  }

  getActiveDriver(appConfig: AppConfig): AntivirusDriver {
    const driver = this.drivers.get(appConfig.activeEngine);
    if (!driver) {
      throw new Error(`Active engine '${appConfig.activeEngine}' is not available`);
    }
    return driver;
  }

  getAllDrivers(): AntivirusDriver[] {
    return Array.from(this.drivers.values());
  }

  getAvailableEngines(): EngineType[] {
    return Array.from(this.drivers.keys());
  }
}
