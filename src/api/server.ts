import express, { Application } from 'express';
import { createRoutes } from './routes';
import { createErrorHandler, requestLogger, notFoundHandler } from './middleware';
import { ScannerService } from '../services/scanner';
import { AppConfig } from '../types';
import { Logger } from '../utils/logger';

export function createServer(
  scanner: ScannerService,
  config: AppConfig,
  logger: Logger
): Application {
  const app = express();

  // Middleware
  app.use(express.json());
  app.use(requestLogger(logger));

  // Security headers
  app.use((req, res, next) => {
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('X-Frame-Options', 'DENY');
    res.setHeader('X-XSS-Protection', '1; mode=block');
    next();
  });

  // API routes
  app.use('/api/v1', createRoutes(scanner, config, logger));

  // Root health check
  app.get('/', (req, res) => {
    res.json({
      service: 'av-scanner',
      version: '1.0.0',
      docs: '/api/v1',
    });
  });

  // 404 handler
  app.use(notFoundHandler);

  // Error handler
  app.use(createErrorHandler(logger));

  return app;
}

export async function startServer(
  app: Application,
  config: AppConfig,
  logger: Logger
): Promise<void> {
  return new Promise((resolve) => {
    app.listen(config.port, () => {
      logger.info(`AV Scanner service started`, {
        port: config.port,
        activeEngine: config.activeEngine,
      });
      resolve();
    });
  });
}
