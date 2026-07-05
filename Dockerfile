# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY data/cmd/loader ./data/cmd/loader
COPY data/internal/cleaner ./data/internal/cleaner
COPY data/internal/loader ./data/internal/loader
COPY data/internal/models ./data/internal/models
COPY data/internal/pipeline ./data/internal/pipeline
RUN CGO_ENABLED=0 GOOS=linux go build -o crimes-loader ./data/cmd/loader

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.22

WORKDIR /app
COPY --from=builder /app/crimes-loader .

# Directorio para montar el dataset desde fuera del contenedor
RUN mkdir -p /app/data /app/output

# Variables de entorno con defaults configurables
ENV WORKERS=8
ENV CHUNK_SIZE=5000
ENV VERBOSE=true
ENV DATA_FILE=/app/data/crimes.csv
ENV OUTPUT_DIR=/app/output

ENTRYPOINT ["sh", "-c", \
  "./crimes-loader -file=$DATA_FILE -workers=$WORKERS -chunk=$CHUNK_SIZE -output=$OUTPUT_DIR"]
