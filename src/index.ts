import { mkdir } from 'fs/promises';
import { loadConfig, validateConfig } from './config';
import { createServer, startServer } from './api';
import { DriverFactory, ScannerService } from './services';
import { createLogger } from './utils/logger';

async function main(): Promise<void> {
  const logger = createLogger('av-scanner');

  try {
    // Load and validate configuration
    const config = loadConfig();
    validateConfig(config);

    logger.info('Configuration loaded', {
      activeEngine: config.activeEngine,
      uploadDir: config.uploadDir,
      maxFileSize: config.maxFileSize,
    });

    // Ensure upload directory exists
    await mkdir(config.uploadDir, { recursive: true });
    logger.info('Upload directory ready', { path: config.uploadDir });

    // Initialize driver factory and drivers
    const driverFactory = new DriverFactory(logger);
    driverFactory.initializeDrivers(config);

    const availableEngines = driverFactory.getAvailableEngines();
    logger.info('Drivers initialized', { engines: availableEngines });

    // Create scanner service
    const scanner = new ScannerService(driverFactory, config, logger);

    // Run initial health check
    const health = await scanner.checkHealth();
    for (const h of health) {
      if (h.healthy) {
        logger.info(`${h.engine} is healthy`, { version: h.version });
      } else {
        logger.warn(`${h.engine} is unhealthy`, { error: h.error });
      }
    }

    // Create and start server
    const app = createServer(scanner, config, logger);
    await startServer(app, config, logger);
  } catch (error) {
    logger.error('Failed to start service', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
    });
    process.exit(1);
  }
}

// Handle graceful shutdown
process.on('SIGTERM', () => {
  console.log('SIGTERM received, shutting down...');
  process.exit(0);
});

process.on('SIGINT', () => {
  console.log('SIGINT received, shutting down...');
  process.exit(0);
});

main();
