# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY go.mod ./
# go mod tidy resuelve y verifica las dependencias (genera go.sum)
COPY cmd ./cmd
COPY internal ./internal
RUN go mod tidy

# Se compilan los tres ejecutables del módulo ML:
#   trainer → entrenamiento local (Entregable 1)
#   node    → nodo trabajador del clúster (TCP)
#   api     → coordinador del clúster + servicio REST/WebSocket
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/trainer ./cmd/trainer && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/node ./cmd/node && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/api ./cmd/api

# ── Runtime stage ──────────────────────────────────────────────────────────────
FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates curl && update-ca-certificates

COPY --from=builder /out/trainer /usr/local/bin/trainer
COPY --from=builder /out/node    /usr/local/bin/node
COPY --from=builder /out/api     /usr/local/bin/api

VOLUME ["/data", "/output"]

# El servicio de docker-compose decide qué binario ejecutar vía `command`.
ENTRYPOINT []
CMD ["trainer", "-data", "/output/clean_records_sample.json", "-out", "/output/train_results.json"]
