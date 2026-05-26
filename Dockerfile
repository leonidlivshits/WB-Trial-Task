FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/trends-service ./cmd/trends-service

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/trends-service /usr/local/bin/trends-service
COPY docs ./docs

EXPOSE 8080

ENTRYPOINT ["trends-service"]
