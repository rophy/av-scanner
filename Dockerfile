# AV Scanner Service - Go binary
# Built in Docker, deployed as host systemd service
#
# Interface for both engines:
# - RTS Log file: monitored for real-time scan results
# - Scan directory: where uploaded files are written

FROM docker.io/library/golang:1.23-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy source code
COPY . .

# Download dependencies and build
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s \
        -X github.com/rophy/av-scanner/internal/version.Version=${VERSION} \
        -X github.com/rophy/av-scanner/internal/version.Commit=${COMMIT} \
        -X github.com/rophy/av-scanner/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o av-scanner \
    main.go

# Final image - just contains the binary for extraction
FROM scratch

COPY --from=builder /app/av-scanner /av-scanner

ENTRYPOINT ["/av-scanner"]
