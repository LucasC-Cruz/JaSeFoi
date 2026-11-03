package main

import (
	"Golang/Apiclient"
	"Golang/Model"
	"encoding/json"
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
	fmt.Println("==================================================")
	fmt.Println("   INICIANDO TESTE DE CONCORRÊNCIA (1000 REQS)   ")
	fmt.Println("==================================================")

	port := "6742"
	serverAddr := "127.0.0.1:" + port

	// 1. Iniciar o servidor em background
	rootDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Erro ao pegar dir: %v\n", err)
		return
	}
	serverDir := filepath.Join(rootDir, "Server")

	cmd := exec.Command("go", "run", "servidor.go", port)
	cmd.Dir = serverDir
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
	for i := 0; i < 30; i++ {
		conn, err := net.DialTimeout("tcp", serverAddr, 300*time.Millisecond)
		if err == nil {
			conn.Close()
			fmt.Println(" Conectado!")
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 2. Conectar cliente motorista e cadastrar corrida
	motoristaClient, err := Apiclient.Dial(serverAddr)
	if err != nil {
		fmt.Printf("Erro ao conectar motorista: %v\n", err)
		return
	}
	defer motoristaClient.Close()

	motNome := fmt.Sprintf("MotConcorr_%d", time.Now().UnixNano()%100000)
	motSenha := "123"
	motCarro := "Fusca Concorrente"

	_, err = motoristaClient.RegistraMotorista(motNome, motSenha, motCarro)
	if err != nil {
		fmt.Printf("Erro ao cadastrar motorista: %v\n", err)
		return
	}

	_, motTk, err := motoristaClient.AuthMotorista(motNome, motSenha)
	if err != nil {
		fmt.Printf("Erro ao logar motorista: %v\n", err)
		return
	}

	numAssentos := 100
	numTentativas := 1000
	origem := "CONCORRENCIA_A"
	destino := "CONCORRENCIA_B"
	dataViagem := "15/11"

	fmt.Printf("\n[1] Cadastrando corrida: %s -> %s | Assentos: %d | Data: %s\n", origem, destino, numAssentos, dataViagem)
	_, err = motoristaClient.CadastrarCorrida(origem+","+destino, dataViagem, numAssentos, 20.0, motTk, true)
	if err != nil {
		fmt.Printf("Erro ao cadastrar corrida: %v\n", err)
		return
	}
	fmt.Println("-> Corrida cadastrada com sucesso!")

	// 3. Buscar a rota para ter a Aresta exata
	rotas, _, err := motoristaClient.BuscarRotas(origem, destino, dataViagem, motTk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rota cadastrada: %v\n", err)
		return
	}
	rotaAlvo := rotas[0]
	fmt.Printf("-> Rota encontrada: %s -> %s (Motoristas na aresta: %v)\n", rotaAlvo[0].Origem, rotaAlvo[0].Destino, rotaAlvo[0].Ids)

	// 4. Pré-registrar e conectar os passageiros
	fmt.Printf("\n[2] Preparando e autenticando %d passageiros...\n", numTentativas)
	type PassSessao struct {
		client *Apiclient.Client
		tk     string
	}
	sessoes := make([]PassSessao, numTentativas)

	var wgSetup sync.WaitGroup
	setupSem := make(chan struct{}, 50) // Limita concorrência na criação para 50 conexões simultâneas

	startTimeSetup := time.Now()
	for i := 0; i < numTentativas; i++ {
		wgSetup.Add(1)
		go func(idx int) {
			defer wgSetup.Done()
			setupSem <- struct{}{}
			defer func() { <-setupSem }()

			c, err := Apiclient.Dial(serverAddr)
			if err != nil {
				return
			}
			pNome := fmt.Sprintf("Pass_%d_%d", time.Now().UnixNano()%100000, idx)
			pSenha := "123"

			c.RegistraPassageiro(pNome, pSenha)
			_, pTk, err := c.AuthPassageiro(pNome, pSenha)
			if err == nil && pTk != "" {
				sessoes[idx] = PassSessao{client: c, tk: pTk}
			}
		}(i)
	}
	wgSetup.Wait()
	fmt.Printf("-> %d passageiros preparados em %v!\n", numTentativas, time.Since(startTimeSetup))

	// 5. TESTE DE CONCORRÊNCIA MASSIVA
	fmt.Printf("\n[3] Disparando %d tentativas de reserva concorrentes simultaneamente...\n", numTentativas)

	var sucessos int64
	var falhas int64
	var wgCorrida sync.WaitGroup
	startBarrier := make(chan struct{})

	for i := 0; i < numTentativas; i++ {
		if sessoes[i].client == nil {
			continue
		}
		wgCorrida.Add(1)
		go func(s PassSessao) {
			defer wgCorrida.Done()
			<-startBarrier // Aguarda o tiro de largada

			_, err := s.client.TentaReserva(rotaAlvo, true, s.tk)
			if err == nil {
				atomic.AddInt64(&sucessos, 1)
			} else {
				atomic.AddInt64(&falhas, 1)
			}
		}(sessoes[i])
	}

	startBench := time.Now()
	close(startBarrier) // TIRO DE LARGADA
	wgCorrida.Wait()
	duracao := time.Since(startBench)

	// Fechar clientes
	for _, s := range sessoes {
		if s.client != nil {
			s.client.Close()
		}
	}

	// 6. RELATÓRIO DO TESTE
	fmt.Println("\n==================================================")
	fmt.Println("              RESULTADO DO TESTE                  ")
	fmt.Println("==================================================")
	fmt.Printf("Total de requisições concorrentes : %d\n", numTentativas)
	fmt.Printf("Capacidade de assentos da corrida : %d\n", numAssentos)
	fmt.Printf("Reservas bem-sucedidas (200 OK)   : %d\n", sucessos)
	fmt.Printf("Reservas recusadas (Sem vaga/Erro): %d\n", falhas)
	fmt.Printf("Tempo total de processamento      : %v\n", duracao)
	fmt.Printf("Throughput aproximado             : %.2f req/s\n", float64(numTentativas)/duracao.Seconds())

	// 7. Auditoria dos dados no servidor (arquivo trechos.json)
	trechosBytes, err := os.ReadFile("data/trechos.json")
	if err == nil {
		var mapaT map[string]Model.Trecho
		json.Unmarshal(trechosBytes, &mapaT)

		for _, p := range rotas[0] {
			for _, mid := range p.Ids {
				// Procura o trecho
				for _, t := range mapaT {
					if t.Data == dataViagem {
						fmt.Println("\n--- Auditoria do Trecho no Disco ---")
						fmt.Printf("ID do Trecho: %s\n", t.Id)
						fmt.Printf("Nassentos final: %d (Esperado: 0)\n", t.Nassentos)
						
						ocupados := 0
						for _, idPass := range t.IdPassageiros {
							if idPass != "" {
								ocupados++
							}
						}
						fmt.Printf("Total de assentos preenchidos: %d (Esperado: %d)\n", ocupados, numAssentos)
						_ = mid
						break
					}
				}
			}
		}
	}

	if sucessos == int64(numAssentos) {
		fmt.Println("\n>>> TESTE PASSOU COM SUCESSO ABSOLUTO! Sem race conditions, sem overbooking. <<<")
	} else {
		fmt.Printf("\n>>> ATENÇÃO: Sucessos (%d) diferente da capacidade (%d)! <<<\n", sucessos, numAssentos)
	}
	fmt.Println("==================================================")
}
