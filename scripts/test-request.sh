#!/bin/bash
# Quick one-shot test against av-scanner API
# Usage: ./scripts/test-request.sh [API_URL]

set -e

API_URL="${1:-http://localhost:3000}"

echo "Testing av-scanner at $API_URL"
echo

# Test health endpoint
echo "==> Health check"
curl -s "$API_URL/api/v1/health" | jq -r '.status'
echo

# Test clean file
echo "==> Scanning clean file"
echo "clean test content" | curl -s -X POST -F "file=@-;filename=clean.txt" "$API_URL/api/v1/scan" | jq -r '.status'
echo

# Test EICAR ('O' replaced with 'x' to avoid triggering local AV, fixed at runtime)
echo "==> Scanning EICAR test file"
echo 'X5x!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*' | sed 's/x/O/' | curl -s -X POST -F "file=@-;filename=eicar.com" "$API_URL/api/v1/scan" | jq -r '.status'
echo

# Test metrics
echo "==> Metrics (av_ only)"
curl -s "$API_URL/metrics" | grep "^av_" | head -10
