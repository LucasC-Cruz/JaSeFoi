<#
.SYNOPSIS
    Script helper alternativo ao Makefile para uso direto no PowerShell do Windows.
.EXAMPLE
    .\make.ps1 help
    .\make.ps1 docker-build
    .\make.ps1 test-all
#>

[CmdletBinding()]
param(
    [Parameter(Position=0)]
    [string]$Target = "help",

    [Parameter(Position=1)]
    [Alias("Vizinho")]
    [string]$Vizinhos = "",

    [string]$ServerId = "server-1",
    [string]$Porta = "6741",
    [string]$PortaHttp = "8741",

    [Parameter(ValueFromRemainingArguments=$true)]
    [string[]]$Restante
)

# Se o usuário passou argumentos adicionais posicionais (ex: .\make.ps1 docker-run-s1 http://IP:PORTA)
if (-not $Vizinhos -and $Restante -and $Restante.Count -gt 0) {
    $Vizinhos = $Restante[0]
}

function Show-Help {
    Write-Host "======================================================================" -ForegroundColor Cyan
    Write-Host "                   Comandos do make.ps1 (JáSeFoi)                     " -ForegroundColor Cyan
    Write-Host "======================================================================" -ForegroundColor Cyan
    Write-Host " Execução Local / Go:"
    Write-Host "   .\make.ps1 build             - Compila servidor e cliente para bin/"
    Write-Host "   .\make.ps1 run-s1            - Inicia Server 1 (configs/server1.json)"
    Write-Host "   .\make.ps1 run-s2            - Inicia Server 2 (configs/server2.json)"
    Write-Host "   .\make.ps1 run-s3            - Inicia Server 3 (configs/server3.json)"
    Write-Host "   .\make.ps1 run-client        - Inicia cliente interativo"
    Write-Host ""
    Write-Host " Docker com IPs Externos (Computadores Separados):"
    Write-Host "   .\make.ps1 docker-build"
    Write-Host "   .\make.ps1 docker-run-s1 -Vizinhos 'http://IP_PC2:8742'"
    Write-Host "   .\make.ps1 docker-run-s2 -Vizinhos 'http://IP_PC1:8741,http://IP_PC3:8743'"
    Write-Host "   .\make.ps1 docker-run-s3 -Vizinhos 'http://IP_PC2:8742'"
    Write-Host ""
    Write-Host " Docker & Docker-Compose (Mesma Máquina):"
    Write-Host "   .\make.ps1 docker-up         - Sobe os 3 servidores via docker compose"
    Write-Host "   .\make.ps1 docker-down       - Derruba os containers"
    Write-Host "   .\make.ps1 docker-logs       - Acompanha logs dos containers"
    Write-Host ""
    Write-Host " Testes Automatizados:"
    Write-Host "   .\make.ps1 test-wal          - Teste WAL com recuperação via Heartbeat"
    Write-Host "   .\make.ps1 test-lamport      - Teste de concorrência simultânea e relógio Lamport"
    Write-Host "   .\make.ps1 test-2pc1         - Teste 2PC 1: Reserva e confirmação em 3 servidores"
    Write-Host "   .\make.ps1 test-2pc2         - Teste 2PC 2: Reserva e cancelamento em 3 servidores"
    Write-Host "   .\make.ps1 test-2pc3         - Teste 2PC 3: 10 passageiros disputam 5 vagas em 3 servidores"
    Write-Host "   .\make.ps1 test-wal1         - Teste WAL 1: Queda de S3 no Prepare (sem voltar)"
    Write-Host "   .\make.ps1 test-wal2         - Teste WAL 2: Queda de S3 no Prepare (com volta)"
    Write-Host "   .\make.ps1 test-wal3         - Teste WAL 3: Queda de S3 durante Commit (sem voltar)"
    Write-Host "   .\make.ps1 test-wal4         - Teste WAL 4: Queda de S3 durante Commit (com volta)"
    Write-Host "   .\make.ps1 test-remoto       - Suíte de testes remotos (com flags -s1, -s2, -s3, -modo)"
    Write-Host "   .\make.ps1 test-all          - Executa todos os testes em sequência"
    Write-Host "======================================================================" -ForegroundColor Cyan
}

