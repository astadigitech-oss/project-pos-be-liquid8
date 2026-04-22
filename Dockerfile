FROM golang:1.23-alpine

WORKDIR /app

RUN apk add --no-cache \
    git \
    bash \
    curl \
    postgresql-client \
    tzdata

COPY . .

RUN go mod download

ENV APP_PORT=8080
ENV TZ=Asia/Jakarta

EXPOSE 8080

CMD ["go", "run", "main.go"]
