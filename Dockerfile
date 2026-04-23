# =========================
# STAGE 1: BUILD
# =========================
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o app

# =========================
# STAGE 2: RUNTIME
# =========================
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache \
    ca-certificates \
    bash

COPY --from=builder /app/app .

EXPOSE 5002

CMD ["./app"]
