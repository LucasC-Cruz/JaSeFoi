# Etapa 1: Build da aplicação Go
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copia arquivos de dependência e módulos
COPY go.mod ./
RUN go mod download

# Copia todo o código-fonte do projeto
COPY . .

# Compila o binário do servidor
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/servidor Server/servidor.go

# Etapa 2: Imagem final enxuta
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copia o binário compilado
COPY --from=builder /app/servidor /app/servidor

# Copia os assets necessários para o funcionamento padrão (telas.json)
COPY data/telas.json /app/data/telas.json

# Diretório para persistência de dados (montável como volume)
VOLUME /app/data

# Variáveis de ambiente padrão (podem ser customizadas no docker run / docker-compose)
ENV PORTA=6741 \
    PORTA_HTTP=8741 \
    DATA_DIR=/app/data \
    CONFIG_FILE=""

# Expõe as portas padrão TCP (clientes) e HTTP (comunicação inter-servidores e 2PC)
EXPOSE 6741 8741

# Executa o servidor
ENTRYPOINT ["/app/servidor"]

