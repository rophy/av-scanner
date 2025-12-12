import { Request, Response, NextFunction, ErrorRequestHandler } from 'express';
import { Logger } from '../utils/logger';

export function createErrorHandler(logger: Logger): ErrorRequestHandler {
  return (err: Error, req: Request, res: Response, next: NextFunction) => {
    logger.error('Request error', {
      error: err.message,
      stack: err.stack,
      method: req.method,
      path: req.path,
    });

    // Handle multer errors
    if (err.name === 'MulterError') {
      if ((err as any).code === 'LIMIT_FILE_SIZE') {
        res.status(413).json({
          error: 'File too large',
          message: 'The uploaded file exceeds the maximum allowed size',
        });
        return;
      }
    }

    res.status(500).json({
      error: 'Internal server error',
      message: process.env.NODE_ENV === 'development' ? err.message : 'An unexpected error occurred',
    });
  };
}

export function requestLogger(logger: Logger) {
  return (req: Request, res: Response, next: NextFunction) => {
    const start = Date.now();

    res.on('finish', () => {
      const duration = Date.now() - start;
      logger.info('Request completed', {
        method: req.method,
        path: req.path,
        status: res.statusCode,
        duration,
      });
    });

    next();
  };
}

export function notFoundHandler(req: Request, res: Response) {
  res.status(404).json({
    error: 'Not found',
    message: `Route ${req.method} ${req.path} not found`,
  });
}
