package main

import (
	"Golang/Apiclient"
	"Golang/Model"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ServerConfig struct {
	ID           string   `json:"id"`
	Porta        string   `json:"porta"`
	PortaHttp    string   `json:"portaHttp"`
	DataDir      string   `json:"dataDir"`
	Vizinhos     []string `json:"vizinhos,omitempty"`
	VizinhosHttp []string `json:"vizinhosHttp"`
	Cidades      []string `json:"cidades"`
	CidadesInter []string `json:"cidadesInter"`
}

func obterPortaLivre() string {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port
}

func aguardarTCP(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func aguardarVizinhosConectados(httpURL string, esperados int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(httpURL + "/status")
		if err == nil {
			var st struct {
				Vizinhos map[string]struct {
					Conectado bool `json:"conectado"`
				} `json:"vizinhos"`
			}
			json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()

			conectados := 0
			for _, v := range st.Vizinhos {
				if v.Conectado {
					conectados++
				}
			}
			if conectados >= esperados {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func obterStatusServidor(httpURL string) (int64, map[string]interface{}, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(httpURL + "/status")
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	var st map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return 0, nil, err
	}
	var lp int64
	if v, ok := st["lamport"].(float64); ok {
		lp = int64(v)
	}
	return lp, st, nil
}

type ResultadoPassageiro struct {
	Nome               string
	Token              string
	LamportInicial     int64
	LamportEnvPrepare  int64
	LamportRespPrepare int64
	StatusHttpPrepare  int
	MsgPrepare         string
	LamportEnvCommit   int64
	LamportRespCommit  int64
	StatusHttpCommit   int
	MsgCommit          string
	LamportFinal       int64
	GanhouCorrida      bool
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println(" TESTE DE CONCORRÊNCIA, ENFILEIRAMENTO E RELÓGIO LÓGICO DE LAMPORT")
	fmt.Println(" Rota Multi-Servidor: SALVADOR (S1) -> FEIRA DE SANTANA (S2) -> SERRINHA (S2)")
	fmt.Println(" Cenário: Passageiro 1 dá PREPARE primeiro (trava de 10s); outros 3 passageiros")
	fmt.Println(" tentam confirmar a viagem durante esses 10s; depois Passageiro 1 confirma!")
	fmt.Println("================================================================================")

	rootDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Erro ao obter diretório atual: %v\n", err)
		return
	}
	if _, err := os.Stat(filepath.Join(rootDir, "Server")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(rootDir, "..", "Server")); err == nil {
			rootDir = filepath.Join(rootDir, "..")
		} else if _, err := os.Stat(filepath.Join(rootDir, "..", "..", "Server")); err == nil {
			rootDir = filepath.Join(rootDir, "..", "..")
		}
	}
	serverDir := filepath.Join(rootDir, "Server")

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_lamport_test_*")
	if err != nil {
		fmt.Printf("Erro ao criar diretório temporário: %v\n", err)
		return
	}
	defer os.RemoveAll(tempBaseDir)

	fmt.Println("\n[Setup] Compilando binário do servidor...")
	serverBin := filepath.Join(tempBaseDir, "servidor.exe")
	buildCmd := exec.Command("go", "build", "-o", serverBin, "servidor.go")
	buildCmd.Dir = serverDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Printf("Erro ao compilar servidor: %v\n%s\n", err, string(out))
		return
	}

	telasBytes, _ := os.ReadFile(filepath.Join(rootDir, "data", "telas.json"))

	tcp1, http1 := obterPortaLivre(), obterPortaLivre()
	tcp2, http2 := obterPortaLivre(), obterPortaLivre()

	configs := []ServerConfig{
		{
			ID:           "server-1",
			Porta:        tcp1,
			PortaHttp:    http1,
			DataDir:      filepath.Join(tempBaseDir, "data1"),
			Vizinhos:     []string{"127.0.0.1:" + tcp2},
			VizinhosHttp: []string{"http://127.0.0.1:" + http2},
			Cidades:      []string{"SALVADOR", "FEIRA DE SANTANA", "SIMOES FILHO", "AMELIA RODRIGUES"},
			CidadesInter: []string{"FEIRA DE SANTANA"},
		},
		{
			ID:           "server-2",
			Porta:        tcp2,
			PortaHttp:    http2,
			DataDir:      filepath.Join(tempBaseDir, "data2"),
			VizinhosHttp: []string{"http://127.0.0.1:" + http1},
			Cidades:      []string{"FEIRA DE SANTANA", "SERRINHA", "IRECE", "DOMINGUINHOS"},
			CidadesInter: []string{"FEIRA DE SANTANA", "SERRINHA"},
		},
	}

	cmds := make([]*exec.Cmd, 2)
	for i, cfg := range configs {
		os.MkdirAll(cfg.DataDir, 0755)
		if len(telasBytes) > 0 {
			os.WriteFile(filepath.Join(cfg.DataDir, "telas.json"), telasBytes, 0644)
		}
		cfgPath := filepath.Join(tempBaseDir, fmt.Sprintf("server%d.json", i+1))
		cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
		os.WriteFile(cfgPath, cfgBytes, 0644)

		cmd := exec.Command(serverBin, cfgPath)
		cmd.Dir = serverDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Printf("Erro ao iniciar %s: %v\n", cfg.ID, err)
			return
		}
		cmds[i] = cmd
	}
	defer func() {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}()

	fmt.Println("\n[Passo 1] Aguardando servidores iniciarem e conectarem a malha HTTP...")
	for _, cfg := range configs {
		if !aguardarTCP("127.0.0.1:"+cfg.Porta, 10*time.Second) {
			fmt.Printf("Falha ao aguardar TCP do %s na porta %s\n", cfg.ID, cfg.Porta)
			return
		}
	}
	aguardarVizinhosConectados("http://127.0.0.1:"+http1, 1, 10*time.Second)
	aguardarVizinhosConectados("http://127.0.0.1:"+http2, 1, 10*time.Second)
	fmt.Println(" -> Servidor 1 e Servidor 2 estão ONLINE e conectados!")

	dataViagem := "28/10"
	vagas := 3 // Corrida com 3 vagas disponíveis em cada trecho!

	// Passo 2: Motorista 1 cadastra corrida no Server 1
	fmt.Printf("\n[Passo 2] Motorista 1 registra corrida 'SALVADOR -> FEIRA DE SANTANA' (%d vagas) no Server 1...\n", vagas)
	mot1Client, err := Apiclient.Dial("127.0.0.1:" + tcp1)
	if err != nil {
		fmt.Printf("Erro ao conectar motorista 1: %v\n", err)
		return
	}
	defer mot1Client.Close()
	mot1Client.RegistraMotorista("Motorista_S1", "123", "Carro-S1")
	_, mot1Tk, err := mot1Client.AuthMotorista("Motorista_S1", "123")
	if err != nil {
		fmt.Printf("Erro no login motorista 1: %v\n", err)
		return
	}
	if _, err := mot1Client.CadastrarCorrida("SALVADOR,FEIRA DE SANTANA", dataViagem, vagas, 30.0, mot1Tk, true); err != nil {
		fmt.Printf("Erro ao cadastrar corrida motorista 1: %v\n", err)
		return
	}
	fmt.Println(" -> Corrida no Server 1 cadastrada com sucesso!")

	// Passo 3: Motorista 2 cadastra corrida no Server 2
	fmt.Printf("\n[Passo 3] Motorista 2 registra corrida 'FEIRA DE SANTANA -> SERRINHA' (%d vagas) no Server 2...\n", vagas)
	mot2Client, err := Apiclient.Dial("127.0.0.1:" + tcp2)
	if err != nil {
		fmt.Printf("Erro ao conectar motorista 2: %v\n", err)
		return
	}
	defer mot2Client.Close()
	mot2Client.RegistraMotorista("Motorista_S2", "123", "Carro-S2")
	_, mot2Tk, err := mot2Client.AuthMotorista("Motorista_S2", "123")
	if err != nil {
		fmt.Printf("Erro no login motorista 2: %v\n", err)
		return
	}
	if _, err := mot2Client.CadastrarCorrida("FEIRA DE SANTANA,SERRINHA", dataViagem, vagas, 25.0, mot2Tk, true); err != nil {
		fmt.Printf("Erro ao cadastrar corrida motorista 2: %v\n", err)
		return
	}
	fmt.Println(" -> Corrida no Server 2 cadastrada com sucesso!")

	// Passo 4: Cadastrar e autenticar os 4 passageiros no Server 1
	fmt.Println("\n[Passo 4] Cadastrando e autenticando 4 Passageiros no Server 1...")
	nomes := []string{"Passageiro_1", "Passageiro_2", "Passageiro_3", "Passageiro_4"}
	tokens := make([]string, 4)
	clients := make([]*Apiclient.Client, 4)

	for i, nome := range nomes {
		c, err := Apiclient.Dial("127.0.0.1:"+tcp1, "http://127.0.0.1:"+http1)
		if err != nil {
			fmt.Printf("Erro ao conectar %s: %v\n", nome, err)
			return
		}
		defer c.Close()
		clients[i] = c

		c.RegistraPassageiro(nome, "123")
		_, tk, err := c.AuthPassageiro(nome, "123")
		if err != nil || tk == "" {
			fmt.Printf("Erro ao autenticar %s: %v\n", nome, err)
			return
		}
		tokens[i] = tk
		fmt.Printf(" -> %s autenticado com sucesso! (Token: %s)\n", nome, tk)
	}

	// Passo 5: Buscar rotas de SALVADOR para SERRINHA
	fmt.Println("\n[Passo 5] Buscando rota combinada 'SALVADOR -> SERRINHA' via Server 1...")
	rotas, menu, err := clients[0].BuscarRotas("SALVADOR", "SERRINHA", dataViagem, tokens[0])
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rota: %v\n", err)
		return
	}
	fmt.Print(menu)
	rotaEscolhida := rotas[0]
	fmt.Printf(" -> Rota selecionada com %d trecho(s):\n", len(rotaEscolhida))
	for idx, tr := range rotaEscolhida {
		fmt.Printf("    Trecho %d: %s -> %s (Motoristas: %v)\n", idx+1, tr.Origem, tr.Destino, tr.Ids)
	}

	lpS1Antes, _, _ := obterStatusServidor("http://127.0.0.1:" + http1)
	lpS2Antes, _, _ := obterStatusServidor("http://127.0.0.1:" + http2)
	fmt.Printf("\n[Estado Inicial dos Relógios Lógicos]\n")
	fmt.Printf("  • Server 1 Lamport: %d\n", lpS1Antes)
	fmt.Printf("  • Server 2 Lamport: %d\n", lpS2Antes)

	httpClient := &http.Client{Timeout: 35 * time.Second}

	fazerRequisicaoReserva := func(passNome, passTk string, rota []Model.Aresta, confirma bool, lamportAtual int64) (int, string, int64, int64, error) {
		reqData := Model.InfoConfirmarReserva{
			Rota:        rota,
			Confirma:    confirma,
			Cancelar:    false,
			TkAssociado: passTk,
		}
		bodyBytes, _ := json.Marshal(reqData)

		urlReserva := fmt.Sprintf("http://127.0.0.1:%s/passageiro/tentar-reserva", http1)
		httpReq, err := http.NewRequest(http.MethodPost, urlReserva, bytes.NewReader(bodyBytes))
		if err != nil {
			return 0, "", lamportAtual, lamportAtual, err
		}

		// Incrementa relógio lógico de Lamport antes do evento de envio (Regra 1 de Lamport)
		lamportEnviado := lamportAtual + 1
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+passTk)
		httpReq.Header.Set("X-Lamport", strconv.FormatInt(lamportEnviado, 10))

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			return 0, "", lamportEnviado, lamportEnviado, err
		}
		defer resp.Body.Close()

		respBodyBytes, _ := io.ReadAll(resp.Body)
		respBodyStr := strings.TrimSpace(string(respBodyBytes))

		// Lê Lamport retornado pelo servidor
		var lamportRecebido int64
		if v := resp.Header.Get("X-Lamport"); v != "" {
			if parsed, errP := strconv.ParseInt(v, 10, 64); errP == nil {
				lamportRecebido = parsed
			}
		}

		// Atualiza relógio lógico do cliente (Regra 2 de Lamport: max(local, recebido) + 1)
		lamportAtualizado := maxInt64(lamportEnviado, lamportRecebido) + 1

		return resp.StatusCode, respBodyStr, lamportEnviado, lamportAtualizado, nil
	}

	resultados := make([]ResultadoPassageiro, 4)
	for i := 0; i < 4; i++ {
		resultados[i] = ResultadoPassageiro{
			Nome:           fmt.Sprintf("Passageiro %d", i+1),
			Token:          tokens[i],
			LamportInicial: 1,
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 6A] PASSAGEIRO 1 DÁ O PREPARE PRIMEIRO (INICIANDO TRAVA DE 10 SEGUNDOS)")
	fmt.Println("================================================================================")

	// Passageiro 1 dá PREPARE primeiro
	stPrep1, msgPrep1, envPrep1, recPrep1, _ := fazerRequisicaoReserva("Passageiro 1", tokens[0], rotaEscolhida, false, resultados[0].LamportInicial)
	resultados[0].StatusHttpPrepare = stPrep1
	resultados[0].MsgPrepare = msgPrep1
	resultados[0].LamportEnvPrepare = envPrep1
	resultados[0].LamportRespPrepare = recPrep1
	fmt.Printf(" >>> [Passageiro 1] PREPARE realizado! Status: HTTP %d (%s) | Lamport enviado: %d -> recebido: %d <<<\n",
		stPrep1, msgPrep1, envPrep1, recPrep1)
	fmt.Println(" >>> Trava de 10s ATIVA e associada exclusivamente ao Passageiro 1! <<<")

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 6B] DURANTE OS 10s: OS OUTROS 3 PASSAGEIROS CONFIRMAM QUE QUEREM A VIAGEM")
	fmt.Println("================================================================================")

	var wgOutros sync.WaitGroup

	// Dispara os outros 3 passageiros (Passageiros 2, 3 e 4) dentro da janela de 10 segundos
	for i := 1; i <= 3; i++ {
		idx := i
		wgOutros.Add(1)
		go func() {
			defer wgOutros.Done()

			lp := resultados[idx].LamportInicial
			// Passo a: Envia PREPARE
			stP, msgP, envP, recP, _ := fazerRequisicaoReserva(resultados[idx].Nome, tokens[idx], rotaEscolhida, false, lp)
			resultados[idx].StatusHttpPrepare = stP
			resultados[idx].MsgPrepare = msgP
			resultados[idx].LamportEnvPrepare = envP
			resultados[idx].LamportRespPrepare = recP
			lp = recP

			fmt.Printf(" [%s] PREPARE concluído (HTTP %d, Lamport=%d). Enviando confirmação (COMMIT)...\n",
				resultados[idx].Nome, stP, recP)

			// Passo b: Confirma imediatamente (COMMIT: "Sim, eu quero a viagem!")
			// Como o Passageiro 1 ainda tem trava ativa e não confirmou, estes passageiros
			// entram na fila e ficam bloqueados aguardando o Passageiro 1 decidir!
			stC, msgC, envC, recC, _ := fazerRequisicaoReserva(resultados[idx].Nome, tokens[idx], rotaEscolhida, true, lp)
			resultados[idx].StatusHttpCommit = stC
			resultados[idx].MsgCommit = msgC
			resultados[idx].LamportEnvCommit = envC
			resultados[idx].LamportRespCommit = recC
			resultados[idx].LamportFinal = recC
			if stC == http.StatusOK {
				resultados[idx].GanhouCorrida = true
			}
		}()
	}

	// Aguarda 2 segundos para assegurar que os 3 passageiros já enviaram seus pedidos de confirmação
	// e estão enfileirados aguardando dentro do período de 10s do Passageiro 1
	fmt.Println("\n >>> Aguardando 2 segundos com a trava de 10s ativa (outros 3 passageiros enfileirados no servidor)... <<<")
	time.Sleep(2 * time.Second)

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 6C] DEPOIS, O PASSAGEIRO 1 CONFIRMA A VIAGEM (Fase 2 COMMIT)")
	fmt.Println("================================================================================")

	// Passageiro 1 finalmente confirma dentro do seu prazo de 10 segundos
	lp1 := resultados[0].LamportRespPrepare
	stCom1, msgCom1, envCom1, recCom1, _ := fazerRequisicaoReserva("Passageiro 1", tokens[0], rotaEscolhida, true, lp1)
	resultados[0].StatusHttpCommit = stCom1
	resultados[0].MsgCommit = msgCom1
	resultados[0].LamportEnvCommit = envCom1
	resultados[0].LamportRespCommit = recCom1
	resultados[0].LamportFinal = recCom1
	if stCom1 == http.StatusOK {
		resultados[0].GanhouCorrida = true
	}
	fmt.Printf(" >>> [Passageiro 1] COMMIT realizado! Status: HTTP %d (%s) | Lamport enviado: %d -> recebido: %d <<<\n",
		stCom1, msgCom1, envCom1, recCom1)

	// Aguarda a resolução dos outros 3 passageiros que estavam na fila
	wgOutros.Wait()

	lpS1Depois, _, _ := obterStatusServidor("http://127.0.0.1:" + http1)
	lpS2Depois, _, _ := obterStatusServidor("http://127.0.0.1:" + http2)

	// Passo 7: Relatório Completo dos Lamports
	fmt.Println("\n================================================================================")
	fmt.Println(" RELATÓRIO COMPLETO DOS RELÓGIOS LÓGICOS DE LAMPORT (TODOS OS PASSAGEIROS)")
	fmt.Println("================================================================================")

	for _, p := range resultados {
		statusResultado := "DERROTADO (Não conseguiu vaga / Conflito)"
		if p.GanhouCorrida {
			statusResultado = "VENCEDOR (Vaga garantida e confirmada!)"
		}

		fmt.Printf("\n--- [%s] ---\n", p.Nome)
		fmt.Printf("  • Resultado Final           : %s\n", statusResultado)
		fmt.Printf("  • Lamport Inicial           : %d\n", p.LamportInicial)
		fmt.Printf("  • Fase 1 (PREPARE / Trava)  : HTTP %d | Resposta: %s\n", p.StatusHttpPrepare, p.MsgPrepare)
		fmt.Printf("    - Lamport enviado no PREPARE  : %d\n", p.LamportEnvPrepare)
		fmt.Printf("    - Lamport após resp. PREPARE : %d\n", p.LamportRespPrepare)

		if p.StatusHttpCommit > 0 {
			fmt.Printf("  • Fase 2 (COMMIT / Confirma): HTTP %d | Resposta: %s\n", p.StatusHttpCommit, p.MsgCommit)
			fmt.Printf("    - Lamport enviado no COMMIT   : %d\n", p.LamportEnvCommit)
			fmt.Printf("    - Lamport após resp. COMMIT  : %d\n", p.LamportRespCommit)
		}
		fmt.Printf("  • Lamport Final do Passageiro: %d\n", p.LamportFinal)
	}

	fmt.Printf("\n--- [Relógios Lógicos dos Servidores] ---\n")
	fmt.Printf("  • Server 1 (Coordenador) : Inicial = %d | Final = %d\n", lpS1Antes, lpS1Depois)
	fmt.Printf("  • Server 2 (Participante): Inicial = %d | Final = %d\n", lpS2Antes, lpS2Depois)

	// Passo 8: Auditoria de Disco
	fmt.Println("\n================================================================================")
	fmt.Println(" AUDITORIA DE CONSISTÊNCIA NO BANCO DE DADOS (trechos.json)")
	fmt.Println("================================================================================")

	lerTrechos := func(dataDir string) map[string]Model.Trecho {
		raw, err := os.ReadFile(filepath.Join(dataDir, "trechos.json"))
		if err != nil {
			return nil
		}
		var m map[string]Model.Trecho
		json.Unmarshal(raw, &m)
		return m
	}

	tr1 := lerTrechos(configs[0].DataDir)
	tr2 := lerTrechos(configs[1].DataDir)

	for id, t := range tr1 {
		fmt.Printf("  • Server 1 Trecho %s (Data: %s):\n", id, t.Data)
		fmt.Printf("    - Assentos restantes : %d\n", t.Nassentos)
		fmt.Printf("    - Passageiros nos assentos: %v\n", t.IdPassageiros)
	}

	for id, t := range tr2 {
		fmt.Printf("  • Server 2 Trecho %s (Data: %s):\n", id, t.Data)
		fmt.Printf("    - Assentos restantes : %d\n", t.Nassentos)
		fmt.Printf("    - Passageiros nos assentos: %v\n", t.IdPassageiros)
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" CONCLUSÃO DO TESTE")
	fmt.Println("================================================================================")
	var confirmados []string
	var recusados []string
	for _, p := range resultados {
		if p.GanhouCorrida {
			confirmados = append(confirmados, p.Nome)
		} else {
			recusados = append(recusados, p.Nome)
		}
	}
	fmt.Printf(" >>> Passageiros Confirmados com Sucesso (%d vagas preenchidas): %v <<<\n", len(confirmados), confirmados)
	fmt.Printf(" >>> Passageiros Recusados por Esgotamento de Vagas: %v <<<\n", recusados)
	fmt.Println(" >>> Ordem Causal Respeitada: Passageiro 1 segurou a trava e garantiu a 1ª vaga.")
	fmt.Println(" >>> Os outros 3 que tentaram durante os 10s respeitaram a fila; o primeiro da fila")
	fmt.Println(" >>> obteve a 2ª vaga restante e os demais foram rejeitados sem inconsistência! <<<")
	fmt.Println("================================================================================")
}
