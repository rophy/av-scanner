import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const scanErrors = new Counter('scan_errors');
const cleanFiles = new Counter('clean_files_sent');
const infectedFiles = new Counter('infected_files_sent');
const correctResults = new Rate('correct_results');
const scanDuration = new Trend('scan_duration_ms');

// Test configuration
export const options = {
  scenarios: {
    stress_test: {
      executor: 'constant-vus',
      vus: 10,
      duration: '30s',
    },
  },
  thresholds: {
    'correct_results': ['rate==1.0'],  // 100% correct results required
    'http_req_failed': ['rate<0.01'],  // <1% HTTP errors
  },
};

// Generate EICAR test string with 'O' replaced by 'x' to avoid AV detection
// https://en.wikipedia.org/wiki/EICAR_test_file
function generateEicar() {
  const broken = 'X5x!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*';
  return broken.replace('x', 'O');
}

// Test file contents
const cleanFileContent = 'This is a clean test file for stress testing';
const eicarContent = generateEicar();

const API_URL = __ENV.API_URL || 'http://10.113.202.142:3000';

export default function () {
  // 80% clean, 20% infected
  const isInfected = Math.random() < 0.2;

  const content = isInfected ? eicarContent : cleanFileContent;
  const fileName = isInfected ? 'eicar.com' : 'clean.txt';
  const expectedStatus = isInfected ? 'infected' : 'clean';

  // Track what we're sending
  if (isInfected) {
    infectedFiles.add(1);
  } else {
    cleanFiles.add(1);
  }

  const res = http.post(`${API_URL}/api/v1/scan`, {
    file: http.file(content, fileName),
  });

  // Parse response
  let result;
  try {
    result = JSON.parse(res.body);
  } catch (e) {
    scanErrors.add(1);
    correctResults.add(0);
    fail(`Failed to parse response: ${res.body}`);
    return;
  }

  // Track scan duration from API
  if (result.duration) {
    scanDuration.add(result.duration);
  }

  // Verify correct result
  const isCorrect = result.status === expectedStatus;
  correctResults.add(isCorrect ? 1 : 0);

  if (!isCorrect) {
    scanErrors.add(1);
    console.error(`MISMATCH: sent=${fileName} expected=${expectedStatus} got=${result.status} fileId=${result.fileId}`);
  }

  // Standard checks
  check(res, {
    'status is 200': (r) => r.status === 200,
    'has status field': (r) => result.status !== undefined,
    'correct scan result': () => isCorrect,
  });
}

export function handleSummary(data) {
  const clean = data.metrics.clean_files_sent ? data.metrics.clean_files_sent.values.count : 0;
  const infected = data.metrics.infected_files_sent ? data.metrics.infected_files_sent.values.count : 0;
  const errors = data.metrics.scan_errors ? data.metrics.scan_errors.values.count : 0;
  const correctRate = data.metrics.correct_results ? data.metrics.correct_results.values.rate : 0;
  const avgDuration = data.metrics.scan_duration_ms ? data.metrics.scan_duration_ms.values.avg : 0;

  console.log('\n========== STRESS TEST SUMMARY ==========');
  console.log(`Total requests:    ${clean + infected}`);
  console.log(`Clean files:       ${clean} (${((clean/(clean+infected))*100).toFixed(1)}%)`);
  console.log(`Infected files:    ${infected} (${((infected/(clean+infected))*100).toFixed(1)}%)`);
  console.log(`Correct results:   ${(correctRate * 100).toFixed(2)}%`);
  console.log(`Scan errors:       ${errors}`);
  console.log(`Avg scan duration: ${avgDuration.toFixed(1)}ms`);
  console.log('==========================================\n');

  return {
    stdout: JSON.stringify(data, null, 2),
  };
}
