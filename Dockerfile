FROM golang:1.24.5-alpine

WORKDIR /app

RUN apk add --no-cache \
    git \
    bash \
    curl \
    tzdata

RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.15.2/migrate.linux-amd64.tar.gz \
    | tar xvz \
 && mv migrate /usr/local/bin/migrate

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV PORT=8080
ENV TZ=Asia/Jakarta

EXPOSE 8080

CMD ["go", "run", "main.go"]
