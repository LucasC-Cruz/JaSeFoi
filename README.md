# JáSeFoi - Sistema Distribuído de Caronas (2PC, Ricart-Agrawala, WAL e Docker)

Sistema distribuído em Go para simulação de reserva e cancelamento de caronas intermunicipais através de múltiplos servidores autônomos, utilizando:
- **Two-Phase Commit (2PC)** em cascata para transações distribuídas atômicas.
- **Ricart-Agrawala** com relógio lógico de **Lamport** para exclusão mútua na alocação de trechos.
- **Write-Ahead Log (WAL)** e checagem ativa por **Heartbeat** para recuperação e tolerância a falhas.
- **Suporte a Docker** para execução em nós/computadores distintos na rede local ou externa.

---

## Manual de Usuário do Makefile

O projeto disponibiliza duas formas equivalentes de executar os comandos:
1. **Linux, macOS, WSL ou Git Bash no Windows**: usando o utilitário nativo `make` com o arquivo `Makefile`.
2. **PowerShell nativo do Windows**: usando o script `./make.ps1` (sem precisar instalar nenhum pacote adicional).

---

### Tabela Rápida de Comandos Equivalentes

| Ação | Linux / WSL / Git Bash (`make`) | Windows PowerShell (`.\make.ps1`) |
| :--- | :--- | :--- |
| **Exibir Ajuda** | `make help` | `.\make.ps1 help` |
| **Compilar Binários** | `make build` | `.\make.ps1 build` |
| **Iniciar Server 1** | `make run-s1` | `.\make.ps1 run-s1` |
| **Iniciar Server 2** | `make run-s2` | `.\make.ps1 run-s2` |
| **Iniciar Server 3** | `make run-s3` | `.\make.ps1 run-s3` |
| **Iniciar Cliente** | `make run-client` | `.\make.ps1 run-client` |
| **Gerar Imagem Docker** | `make docker-build` | `.\make.ps1 docker-build` |
| **Subir Cluster Docker** | `make docker-up` | `.\make.ps1 docker-up` |
| **Parar Cluster Docker** | `make docker-down` | `.\make.ps1 docker-down` |
| **Ver Logs Docker** | `make docker-logs` | `.\make.ps1 docker-logs` |
| **Executar Todos os Testes**| `make test-all` | `.\make.ps1 test-all` |

---

## 1. Execução no Linux / macOS / WSL / Git Bash

Abra o terminal na pasta raiz do projeto.

### 1.1. Compilação Local
Compila o servidor e o cliente gerando os binários na pasta `bin/`:
```bash
make build
```

### 1.2. Execução Local dos Servidores e Cliente
Para rodar os 3 servidores na mesma máquina em abas separadas do terminal:
```bash
# Terminal 1:
make run-s1

# Terminal 2:
make run-s2

# Terminal 3:
make run-s3
```

Para abrir o cliente interativo conectado em um servidor local ou remoto:
```bash
# Conectar no localhost padrão (porta 6741):
make run-client

# Conectar em outro computador pelo IP e Porta TCP:
make run-client ADDR="192.168.1.102:6742"
```

---

### 1.3. Executando em Computadores Diferentes com Docker (Passando IPs)

Quando for executar cada servidor em um computador diferente, passe o IP dos outros servidores na variável `VIZINHOS`:

#### Suponha o seguinte cenário de rede:
- **PC 1**: `192.168.1.101` (Servidor 1: Salvador -> Feira)
- **PC 2**: `192.168.1.102` (Servidor 2: Feira -> Serrinha)
- **PC 3**: `192.168.1.103` (Servidor 3: Serrinha -> Valença)

#### No Computador 1 (PC 1):
```bash
make docker-build
make docker-run-s1 VIZINHOS="http://192.168.1.102:8742"
```

#### No Computador 2 (PC 2):
```bash
make docker-build
make docker-run-s2 VIZINHOS="http://192.168.1.101:8741,http://192.168.1.103:8743"
```

#### No Computador 3 (PC 3):
```bash
make docker-build
make docker-run-s3 VIZINHOS="http://192.168.1.102:8742"
```

---

## 2. Execução no Windows (via PowerShell)

No Windows, abra o **PowerShell** na pasta raiz do projeto.

> *Nota:* Se o PowerShell bloquear a execução de scripts por padrão, libere apenas para a sessão atual com:
> ```powershell
> Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
> ```

### 2.1. Compilação Local
```powershell
.\make.ps1 build
```

### 2.2. Execução Local dos Servidores
Abra três janelas do PowerShell:
```powershell
# Janela 1:
.\make.ps1 run-s1

# Janela 2:
.\make.ps1 run-s2

# Janela 3:
.\make.ps1 run-s3
```

Para abrir o cliente:
```powershell
# Conectar no localhost padrão (porta 6741):
.\make.ps1 run-client

# Conectar em outro computador pelo IP e Porta TCP:
.\make.ps1 run-client -Vizinhos "192.168.1.102:6742"
# ou direto com Go:
go run Client/main.go -addr 192.168.1.102:6742
```

---

### 2.3. Executando em Computadores Diferentes com Docker no Windows

#### No Computador 1 (`192.168.1.101`):
```powershell
.\make.ps1 docker-build
.\make.ps1 docker-run-s1 -Vizinhos "http://192.168.1.102:8742"
```

#### No Computador 2 (`192.168.1.102`):
```powershell
.\make.ps1 docker-build
.\make.ps1 docker-run-s2 -Vizinhos "http://192.168.1.101:8741,http://192.168.1.103:8743"
```

#### No Computador 3 (`192.168.1.103`):
```powershell
.\make.ps1 docker-build
.\make.ps1 docker-run-s3 -Vizinhos "http://192.168.1.102:8742"
```

---

## 3. Testes Automatizados

O sistema conta com testes automatizados ponta-a-ponta que sobem clusters temporários isolados para validar os algoritmos distribuídos:

### Executar individualmente:
- **Teste 1 (Trava exclusiva de 10s e concorrência de 10 passageiros simultâneos)**:
  - Linux/WSL: `make test-teste1`
  - Windows: `.\make.ps1 test-teste1`
- **Teste 2PC (Queda do Servidor 3 na 1ª fase de Prepare com Abort global)**:
  - Linux/WSL: `make test-teste2pc`
  - Windows: `.\make.ps1 test-teste2pc`
- **Teste 2PC Commit Crash (Queda do Servidor 3 pós-commit no S1 com Rollback total)**:
  - Linux/WSL: `make test-commit-crash`
  - Windows: `.\make.ps1 test-commit-crash`
- **Teste WAL + Heartbeat (Queda pós-commit com recuperação e conclusão da transação)**:
  - Linux/WSL: `make test-wal`
  - Windows: `.\make.ps1 test-wal`

### Executar todos os testes de uma vez:
- Linux/WSL: `make test-all`
- Windows: `.\make.ps1 test-all`
