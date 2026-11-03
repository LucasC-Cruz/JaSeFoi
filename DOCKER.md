# Como Executar o JáSeFoi com Docker em Computadores Diferentes

Este guia explica como rodar cada servidor em uma máquina diferente na rede local ou externa, passando os IPs correspondentes para a comunicação distribuída (TCP, HTTP, 2PC, Ricart-Agrawala, WAL e Heartbeat).

---

## 1. Gerar a Imagem Docker

Em qualquer máquina onde você clonou o projeto, gere a imagem com o comando:

```bash
docker build -t jasefoi-server .
```

> **Dica:** Você pode exportar a imagem para um pendrive/transferir via `docker save -o jasefoi-server.tar jasefoi-server` e importar no outro computador com `docker load -i jasefoi-server.tar`, ou compilar localmente com `docker build -t jasefoi-server .` em cada um dos computadores.

---

## 2. Executar em Máquinas Diferentes via `docker run`

Suponha o seguinte cenário com 3 computadores na mesma rede:
- **Computador 1**: IP `192.168.1.101` (Servidor 1 - Salvador e Feira)
- **Computador 2**: IP `192.168.1.102` (Servidor 2 - Feira e Serrinha)
- **Computador 3**: IP `192.168.1.103` (Servidor 3 - Serrinha e Valença)

### No Computador 1 (`192.168.1.101`):
```bash
docker run -d \
  --name jasefoi-s1 \
  -p 6741:6741 \
  -p 8741:8741 \
  -v $(pwd)/data:/app/data \
  -e SERVER_ID=server-1 \
  -e PORTA=6741 \
  -e PORTA_HTTP=8741 \
  -e CIDADES="SALVADOR,FEIRA DE SANTANA,SIMOES FILHO,AMELIA RODRIGUES" \
  -e CIDADES_INTER="FEIRA DE SANTANA" \
  -e VIZINHOS_HTTP="http://192.168.1.102:8742" \
  jasefoi-server
```

---

### No Computador 2 (`192.168.1.102`):
```bash
docker run -d \
  --name jasefoi-s2 \
  -p 6742:6742 \
  -p 8742:8742 \
  -v $(pwd)/data:/app/data \
  -e SERVER_ID=server-2 \
  -e PORTA=6742 \
  -e PORTA_HTTP=8742 \
  -e CIDADES="FEIRA DE SANTANA,SERRINHA,IRECE,DOMINGUINHOS" \
  -e CIDADES_INTER="FEIRA DE SANTANA,SERRINHA" \
  -e VIZINHOS_HTTP="http://192.168.1.101:8741,http://192.168.1.103:8743" \
  jasefoi-server
```

---

### No Computador 3 (`192.168.1.103`):
```bash
docker run -d \
  --name jasefoi-s3 \
  -p 6743:6743 \
  -p 8743:8743 \
  -v $(pwd)/data:/app/data \
  -e SERVER_ID=server-3 \
  -e PORTA=6743 \
  -e PORTA_HTTP=8743 \
  -e CIDADES="SERRINHA,VALENCA,XIQUE XIQUE" \
  -e CIDADES_INTER="SERRINHA" \
  -e VIZINHOS_HTTP="http://192.168.1.102:8742" \
  jasefoi-server
```

---

## 3. Ou Executar Usando Arquivo JSON de Configuração

Se você preferir passar um arquivo `.json` (ex: `configs/server1.json`), basta colocar o IP do vizinho no campo `vizinhosHttp` do arquivo json e rodar:

```bash
docker run -d \
  --name jasefoi-s1 \
  -p 6741:6741 \
  -p 8741:8741 \
  -v $(pwd)/configs:/app/configs \
  -v $(pwd)/data:/app/data \
  -e CONFIG_FILE=/app/configs/server1.json \
  jasefoi-server
```

---

## 4. Testar na Mesma Máquina (Opcional)

Se quiser simular o ambiente multi-container em uma única máquina para testes:

```bash
docker-compose up --build
```

Os 3 containers subirão conectados numa mesma rede virtual Docker, já configurados com seus vizinhos e portas mapeadas.

