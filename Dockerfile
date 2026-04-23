# =========================
# STAGE 1: BUILD
# =========================
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build semua binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app ./main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o migrate ./cmd/migrate/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o seeder ./cmd/seeders/main.go

# =========================
# STAGE 2: RUNTIME
# =========================
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache \
ca-certificates \
bash \
tzdata

# Set timezone (contoh: Asia/Jakarta)
ENV TZ=Asia/Jakarta

# Apply timezone ke sistem
RUN cp /usr/share/zoneinfo/$TZ /etc/localtime && \
    echo $TZ > /etc/timezone


# Copy semua binary
COPY --from=builder /app/app .
COPY --from=builder /app/migrate .
COPY --from=builder /app/seeder .

EXPOSE 5002

CMD ["./app"]
