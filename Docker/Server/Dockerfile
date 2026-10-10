FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copia todo o projeto da raiz para o container de build
COPY . .

# Compila o binário do servidor a partir do arquivo na pasta Server/
RUN go build -o server ./Server/servidor.go

FROM alpine:latest
WORKDIR /app

# Copia o binário gerado
COPY --from=builder /app/server .

# Copia a pasta data da raiz para /data dentro do container
# Desta forma, o comando ../data/ do servidor.go encontrará os arquivos JSON corretamente
COPY --from=builder /app/data /data

EXPOSE 6742

# Executa o servidor passando a porta 6742 como parâmetro (os.Args[1])
CMD ["./server", "6742"]