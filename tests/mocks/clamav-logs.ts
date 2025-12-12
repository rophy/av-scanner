// Mock ClamAV log entries and command outputs for testing

export const mockClamavOutputs = {
  // Clean file - clamdscan output
  cleanScan: (filePath: string) => ({
    stdout: `${filePath}: OK\n`,
    stderr: '',
    exitCode: 0,
  }),

  // Infected file - clamdscan output
  infectedScan: (filePath: string, signature: string = 'Eicar-Test-Signature') => ({
    stdout: `${filePath}: ${signature} FOUND\n`,
    stderr: '',
    exitCode: 1,
  }),

  // Scan error
  errorScan: (filePath: string) => ({
    stdout: '',
    stderr: `ERROR: Can't access file ${filePath}\n`,
    exitCode: 2,
  }),

  // Version output
  versionOutput: () => ({
    stdout: 'ClamAV 1.2.0/27150/Thu Nov 21 09:00:00 2024\n',
    stderr: '',
    exitCode: 0,
  }),
};

// Mock clamonacc (on-access) log entries
export const mockClamonaccLogs = {
  clean: (filePath: string) => `${filePath}: OK`,

  infected: (filePath: string, signature: string = 'Eicar-Test-Signature') =>
    `${filePath}: ${signature} FOUND`,

  // Generate a sequence of on-access log entries
  generateLogSequence: (filePath: string, isInfected: boolean): string[] => {
    const logs: string[] = [];

    if (isInfected) {
      logs.push(mockClamonaccLogs.infected(filePath));
    } else {
      logs.push(mockClamonaccLogs.clean(filePath));
    }

    return logs;
  },
};

// Sample clamonacc log file content
export const sampleClamonaccLogContent = `
/tmp/av-scanner/clean-file.txt: OK
/tmp/av-scanner/infected-file.exe: Eicar-Test-Signature FOUND
/tmp/av-scanner/another-clean.pdf: OK
/tmp/av-scanner/trojan.bin: Win.Trojan.Agent-123456 FOUND
`.trim();
