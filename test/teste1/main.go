package main

import (
	"Golang/Apiclient"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	fmt.Println("==================================================================")
	fmt.Println(" TESTE 1: 5 VAGAS | 1 PASSAGEIRO TRAVA + 10 PEDEM E CONFIRMAM")
	fmt.Println(" Cenário: Passageiro 1 confirma dentro dos 10s.")
	fmt.Println(" Esperado: Passageiro 1 + 4 dos outros 10 são atendidos (Total=5)")
	fmt.Println("           Os outros 6 têm as vagas negadas!")
	fmt.Println("==================================================================")

	port := "6745"
	serverAddr := "127.0.0.1:" + port

	rootDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Erro ao obter diretório atual: %v\n", err)
		return
	}
	// Permite rodar tanto da raiz do projeto quanto de dentro de test/ ou test/teste1/
	if _, err := os.Stat(filepath.Join(rootDir, "Server")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(rootDir, "..", "Server")); err == nil {
			rootDir = filepath.Join(rootDir, "..")
		} else if _, err := os.Stat(filepath.Join(rootDir, "..", "..", "Server")); err == nil {
			rootDir = filepath.Join(rootDir, "..", "..")
		}
	}
	serverDir := filepath.Join(rootDir, "Server")

	// Cria diretório temporário de dados isolado para o teste
	tempDataDir, err := os.MkdirTemp("", "jasefoi_teste1_*")
	if err != nil {
		fmt.Printf("Erro ao criar diretório temporário: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDataDir)

	// Copia telas.json para o diretório temporário
	telasBytes, err := os.ReadFile(filepath.Join(rootDir, "data", "telas.json"))
	if err == nil {
		os.WriteFile(filepath.Join(tempDataDir, "telas.json"), telasBytes, 0644)
	}

	// Compila o binário do servidor no diretório temporário para encerramento limpo ao final do teste
	serverBin := filepath.Join(tempDataDir, "servidor.exe")
	buildCmd := exec.Command("go", "build", "-o", serverBin, "servidor.go")
	buildCmd.Dir = serverDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Printf("Erro ao compilar servidor: %v\n%s\n", err, string(out))
		return
	}

	cmd := exec.Command(serverBin, port)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(),
		"SERVER_ID=server-teste-1",
		"PORTA="+port,
		"PORTA_HTTP=8745",
		"DATA_DIR="+tempDataDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("Erro ao iniciar servidor: %v\n", err)
		return
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Aguarda o servidor subir
	fmt.Print("Aguardando servidor iniciar...")
	conectou := false
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", serverAddr, 300*time.Millisecond)
		if err == nil {
			conn.Close()
			fmt.Println(" Conectado!")
			conectou = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !conectou {
		fmt.Println("\nFalha ao aguardar servidor iniciar.")
		return
	}

	// 1. Motorista se cadastra, faz login e registra uma corrida com 5 vagas
	motClient, err := Apiclient.Dial(serverAddr)
	if err != nil {
		fmt.Printf("Erro ao conectar motorista: %v\n", err)
		return
	}
	defer motClient.Close()

	_, err = motClient.RegistraMotorista("MotoristaT1", "123", "Van 5 Lugares")
	if err != nil {
		fmt.Printf("Erro ao registrar motorista: %v\n", err)
		return
	}
	_, motTk, err := motClient.AuthMotorista("MotoristaT1", "123")
	if err != nil {
		fmt.Printf("Erro no login do motorista: %v\n", err)
		return
	}

	vagasTotais := 5
	origem := "SALVADOR"
	destino := "FEIRA DE SANTANA"
	data := "25/10"

	fmt.Printf("\n[Passo 1] Motorista cadastrando corrida %s -> %s com %d vagas...\n", origem, destino, vagasTotais)
	_, err = motClient.CadastrarCorrida(origem+","+destino, data, vagasTotais, 35.0, motTk, true)
	if err != nil {
		fmt.Printf("Erro ao cadastrar corrida: %v\n", err)
		return
	}

	// 2. Preparar 1 Passageiro Principal + 10 Outros Passageiros
	type SessaoPassageiro struct {
		nome   string
		client *Apiclient.Client
		tk     string
	}

	criarPassageiro := func(nome string) SessaoPassageiro {
		c, err := Apiclient.Dial(serverAddr)
		if err != nil {
			panic(err)
		}
		c.RegistraPassageiro(nome, "123")
		_, tk, err := c.AuthPassageiro(nome, "123")
		if err != nil {
			panic(err)
		}
		return SessaoPassageiro{nome: nome, client: c, tk: tk}
	}

	passPrincipal := criarPassageiro("Passageiro_Principal")
	defer passPrincipal.client.Close()

	outrosPassageiros := make([]SessaoPassageiro, 10)
	for i := 0; i < 10; i++ {
		outrosPassageiros[i] = criarPassageiro(fmt.Sprintf("Passageiro_Concorrente_%02d", i+1))
		defer outrosPassageiros[i].client.Close()
	}

	// 3. Passageiro Principal busca a rota e pede a reserva (TRAVA por 10s, sem confirmar ainda)
	rotas, _, err := passPrincipal.client.BuscarRotas(origem, destino, data, passPrincipal.tk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rotas: %v\n", err)
		return
	}
	rotaEscolhida := rotas[0]

	fmt.Printf("\n[Passo 2] %s escolheu a rota e travou 1 vaga (tem 10s para confirmar)...\n", passPrincipal.nome)
	msg, err := passPrincipal.client.TentaReserva(rotaEscolhida, false, passPrincipal.tk)
	if err != nil {
		fmt.Printf("Erro ao travar rota para %s: %v (%s)\n", passPrincipal.nome, err, msg)
		return
	}

	// 4. Dentro dos 10 segundos, os outros 10 passageiros pedem a mesma rota e confirmam!
	fmt.Println("\n[Passo 3] Dentro dos 10s, outros 10 passageiros pedem a mesma rota e confirmam...")
	var atendidosDos10 int32
	var negadosDos10 int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(p SessaoPassageiro) {
			defer wg.Done()
			// Passo A: escolhe a rota (pede trava)
			_, errTrava := p.client.TentaReserva(rotaEscolhida, false, p.tk)
			if errTrava != nil {
				atomic.AddInt32(&negadosDos10, 1)
				fmt.Printf(" -> [%s] Vaga NEGADA já na solicitação inicial.\n", p.nome)
				return
			}
			// Passo B: confirma imediatamente dentro dos 10s
			_, errConf := p.client.TentaReserva(rotaEscolhida, true, p.tk)
			if errConf == nil {
				atomic.AddInt32(&atendidosDos10, 1)
				fmt.Printf(" -> [%s] Vaga ATENDIDA (Confirmada com sucesso)!\n", p.nome)
			} else {
				atomic.AddInt32(&negadosDos10, 1)
				fmt.Printf(" -> [%s] Vaga NEGADA após confirmação das vagas restantes!\n", p.nome)
			}
		}(outrosPassageiros[i])
	}

	// Aguarda 2 segundos (ainda dentro da janela de 10s) para que todos os 10 façam o pedido e confirmem
	time.Sleep(2 * time.Second)

	atendidosAntesDoPrincipal := atomic.LoadInt32(&atendidosDos10)
	fmt.Printf("\n[Verificação de Trava] Quantos passaram na frente do %s durante os 10s? %d (Esperado: 0)\n", passPrincipal.nome, atendidosAntesDoPrincipal)

	// 5. Passageiro Principal confirma sua reserva dentro dos 10 segundos!
	fmt.Printf("\n[Passo 4] (Após 2s, dentro dos 10s) %s envia CONFIRMAÇÃO (Sim)...\n", passPrincipal.nome)
	_, errPrincipal := passPrincipal.client.TentaReserva(rotaEscolhida, true, passPrincipal.tk)
	principalAtendido := (errPrincipal == nil)
	if principalAtendido {
		fmt.Printf(" -> [%s] Vaga ATENDIDA (Confirmou dentro dos 10s)!\n", passPrincipal.nome)
	} else {
		fmt.Printf(" -> [%s] ERRO: Falhou ao confirmar dentro dos 10s: %v\n", passPrincipal.nome, errPrincipal)
	}

	// Aguarda todas as goroutines dos 10 passageiros finalizarem suas respostas
	wg.Wait()

	fmt.Println("\n==================================================================")
	fmt.Println("                       RESULTADO DO TESTE 1                       ")
	fmt.Println("==================================================================")
	fmt.Printf("Vagas totais na corrida                 : %d\n", vagasTotais)
	fmt.Printf("Passaram na frente durante os 10s?      : %d (Esperado: 0)\n", atendidosAntesDoPrincipal)
	fmt.Printf("Passageiro Principal atendido?          : %v (Esperado: true)\n", principalAtendido)
	fmt.Printf("Outros 10 passageiros - Atendidos       : %d (Esperado: 4)\n", atendidosDos10)
	fmt.Printf("Outros 10 passageiros - Vagas Negadas   : %d (Esperado: 6)\n", negadosDos10)
	fmt.Println("------------------------------------------------------------------")

	if atendidosAntesDoPrincipal == 0 && principalAtendido && atendidosDos10 == 4 && negadosDos10 == 6 {
		fmt.Println(">>> SUCESSO! Nenhum passageiro passou na frente durante os 10s, o Passageiro Principal + 4 foram atendidos (5 vagas) e os outros 6 tiveram as vagas negadas! <<<")
	} else {
		fmt.Println(">>> FALHA: O resultado não correspondeu ao esperado! <<<")
		os.Exit(1)
	}
}

