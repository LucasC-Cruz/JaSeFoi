# ==============================================================================
# Makefile - Projeto JáSeFoi (Sistema Distribuído com 2PC, WAL e Docker)
# ==============================================================================

SHELL := bash
APP_NAME := jasefoi
IMAGE_NAME := jasefoi-server

# Variáveis configuráveis por linha de comando
# Exemplo: make docker-run-s1 VIZINHOS=http://192.168.1.102:8742
SERVER_ID ?= server-1
PORTA ?= 6741
PORTA_HTTP ?= 8741
DATA_DIR ?= ./data
VIZINHOS ?=
ADDR ?= localhost:6741
CIDADES ?=
CIDADES_INTER ?=

.PHONY: help build run-client run-s1 run-s2 run-s3 run-custom \
        docker-build docker-up docker-down docker-logs docker-run \
        docker-run-s1 docker-run-s2 docker-run-s3 \
        test-teste1 test-teste2pc test-commit-crash test-wal test-lamport \
        test-2pc1 test-2pc2 test-2pc3 test-wal1 test-wal2 test-wal3 test-wal4 \
        test-remoto clean

# ------------------------------------------------------------------------------
# Ajuda / Menu de Comandos
# ------------------------------------------------------------------------------
help:
	@echo "======================================================================"
	@echo "                      Comandos do Makefile JáSeFoi                    "
	@echo "======================================================================"
	@echo " Execução Local / Go:"
	@echo "   make build              - Compila o binário do servidor e do cliente"
	@echo "   make run-s1             - Executa Servidor 1 localmente (configs/server1.json)"
	@echo "   make run-s2             - Executa Servidor 2 localmente (configs/server2.json)"
	@echo "   make run-s3             - Executa Servidor 3 localmente (configs/server3.json)"
	@echo "   make run-custom VIZINHOS=\"http://IP:PORTA\" [PORTA=...] [PORTA_HTTP=...]"
	@echo "                           - Executa servidor passando IPs dos vizinhos via CLI"
	@echo "   make run-client         - Inicia o cliente interativo em Client/main.go"
	@echo ""
	@echo " Docker em Computadores Separados (Passando IPs dos Vizinhos):"
	@echo "   make docker-build       - Constrói a imagem Docker (jasefoi-server)"
	@echo ""
	@echo "   make docker-run-s1 VIZINHOS=\"http://IP_PC2:8742\""
	@echo "                           - Sobe Servidor 1 (Salvador) apontando para o IP do PC2"
	@echo ""
	@echo "   make docker-run-s2 VIZINHOS=\"http://IP_PC1:8741,http://IP_PC3:8743\""
	@echo "                           - Sobe Servidor 2 (Feira/Serrinha) apontando para PC1 e PC3"
	@echo ""
	@echo "   make docker-run-s3 VIZINHOS=\"http://IP_PC2:8742\""
	@echo "                           - Sobe Servidor 3 (Serrinha/Valença) apontando para PC2"
	@echo ""
	@echo "   make docker-run SERVER_ID=s1 PORTA=6741 PORTA_HTTP=8741 VIZINHOS=\"http://IP:PORTA\""
	@echo "                           - Sobe container totalmente customizado com qualquer IP"
	@echo ""
	@echo " Docker-Compose (Simulação na mesma máquina):"
	@echo "   make docker-up          - Sobe os 3 servidores interligados localmente"
	@echo "   make docker-down        - Para e remove os containers Docker"
	@echo "   make docker-logs        - Exibe os logs dos 3 containers"
	@echo ""
	@echo " Testes Automatizados:"
	@echo "   make test-teste1        - Teste 1: Trava de 10s e concorrência de 10 passageiros"
	@echo "   make test-teste2pc      - Teste 2PC: Queda do Servidor 3 na 1ª fase (Prepare)"
	@echo "   make test-commit-crash  - Teste 2PC: Queda do Servidor 3 após commit no S1"
	@echo "   make test-wal           - Teste WAL: Servidor 3 cai no commit e recupera via Heartbeat"
	@echo "   make test-lamport       - Teste Concorrência: 2 passageiros disputam vagas com Lamport"
	@echo "   make test-2pc1          - Teste 2PC 1: Reserva e confirmação (commit) em 3 servidores"
	@echo "   make test-2pc2          - Teste 2PC 2: Reserva e cancelamento (abort) em 3 servidores"
	@echo "   make test-2pc3          - Teste 2PC 3: 10 passageiros disputam 5 vagas em 3 servidores"
	@echo "   make test-wal1          - Teste WAL 1: Queda de S3 no Prepare (sem voltar)"
	@echo "   make test-wal2          - Teste WAL 2: Queda de S3 no Prepare (com volta)"
	@echo "   make test-wal3          - Teste WAL 3: Queda de S3 durante Commit (sem voltar)"
	@echo "   make test-wal4          - Teste WAL 4: Queda de S3 durante Commit (com volta)"
	@echo "   make test-remoto [ARGS=\"...\"] - Suíte de testes remotos apontando para IPs externos"
	@echo "   make test-all           - Executa todos os testes automatizados em sequência"
	@echo "======================================================================"

