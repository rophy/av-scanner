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

# Test EICAR (base64 encoded to avoid triggering local AV)
echo "==> Scanning EICAR test file"
echo "WDVPIVAlQEFQWzRcUFpYNTQoUF4pN0NDKTd9JEVJQ0FSLVNUQU5EQVJELUFOVElWSVJVUy1URVNULUZJTEUhJEgrSCo=" | base64 -d | curl -s -X POST -F "file=@-;filename=eicar.com" "$API_URL/api/v1/scan" | jq -r '.status'
echo

# Test metrics
echo "==> Metrics (av_ only)"
curl -s "$API_URL/metrics" | grep "^av_" | head -10
