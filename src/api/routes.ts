import { Router, Request, Response, NextFunction } from 'express';
import multer from 'multer';
import path from 'path';
import { ScannerService } from '../services/scanner';
import { AppConfig, ScanRequest } from '../types';
import { Logger } from '../utils/logger';

export function createRoutes(
  scanner: ScannerService,
  config: AppConfig,
  logger: Logger
): Router {
  const router = Router();

  // Configure multer for file uploads
  const storage = multer.diskStorage({
    destination: config.uploadDir,
    filename: (req, file, cb) => {
      const fileId = scanner.generateFileId();
      const ext = path.extname(file.originalname);
      cb(null, `${fileId}${ext}`);
    },
  });

  const upload = multer({
    storage,
    limits: {
      fileSize: config.maxFileSize,
    },
  });

  // POST /scan - Upload and scan a file
  router.post(
    '/scan',
    upload.single('file'),
    async (req: Request, res: Response, next: NextFunction) => {
      try {
        if (!req.file) {
          res.status(400).json({
            error: 'No file provided',
            message: 'Please upload a file using the "file" field',
          });
          return;
        }

        const fileId = path.basename(req.file.filename, path.extname(req.file.filename));
        const scanRequest: ScanRequest = {
          fileId,
          filePath: req.file.path,
          originalName: req.file.originalname,
          mimeType: req.file.mimetype,
          size: req.file.size,
        };

        logger.info('Received scan request', {
          fileId,
          originalName: req.file.originalname,
          size: req.file.size,
          mimeType: req.file.mimetype,
        });

        const skipManualScan = req.query.skipManualScan === 'true';
        const rtsTimeout = req.query.rtsTimeout
          ? parseInt(req.query.rtsTimeout as string, 10)
          : undefined;

        const result = await scanner.scan(scanRequest, {
          skipManualScan,
          rtsTimeout,
        });

        // Return sanitized response (no file paths exposed)
        res.json({
          fileId: result.fileId,
          status: result.status,
          engine: result.engine,
          signature: result.signature,
          duration: result.totalDuration,
          phases: {
            rts: result.rtsResult
              ? {
                  status: result.rtsResult.status,
                  signature: result.rtsResult.signature,
                  duration: result.rtsResult.duration,
                }
              : null,
            manual: result.manualResult
              ? {
                  status: result.manualResult.status,
                  signature: result.manualResult.signature,
                  duration: result.manualResult.duration,
                }
              : null,
          },
        });
      } catch (error) {
        next(error);
      }
    }
  );

  // GET /health - Health check endpoint
  router.get('/health', async (req: Request, res: Response, next: NextFunction) => {
    try {
      const healthResults = await scanner.checkHealth();
      const activeEngine = scanner.getActiveEngine();

      const allHealthy = healthResults.every((h) => h.healthy);
      const activeHealthy = healthResults.find((h) => h.engine === activeEngine)?.healthy ?? false;

      res.status(activeHealthy ? 200 : 503).json({
        status: activeHealthy ? 'healthy' : 'unhealthy',
        activeEngine,
        engines: healthResults.map((h) => ({
          engine: h.engine,
          healthy: h.healthy,
          version: h.version,
          lastCheck: h.lastCheck,
          error: h.error,
        })),
      });
    } catch (error) {
      next(error);
    }
  });

  // GET /engines - List available engines
  router.get('/engines', (req: Request, res: Response) => {
    const engines = scanner.getEngineInfo();
    const activeEngine = scanner.getActiveEngine();

    res.json({
      activeEngine,
      engines: engines.map((e) => ({
        engine: e.engine,
        available: e.available,
        rtsEnabled: e.rtsEnabled,
        manualScanAvailable: e.manualScanAvailable,
        active: e.engine === activeEngine,
      })),
    });
  });

  // GET /ready - Readiness probe
  router.get('/ready', async (req: Request, res: Response) => {
    try {
      const health = await scanner.getActiveEngineHealth();
      if (health.healthy) {
        res.status(200).json({ ready: true });
      } else {
        res.status(503).json({ ready: false, error: health.error });
      }
    } catch (error) {
      res.status(503).json({
        ready: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // GET /live - Liveness probe
  router.get('/live', (req: Request, res: Response) => {
    res.status(200).json({ alive: true });
  });

  return router;
}