# ------------------------------------------------------------------------------
# Build e Execução Local
# ------------------------------------------------------------------------------
build:
	@echo "==> Compilando Server e Client..."
	go build -o bin/servidor Server/servidor.go
	go build -o bin/cliente Client/main.go
	@echo "==> Binários gerados em bin/"

run-s1:
	@echo "==> Iniciando Server 1..."
	go run Server/servidor.go configs/server1.json

run-s2:
	@echo "==> Iniciando Server 2..."
	go run Server/servidor.go configs/server2.json

run-s3:
	@echo "==> Iniciando Server 3..."
	go run Server/servidor.go configs/server3.json

run-custom:
	@echo "==> Iniciando Servidor customizado ($(SERVER_ID)) com vizinhos: $(VIZINHOS)..."
	SERVER_ID=$(SERVER_ID) PORTA=$(PORTA) PORTA_HTTP=$(PORTA_HTTP) VIZINHOS_HTTP="$(VIZINHOS)" CIDADES="$(CIDADES)" CIDADES_INTER="$(CIDADES_INTER)" go run Server/servidor.go

run-client:
	@echo "==> Iniciando Cliente em $(ADDR)..."
	go run Client/main.go -addr $(ADDR)

# ------------------------------------------------------------------------------
# Comandos Docker
# ------------------------------------------------------------------------------
docker-build:
	@echo "==> Construindo imagem Docker: $(IMAGE_NAME)..."
	docker build -t $(IMAGE_NAME) .

docker-run:
	@echo "==> Subindo container $(SERVER_ID) na porta TCP $(PORTA) e HTTP $(PORTA_HTTP)..."
	docker run -d --name jasefoi-$(SERVER_ID) \
	  -p $(PORTA):$(PORTA) \
	  -p $(PORTA_HTTP):$(PORTA_HTTP) \
	  -v $(PWD)/data:/app/data \
	  -e SERVER_ID=$(SERVER_ID) \
	  -e PORTA=$(PORTA) \
	  -e PORTA_HTTP=$(PORTA_HTTP) \
	  -e VIZINHOS_HTTP="$(VIZINHOS)" \
	  -e CIDADES="$(CIDADES)" \
	  -e CIDADES_INTER="$(CIDADES_INTER)" \
	  $(IMAGE_NAME)

docker-run-s1:
	@echo "==> Iniciando Container Servidor 1 (Salvador) apontando para: $(VIZINHOS)..."
	docker run -d --name jasefoi-s1 \
	  -p 6741:6741 -p 8741:8741 \
	  -v $(PWD)/data/server1:/app/data \
	  -e SERVER_ID=server-1 \
	  -e PORTA=6741 \
	  -e PORTA_HTTP=8741 \
	  -e CIDADES="SALVADOR,FEIRA DE SANTANA,SIMOES FILHO,AMELIA RODRIGUES" \
	  -e CIDADES_INTER="FEIRA DE SANTANA" \
	  -e VIZINHOS_HTTP="$(VIZINHOS)" \
	  $(IMAGE_NAME)

