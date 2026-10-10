FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY . .

# Compila o binário do cliente
RUN go build -o client ./Client/main.go

FROM alpine:latest
WORKDIR /app

COPY --from=builder /app/client .

# Conecta ao hostname 'server' na porta 6742
CMD ["./client", "-addr", "server:6742"]