import { AppConfig, EngineType } from '../types';

export function loadConfig(): AppConfig {
  const activeEngine = (process.env.AV_ENGINE || 'clamav') as EngineType;

  return {
    port: parseInt(process.env.PORT || '3000', 10),
    uploadDir: process.env.UPLOAD_DIR || '/tmp/av-scanner',
    maxFileSize: parseInt(process.env.MAX_FILE_SIZE || '104857600', 10), // 100MB default
    activeEngine,

    drivers: {
      // Both drivers have the same interface: rtsLogPath + scanBinaryPath
      clamav: {
        engine: 'clamav',
        rtsEnabled: process.env.CLAMAV_RTS_ENABLED === 'true',
        rtsLogPath: process.env.CLAMAV_RTS_LOG_PATH || '/var/log/clamav/clamonacc.log',
        scanBinaryPath: process.env.CLAMAV_SCAN_BINARY_PATH || '/usr/bin/clamdscan',
        timeout: parseInt(process.env.CLAMAV_TIMEOUT || '60000', 10),
      },
      trendmicro: {
        engine: 'trendmicro',
        rtsEnabled: true, // Always true for DS Agent
        rtsLogPath: process.env.TM_RTS_LOG_PATH || '/var/log/ds_agent/ds_agent.log',
        scanBinaryPath: process.env.TM_SCAN_BINARY_PATH || '/opt/ds_agent/dsa_scan',
        timeout: parseInt(process.env.TM_TIMEOUT || '60000', 10),
      },
    },
  };
}

export function validateConfig(config: AppConfig): void {
  if (!['clamav', 'trendmicro'].includes(config.activeEngine)) {
    throw new Error(`Invalid active engine: ${config.activeEngine}`);
  }

  if (config.port < 1 || config.port > 65535) {
    throw new Error(`Invalid port: ${config.port}`);
  }

  if (config.maxFileSize < 1) {
    throw new Error(`Invalid max file size: ${config.maxFileSize}`);
  }
}
