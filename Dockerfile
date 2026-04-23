# =========================
# STAGE 1: BUILD
# =========================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependency untuk build
RUN apk add --no-cache git

# Cache dependency
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary (lebih optimal & kecil)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o app

# =========================
# STAGE 2: RUNTIME
# =========================
FROM alpine:3.19

WORKDIR /app

# Install hanya yang dibutuhkan di runtime
RUN apk add --no-cache \
    ca-certificates \
    bash

# Copy binary dari builder
COPY --from=builder /app/app .

# Expose port sesuai app kamu
EXPOSE 5002

# Jalankan app
CMD ["./app"]
