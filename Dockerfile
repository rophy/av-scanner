# AV Scanner Service - RTS-only mode
# Watches AV engine log files on the HOST machine for scan results.
# Files uploaded to the scan directory trigger host RTS (Real-Time Scan).
#
# Interface for both engines:
# - RTS Log file: monitored for real-time scan results
# - Scan directory: shared with host for RTS triggers

FROM node:20-slim AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY tsconfig.json ./
COPY src ./src
RUN npm run build

# Production image - RTS-only mode
# Watches host AV log files for scan results, no direct binary execution
FROM node:20-alpine

WORKDIR /app

# Create non-root user
RUN addgroup -g 1001 -S avscanner && \
    adduser -S -D -H -u 1001 -h /app -s /sbin/nologin -G avscanner avscanner

# Copy built application
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY package.json ./

# Create scan directory (will be bind-mounted from host)
RUN mkdir -p /tmp/av-scanner && chown avscanner:avscanner /tmp/av-scanner

ENV NODE_ENV=production
ENV PORT=3000
ENV UPLOAD_DIR=/tmp/av-scanner

EXPOSE 3000

USER avscanner

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD node -e "require('http').get('http://localhost:3000/api/v1/live', (r) => process.exit(r.statusCode === 200 ? 0 : 1)).on('error', () => process.exit(1))"

CMD ["node", "dist/index.js"]
