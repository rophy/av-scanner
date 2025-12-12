# AV Scanner Service
# Connects to AV engines (ClamAV or Trend Micro DS Agent) running on the HOST machine
# via mounted log files and binaries.
#
# Interface for both engines:
# - RTS Log file: monitored for real-time scan results
# - Scan binary: executed for manual scans

FROM node:20-alpine AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY tsconfig.json ./
COPY src ./src
RUN npm run build

# Production image - minimal, no AV software included
# AV binaries are mounted from host
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
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/api/v1/live || exit 1

CMD ["node", "dist/index.js"]
