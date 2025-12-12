# AV Scanner Service - Go binary
# Built in Docker, deployed as host systemd service
#
# Interface for both engines:
# - RTS Log file: monitored for real-time scan results
# - Scan directory: where uploaded files are written

FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy source code
COPY . .

# Download dependencies and build
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o av-scanner \
    main.go

# Final image - just contains the binary for extraction
FROM scratch

COPY --from=builder /app/av-scanner /av-scanner

ENTRYPOINT ["/av-scanner"]
