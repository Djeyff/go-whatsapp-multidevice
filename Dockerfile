# syntax=docker/dockerfile:1
# Zeabur: APP_PORT=${PORT} env var must be set so GOWA binds to the correct port
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache gcc musl-dev gcompat
WORKDIR /whatsapp
COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src/ .
RUN go build -ldflags="-w -s" -o /app/whatsapp

FROM alpine:3.21
RUN apk add --no-cache ffmpeg libwebp-tools tzdata ca-certificates
WORKDIR /app
COPY --from=builder /app/whatsapp /app/whatsapp
RUN mkdir -p /app/storages
EXPOSE 3000
CMD ["/app/whatsapp", "rest"]