switch ($Target.ToLower()) {
    "build" {
        New-Item -ItemType Directory -Force -Path bin | Out-Null
        Write-Host "==> Compilando Server e Client..." -ForegroundColor Green
        go build -o bin/servidor.exe Server/servidor.go
        go build -o bin/cliente.exe Client/main.go
        Write-Host "==> Concluído em bin/" -ForegroundColor Green
    }
    "run-s1" {
        go run Server/servidor.go configs/server1.json
    }
    "run-s2" {
        go run Server/servidor.go configs/server2.json
    }
    "run-s3" {
        go run Server/servidor.go configs/server3.json
    }
    "run-client" {
        $addr = "localhost:6741"
        if ($Vizinhos) {
            $addr = $Vizinhos
        }
        # Se na máquina local está rodando o server-2 ou server-3, ajusta a porta padrão se não especificada
        Write-Host "==> Conectando cliente em: $addr" -ForegroundColor Green
        go run Client/main.go -addr $addr
    }
    "docker-build" {
        docker build -t jasefoi-server .
    }
    "docker-run-s1" {
        Write-Host "==> Iniciando Server 1 apontando para: $Vizinhos" -ForegroundColor Green
        docker run -d --name jasefoi-s1 `
          -p 6741:6741 -p 8741:8741 `
          -v "${PWD}/data/server1:/app/data" `
          -e SERVER_ID=server-1 `
          -e PORTA=6741 `
          -e PORTA_HTTP=8741 `
          -e CIDADES="SALVADOR,FEIRA DE SANTANA,SIMOES FILHO,AMELIA RODRIGUES" `
          -e CIDADES_INTER="FEIRA DE SANTANA" `
          -e VIZINHOS_HTTP="$Vizinhos" `
          jasefoi-server
    }
    "docker-run-s2" {
        Write-Host "==> Iniciando Server 2 apontando para: $Vizinhos" -ForegroundColor Green
        docker run -d --name jasefoi-s2 `
          -p 6742:6742 -p 8742:8742 `
          -v "${PWD}/data/server2:/app/data" `
          -e SERVER_ID=server-2 `
          -e PORTA=6742 `
          -e PORTA_HTTP=8742 `
          -e CIDADES="FEIRA DE SANTANA,SERRINHA,IRECE,DOMINGUINHOS" `
          -e CIDADES_INTER="FEIRA DE SANTANA,SERRINHA" `
          -e VIZINHOS_HTTP="$Vizinhos" `
          jasefoi-server
    }
    "docker-run-s3" {
        Write-Host "==> Iniciando Server 3 apontando para: $Vizinhos" -ForegroundColor Green
        docker run -d --name jasefoi-s3 `
          -p 6743:6743 -p 8743:8743 `
          -v "${PWD}/data/server3:/app/data" `
          -e SERVER_ID=server-3 `
          -e PORTA=6743 `
          -e PORTA_HTTP=8743 `
          -e CIDADES="SERRINHA,VALENCA,XIQUE XIQUE" `
          -e CIDADES_INTER="SERRINHA" `
          -e VIZINHOS_HTTP="$Vizinhos" `
          jasefoi-server
    }
    "docker-up" {
        docker compose up -d --build
    }
    "docker-down" {
        docker compose down
    }
    "docker-logs" {
        docker compose logs -f
    }
    "test-teste1" {
        go run ./test/teste1/main.go
    }
    "test-teste2pc" {
        go run ./test/teste2pc/main.go
    }
    "test-commit-crash" {
        go run ./test/teste2pc_commit_crash/main.go
    }
    "test-wal" {
        go run ./test/teste_wal/main.go
    }
    "test-lamport" {
        go run ./test/teste_concorrencia_lamport/main.go
    }
    "test-2pc1" {
        go run ./test/2pc1/main.go
    }
    "test-2pc2" {
        go run ./test/2pc2/main.go
    }
    "test-2pc3" {
        go run ./test/2pc3/main.go
    }
    "test-wal1" {
        go run ./test/wal1/main.go
    }
    "test-wal2" {
        go run ./test/wal2/main.go
    }
    "test-wal3" {
        go run ./test/wal3/main.go
    }
    "test-wal4" {
        go run ./test/wal4/main.go
    }
    "test-remoto" {
        go run ./test/remoto/main.go $Restante
    }
    "test-remote" {
        go run ./test/remoto/main.go $Restante
    }
    "test-all" {
        Write-Host "==> Executando Teste 1..." -ForegroundColor Yellow
        go run ./test/teste1/main.go
        Write-Host "==> Executando Teste 2PC (Prepare Crash)..." -ForegroundColor Yellow
        go run ./test/teste2pc/main.go
        Write-Host "==> Executando Teste 2PC (Commit Crash / Rollback)..." -ForegroundColor Yellow
        go run ./test/teste2pc_commit_crash/main.go
        Write-Host "==> Executando Teste WAL (Commit Crash / Recovery)..." -ForegroundColor Yellow
        go run ./test/teste_wal/main.go
        Write-Host "==> Executando Teste Concorrência e Lamport..." -ForegroundColor Yellow
        go run ./test/teste_concorrencia_lamport/main.go
        Write-Host "==> Todos os testes concluídos!" -ForegroundColor Green
    }
    default {
        Show-Help
    }
}