docker-run-s2:
	@echo "==> Iniciando Container Servidor 2 (Feira/Serrinha) apontando para: $(VIZINHOS)..."
	docker run -d --name jasefoi-s2 \
	  -p 6742:6742 -p 8742:8742 \
	  -v $(PWD)/data/server2:/app/data \
	  -e SERVER_ID=server-2 \
	  -e PORTA=6742 \
	  -e PORTA_HTTP=8742 \
	  -e CIDADES="FEIRA DE SANTANA,SERRINHA,IRECE,DOMINGUINHOS" \
	  -e CIDADES_INTER="FEIRA DE SANTANA,SERRINHA" \
	  -e VIZINHOS_HTTP="$(VIZINHOS)" \
	  $(IMAGE_NAME)

docker-run-s3:
	@echo "==> Iniciando Container Servidor 3 (Serrinha/Valença) apontando para: $(VIZINHOS)..."
	docker run -d --name jasefoi-s3 \
	  -p 6743:6743 -p 8743:8743 \
	  -v $(PWD)/data/server3:/app/data \
	  -e SERVER_ID=server-3 \
	  -e PORTA=6743 \
	  -e PORTA_HTTP=8743 \
	  -e CIDADES="SERRINHA,VALENCA,XIQUE XIQUE" \
	  -e CIDADES_INTER="SERRINHA" \
	  -e VIZINHOS_HTTP="$(VIZINHOS)" \
	  $(IMAGE_NAME)

docker-up:
	@echo "==> Subindo cluster de 3 servidores via docker-compose..."
	docker compose up -d --build

docker-down:
	@echo "==> Derrubando cluster Docker..."
	docker compose down

docker-logs:
	docker compose logs -f

# ------------------------------------------------------------------------------
# Testes Automatizados
# ------------------------------------------------------------------------------
test-teste1:
	@echo "==> Executando Teste 1 (Trava 10s / Concorrência de vagas)..."
	go run ./test/teste1/main.go

test-teste2pc:
	@echo "==> Executando Teste 2PC (Queda no Prepare)..."
	go run ./test/teste2pc/main.go

test-commit-crash:
	@echo "==> Executando Teste 2PC (Queda pós-Commit com Rollback)..."
	go run ./test/teste2pc_commit_crash/main.go

test-wal:
	@echo "==> Executando Teste WAL (Queda pós-Commit com Recuperação)..."
	go run ./test/teste_wal/main.go

test-lamport:
	@echo "==> Executando Teste de Concorrência e Lamport..."
	go run ./test/teste_concorrencia_lamport/main.go

test-2pc1:
	@echo "==> Executando Teste 2PC 1 (Reserva e Commit em 3 Servidores)..."
	go run ./test/2pc1/main.go

test-2pc2:
	@echo "==> Executando Teste 2PC 2 (Reserva e Abort em 3 Servidores)..."
	go run ./test/2pc2/main.go

test-2pc3:
	@echo "==> Executando Teste 2PC 3 (10 Passageiros disputam 5 Vagas em 3 Servidores)..."
	go run ./test/2pc3/main.go

test-wal1:
	@echo "==> Executando Teste WAL 1 (Queda S3 no Prepare sem voltar)..."
	go run ./test/wal1/main.go

test-wal2:
	@echo "==> Executando Teste WAL 2 (Queda S3 no Prepare com recuperação)..."
	go run ./test/wal2/main.go

test-wal3:
	@echo "==> Executando Teste WAL 3 (Queda S3 durante Commit com Rollback)..."
	go run ./test/wal3/main.go

test-wal4:
	@echo "==> Executando Teste WAL 4 (Queda S3 durante Commit com recuperação)..."
	go run ./test/wal4/main.go

test-ra:
	@echo "==> Executando Teste de Validação de Ricart-Agrawala (Exclusão Mútua Distribuída)..."
	go run ./test/ra/main.go $(ARGS)

test-remoto:
	@echo "==> Executando Suíte de Testes Remotos..."
	go run ./test/remoto/main.go $(ARGS)

test-all: test-teste1 test-teste2pc test-commit-crash test-wal test-lamport test-2pc1 test-2pc2 test-2pc3 test-wal1 test-wal2 test-wal3 test-wal4
	@echo "==> Todos os testes foram executados com sucesso!"

# ------------------------------------------------------------------------------
# Limpeza
# ------------------------------------------------------------------------------
clean:
	@echo "==> Limpando arquivos compilados..."
	rm -rf bin/ *.exe *.log

