# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Instalar herramientas necesarias para compilar y descargar dependencias
RUN apk add --no-cache git

# Copiar dependencias primero para aprovechar cache de Docker
COPY go.mod go.sum ./
RUN go mod download

# Copiar el codigo fuente
COPY . .

# Compilar binarios de produccion y de mock de forma estatica
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o vps-powermix ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mockpvs ./cmd/mockpvs

# Run stage
FROM alpine:3.19

WORKDIR /app

# Agregar certificados CA para llamadas HTTPS salientes si se conecta a sandbox real
RUN apk add --no-cache ca-certificates tzdata

# Copiar binarios desde el builder
COPY --from=builder /app/vps-powermix .
COPY --from=builder /app/mockpvs .

EXPOSE 8080
EXPOSE 8081

# Comando por defecto (ejecuta el Bridge)
CMD ["./vps-powermix"]
