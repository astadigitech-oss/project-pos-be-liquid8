# =========================
# BUILD STAGE
# =========================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# install git (penting untuk go mod)
RUN apk add --no-cache git bash

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# build binary
RUN go build -o app

# =========================
# RUN STAGE (lebih ringan)
# =========================
FROM alpine:latest

WORKDIR /app

# optional (timezone + cert)
RUN apk add --no-cache ca-certificates

COPY --from=builder /app/app .

EXPOSE 5002

CMD ["./app"]
