# ==============================================================================
# Dockerfile: ms-checkin (Go Alpine - Multi-Stage Non-Root)
# ==============================================================================

# Etapa 1: Compilación estática
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/ms-checkin-bin ./cmd/api

# Etapa 2: Imagen mínima de ejecución
FROM alpine:3.20 AS runner

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 10001 appuser

COPY --from=builder /app/ms-checkin-bin ./

USER appuser

ENV PORT=8007

EXPOSE 8007

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8007/health || exit 1

CMD ["./ms-checkin-bin"]
