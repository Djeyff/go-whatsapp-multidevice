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
ARG COMMIT_SHA=dev
ENV COMMIT_SHA=$COMMIT_SHA
RUN apk add --no-cache ffmpeg libwebp-tools tzdata ca-certificates dcron
WORKDIR /app
COPY --from=builder /app/whatsapp /app/whatsapp
RUN mkdir -p /app/storages/statics/media

# Cron: delete media files older than 1 hour every 15 minutes
RUN echo "*/15 * * * * find /app/storages/statics/media -type f -mmin +60 -delete 2>/dev/null" > /etc/crontabs/root

EXPOSE 3000

# Start cron + GOWA
CMD crond -b && /app/whatsapp rest
