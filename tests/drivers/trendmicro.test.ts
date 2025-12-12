import { TrendMicroDriver } from '../../src/drivers/trendmicro';
import { TrendMicroConfig } from '../../src/types';
import { createLogger } from '../../src/utils/logger';
import { mockDsAgentLogs } from '../mocks/ds-agent-logs';

describe('TrendMicroDriver', () => {
  const logger = createLogger('test');

  const createConfig = (overrides: Partial<TrendMicroConfig> = {}): TrendMicroConfig => ({
    engine: 'trendmicro',
    rtsEnabled: true,
    rtsLogPath: '/var/log/ds_agent/ds_agent.log',
    scanBinaryPath: '/opt/ds_agent/dsa_scan',
    timeout: 5000,
    ...overrides,
  });

  describe('parseLogEntry', () => {
    it('should parse clean file SCTRL entry', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const line = mockDsAgentLogs.clean('/tmp/test.txt');

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.status).toBe('clean');
      expect(result.filePath).toBe('/tmp/test.txt');
    });

    it('should parse virus found SCTRL entry', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const line = mockDsAgentLogs.virusFound('/tmp/malware.exe', 2);

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.status).toBe('infected');
      expect(result.filePath).toBe('/tmp/malware.exe');
      expect(result.signature).toContain('Eicar');
    });

    it('should parse failed SCTRL entry as error', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const line = mockDsAgentLogs.failed('/tmp/error.bin', 3);

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.status).toBe('error');
      expect(result.filePath).toBe('/tmp/error.bin');
    });

    it('should return null for non-SCTRL entries', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const line = mockDsAgentLogs.nonSctrlEntry();

      const result = (driver as any).parseLogEntry(line);

      expect(result).toBeNull();
    });

    it('should handle paths with spaces', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const filePath = '/tmp/av-scanner/file with spaces.txt';
      const line = `2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, ${filePath}) clean`;

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.filePath).toBe(filePath);
      expect(result.status).toBe('clean');
    });
  });

  describe('getInfo', () => {
    it('should return correct engine info with RTS always enabled', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const info = driver.getInfo();

      expect(info.engine).toBe('trendmicro');
      expect(info.available).toBe(true);
      expect(info.rtsEnabled).toBe(true);  // Always true for DS Agent
      expect(info.manualScanAvailable).toBe(true);
    });
  });

  describe('parseManualScanOutput', () => {
    it('should parse clean JSON result', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const rawResult = {
        engine: 'trendmicro' as const,
        exitCode: 0,
        stdout: JSON.stringify({
          file: '/tmp/test.txt',
          infected: false,
          status: 'clean',
        }),
        stderr: '',
      };

      const result = (driver as any).parseManualScanOutput(rawResult);

      expect(result.status).toBe('clean');
      expect(result.phase).toBe('manual');
    });

    it('should parse infected JSON result', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const rawResult = {
        engine: 'trendmicro' as const,
        exitCode: 1,
        stdout: JSON.stringify({
          file: '/tmp/malware.exe',
          infected: true,
          threats: [{ name: 'Eicar-Test-Signature' }],
        }),
        stderr: '',
      };

      const result = (driver as any).parseManualScanOutput(rawResult);

      expect(result.status).toBe('infected');
      expect(result.signature).toBe('Eicar-Test-Signature');
      expect(result.phase).toBe('manual');
    });

    it('should handle non-JSON output gracefully', () => {
      const driver = new TrendMicroDriver(createConfig(), logger);
      const rawResult = {
        engine: 'trendmicro' as const,
        exitCode: 0,
        stdout: 'Scan completed: No threats found',
        stderr: '',
      };

      const result = (driver as any).parseManualScanOutput(rawResult);

      expect(result.status).toBe('clean');
    });
  });
});
