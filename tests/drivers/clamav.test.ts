import { ClamAVDriver } from '../../src/drivers/clamav';
import { ClamAVConfig } from '../../src/types';
import { createLogger } from '../../src/utils/logger';

describe('ClamAVDriver', () => {
  const logger = createLogger('test');

  const createConfig = (overrides: Partial<ClamAVConfig> = {}): ClamAVConfig => ({
    engine: 'clamav',
    rtsEnabled: false,
    rtsLogPath: '/var/log/clamav/clamonacc.log',
    scanBinaryPath: '/usr/bin/clamdscan',
    timeout: 5000,
    ...overrides,
  });

  describe('parseLogEntry', () => {
    it('should parse clean file entry', () => {
      const driver = new ClamAVDriver(createConfig(), logger);
      const line = '/tmp/test.txt: OK';

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.status).toBe('clean');
      expect(result.filePath).toBe('/tmp/test.txt');
      expect(result.signature).toBeUndefined();
    });

    it('should parse infected file entry', () => {
      const driver = new ClamAVDriver(createConfig(), logger);
      const line = '/tmp/malware.exe: Eicar-Test-Signature FOUND';

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.status).toBe('infected');
      expect(result.filePath).toBe('/tmp/malware.exe');
      expect(result.signature).toBe('Eicar-Test-Signature');
    });

    it('should return null for non-matching lines', () => {
      const driver = new ClamAVDriver(createConfig(), logger);
      const line = 'Some random log line';

      const result = (driver as any).parseLogEntry(line);

      expect(result).toBeNull();
    });

    it('should handle complex signatures', () => {
      const driver = new ClamAVDriver(createConfig(), logger);
      const line = '/tmp/file.bin: Win.Trojan.Agent-123456 FOUND';

      const result = (driver as any).parseLogEntry(line);

      expect(result).not.toBeNull();
      expect(result.status).toBe('infected');
      expect(result.signature).toBe('Win.Trojan.Agent-123456');
    });
  });

  describe('getInfo', () => {
    it('should return correct engine info', () => {
      const driver = new ClamAVDriver(createConfig({ rtsEnabled: true }), logger);
      const info = driver.getInfo();

      expect(info.engine).toBe('clamav');
      expect(info.available).toBe(true);
      expect(info.rtsEnabled).toBe(true);
      expect(info.manualScanAvailable).toBe(true);
    });
  });

  describe('parseManualScanOutput', () => {
    it('should parse clean scan result', () => {
      const driver = new ClamAVDriver(createConfig(), logger);
      const rawResult = {
        engine: 'clamav' as const,
        exitCode: 0,
        stdout: '/tmp/test.txt: OK\n',
        stderr: '',
      };

      const result = (driver as any).parseManualScanOutput(rawResult);

      expect(result.status).toBe('clean');
      expect(result.phase).toBe('manual');
    });

    it('should parse infected scan result', () => {
      const driver = new ClamAVDriver(createConfig(), logger);
      const rawResult = {
        engine: 'clamav' as const,
        exitCode: 1,
        stdout: '/tmp/malware.exe: Eicar-Test-Signature FOUND\n',
        stderr: '',
      };

      const result = (driver as any).parseManualScanOutput(rawResult);

      expect(result.status).toBe('infected');
      expect(result.signature).toBe('Eicar-Test-Signature');
      expect(result.phase).toBe('manual');
    });
  });
});
