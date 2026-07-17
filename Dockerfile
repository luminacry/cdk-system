# syntax=docker/dockerfile:1

# Build the frontend independently so the image never relies on host artifacts.
FROM node:24-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Build Go binaries.
FROM golang:1.26-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-w -s' -o bin/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-w -s' -o bin/worker ./cmd/worker \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-w -s' -o bin/migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-w -s' -o bin/admin-create ./cmd/admin-create

# API runtime image.
FROM gcr.io/distroless/static-debian12:nonroot AS api
WORKDIR /app
COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/db/migrations /app/db/migrations
COPY --from=frontend /app/web/dist /app/web/dist
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]

# Worker runtime image.
FROM gcr.io/distroless/static-debian12:nonroot AS worker
WORKDIR /app
COPY --from=builder /app/bin/worker /app/worker
USER nonroot:nonroot
ENTRYPOINT ["/app/worker"]

# One-shot CLI image. PostgreSQL client makes admin bootstrap idempotent.
FROM alpine:3.23 AS cli
RUN apk add --no-cache ca-certificates postgresql-client \
    && addgroup -S -g 65532 nonroot \
    && adduser -S -D -H -u 65532 -G nonroot nonroot
WORKDIR /app
COPY --from=builder /app/bin/admin-create /app/admin-create
COPY --from=builder /app/bin/migrate /app/migrate
COPY --from=builder /app/db/migrations /app/db/migrations
USER nonroot
