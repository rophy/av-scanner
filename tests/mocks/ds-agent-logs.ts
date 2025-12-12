// Mock DS Agent log entries for testing

export const mockDsAgentLogs = {
  // Clean file scan
  clean: (filePath: string) =>
    `2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, ${filePath}) clean`,

  // Virus detected
  virusFound: (filePath: string, threatCount: number = 1) =>
    `2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, ${filePath}) virus found: ${threatCount} Eicar-Test-Signature`,

  // Scan failure
  failed: (filePath: string, errorCode: number = 3) =>
    `2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, ${filePath}) failed: ${errorCode}`,

  // Multiple threats
  multipleThreats: (filePath: string) =>
    `2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, ${filePath}) virus found: 3 Trojan.Generic, Backdoor.Agent, Worm.Conficker`,

  // Non-SCTRL log entries (should be ignored)
  nonSctrlEntry: () =>
    `2025-11-21 13:11:30.123456: [ds_am/4] | [INFO] Starting real-time scan engine`,

  // Generate a sequence of log entries
  generateLogSequence: (filePath: string, isInfected: boolean): string[] => {
    const logs: string[] = [];
    logs.push(`2025-11-21 13:11:30.123456: [ds_am/4] | [INFO] Starting real-time scan engine`);
    logs.push(`2025-11-21 13:11:30.234567: [ds_am/4] | [DEBUG] Scanning file: ${filePath}`);

    if (isInfected) {
      logs.push(mockDsAgentLogs.virusFound(filePath));
    } else {
      logs.push(mockDsAgentLogs.clean(filePath));
    }

    return logs;
  },
};

// Sample log file content for testing
export const sampleDsAgentLogContent = `
2025-11-21 13:10:00.000000: [ds_am/4] | [INFO] DS Agent started
2025-11-21 13:10:01.000000: [ds_am/4] | [INFO] Real-time scan enabled
2025-11-21 13:11:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, /tmp/av-scanner/test-clean.txt) clean
2025-11-21 13:12:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, /tmp/av-scanner/test-infected.exe) virus found: 1 Eicar-Test-Signature
2025-11-21 13:13:31.654744: [ds_am/4] | [SCTRL] (0000-0000-0000, /tmp/av-scanner/test-error.bin) failed: 3
`.trim();
