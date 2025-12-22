FROM docker.io/library/golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o kube-auth-proxy .

FROM docker.io/library/alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/kube-auth-proxy /usr/local/bin/
USER 65534:65534
ENTRYPOINT ["kube-auth-proxy"]
