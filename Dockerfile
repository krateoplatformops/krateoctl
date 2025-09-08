# Build stage
FROM golang:1.25.0-alpine3.21 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o krateoctl

# UPX stage
FROM alpine:3.21 AS shrinker

RUN apk add --no-cache upx

COPY --from=builder /app/krateoctl /app/krateoctl
RUN upx --best --lzma /app/krateoctl

# Final Image
FROM golang:1.25.0-alpine3.21

LABEL org.opencontainers.image.source="https://github.com/krateoplatformops/krateoctl"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=shrinker /app/krateoctl /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/krateoctl"]