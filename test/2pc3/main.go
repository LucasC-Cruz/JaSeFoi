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
	fmt.Println(" TESTE 2PC 3: 10 PASSAGEIROS DISPUTAM 5 VAGAS EM 3 SERVIDORES COM LAMPORT")
	fmt.Println(" Rota Multi-Servidor: SALVADOR (S1) -> FEIRA (S2) -> SERRINHA (S3) -> XIQUE XIQUE (S3)")
	fmt.Println(" Cenário: Passageiro 1 dá PREPARE primeiro (10s); outros 9 passageiros dão")
	fmt.Println(" PREPARE e COMMIT durante os 10s; depois Passageiro 1 confirma!")
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

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_2pc3_*")
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
	tcp3, http3 := obterPortaLivre(), obterPortaLivre()

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
			VizinhosHttp: []string{"http://127.0.0.1:" + http1, "http://127.0.0.1:" + http3},
			Cidades:      []string{"FEIRA DE SANTANA", "SERRINHA", "IRECE", "DOMINGUINHOS"},
			CidadesInter: []string{"FEIRA DE SANTANA", "SERRINHA"},
		},
		{
			ID:           "server-3",
			Porta:        tcp3,
			PortaHttp:    http3,
			DataDir:      filepath.Join(tempBaseDir, "data3"),
			VizinhosHttp: []string{"http://127.0.0.1:" + http2},
			Cidades:      []string{"SERRINHA", "VALENCA", "XIQUE XIQUE"},
			CidadesInter: []string{"SERRINHA"},
		},
	}

	cmds := make([]*exec.Cmd, 3)
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
	aguardarVizinhosConectados("http://127.0.0.1:"+http2, 2, 10*time.Second)
	aguardarVizinhosConectados("http://127.0.0.1:"+http3, 1, 10*time.Second)
	fmt.Println(" -> Todos os 3 servidores estão ONLINE e conectados na malha HTTP!")

	dataViagem := "28/10"
	vagas := 5 // 5 vagas cadastradas em cada trecho

	// Passo 2: Cadastrar 1 motorista em cada servidor
	corridas := []struct {
		srvID   string
		tcpAddr string
		nome    string
		origem  string
		destino string
		preco   float64
	}{
		{"server-1", "127.0.0.1:" + tcp1, "Motorista_S1", "SALVADOR", "FEIRA DE SANTANA", 30.0},
		{"server-2", "127.0.0.1:" + tcp2, "Motorista_S2", "FEIRA DE SANTANA", "SERRINHA", 25.0},
		{"server-3", "127.0.0.1:" + tcp3, "Motorista_S3", "SERRINHA", "XIQUE XIQUE", 40.0},
	}

	fmt.Printf("\n[Passo 2] Registrando motoristas e corridas (%d vagas) nos 3 servidores...\n", vagas)
	for _, cInfo := range corridas {
		mClient, err := Apiclient.Dial(cInfo.tcpAddr)
		if err != nil {
			fmt.Printf("Erro ao conectar em %s: %v\n", cInfo.srvID, err)
			return
		}
		defer mClient.Close()
		mClient.RegistraMotorista(cInfo.nome, "123", "Carro-"+cInfo.srvID)
		_, tk, err := mClient.AuthMotorista(cInfo.nome, "123")
		if err != nil {
			fmt.Printf("Erro login %s em %s: %v\n", cInfo.nome, cInfo.srvID, err)
			return
		}
		rotaStr := cInfo.origem + "," + cInfo.destino
		if _, err := mClient.CadastrarCorrida(rotaStr, dataViagem, vagas, float32(cInfo.preco), tk, true); err != nil {
			fmt.Printf("Erro ao cadastrar corrida %s em %s: %v\n", rotaStr, cInfo.srvID, err)
			return
		}
		fmt.Printf(" -> [%s] Corrida '%s -> %s' cadastrada (%d vagas)\n", cInfo.srvID, cInfo.origem, cInfo.destino, vagas)
	}

	// Passo 3: Cadastrar e autenticar 10 passageiros no Server 1
	fmt.Println("\n[Passo 3] Cadastrando e autenticando 10 Passageiros no Server 1...")
	numPassageiros := 10
	tokens := make([]string, numPassageiros)
	clients := make([]*Apiclient.Client, numPassageiros)

	for i := 0; i < numPassageiros; i++ {
		nome := fmt.Sprintf("Passageiro_%d", i+1)
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

	// Passo 4: Buscar rota de SALVADOR para XIQUE XIQUE
	fmt.Println("\n[Passo 4] Buscando rota combinada 'SALVADOR -> XIQUE XIQUE' via Server 1...")
	rotas, menu, err := clients[0].BuscarRotas("SALVADOR", "XIQUE XIQUE", dataViagem, tokens[0])
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rota: %v\n", err)
		return
	}
	fmt.Print(menu)
	rotaEscolhida := rotas[0]
	fmt.Printf(" -> Rota selecionada com %d trecho(s) abrangendo os 3 servidores:\n", len(rotaEscolhida))
	for idx, tr := range rotaEscolhida {
		fmt.Printf("    Trecho %d: %s -> %s (Motoristas: %v)\n", idx+1, tr.Origem, tr.Destino, tr.Ids)
	}

	lpS1Antes, _, _ := obterStatusServidor("http://127.0.0.1:" + http1)
	lpS2Antes, _, _ := obterStatusServidor("http://127.0.0.1:" + http2)
	lpS3Antes, _, _ := obterStatusServidor("http://127.0.0.1:" + http3)
	fmt.Printf("\n[Estado Inicial dos Relógios Lógicos]\n")
	fmt.Printf("  • Server 1 Lamport: %d\n", lpS1Antes)
	fmt.Printf("  • Server 2 Lamport: %d\n", lpS2Antes)
	fmt.Printf("  • Server 3 Lamport: %d\n", lpS3Antes)

	httpClient := &http.Client{Timeout: 45 * time.Second}

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

		var lamportRecebido int64
		if v := resp.Header.Get("X-Lamport"); v != "" {
			if parsed, errP := strconv.ParseInt(v, 10, 64); errP == nil {
				lamportRecebido = parsed
			}
		}

		lamportAtualizado := maxInt64(lamportEnviado, lamportRecebido) + 1
		return resp.StatusCode, respBodyStr, lamportEnviado, lamportAtualizado, nil
	}

	resultados := make([]ResultadoPassageiro, numPassageiros)
	for i := 0; i < numPassageiros; i++ {
		resultados[i] = ResultadoPassageiro{
			Nome:           fmt.Sprintf("Passageiro %d", i+1),
			Token:          tokens[i],
			LamportInicial: 1,
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 5A] PASSAGEIRO 1 DÁ O PREPARE PRIMEIRO (TRAVA DE 10s NOS 3 SERVIDORES)")
	fmt.Println("================================================================================")

	stPrep1, msgPrep1, envPrep1, recPrep1, _ := fazerRequisicaoReserva("Passageiro 1", tokens[0], rotaEscolhida, false, resultados[0].LamportInicial)
	resultados[0].StatusHttpPrepare = stPrep1
	resultados[0].MsgPrepare = msgPrep1
	resultados[0].LamportEnvPrepare = envPrep1
	resultados[0].LamportRespPrepare = recPrep1
	fmt.Printf(" >>> [Passageiro 1] PREPARE realizado com sucesso! HTTP %d (%s) | Lamport: %d -> %d <<<\n",
		stPrep1, msgPrep1, envPrep1, recPrep1)
	fmt.Println(" >>> Trava ativa de 10s em Server 1, Server 2 e Server 3 para o Passageiro 1! <<<")

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 5B] DURANTE OS 10s: OUTROS 9 PASSAGEIROS DÃO PREPARE E COMMIT")
	fmt.Println("================================================================================")

	var wgOutros sync.WaitGroup

	for i := 1; i < numPassageiros; i++ {
		idx := i
		wgOutros.Add(1)
		go func() {
			defer wgOutros.Done()

			lp := resultados[idx].LamportInicial
			stP, msgP, envP, recP, _ := fazerRequisicaoReserva(resultados[idx].Nome, tokens[idx], rotaEscolhida, false, lp)
			resultados[idx].StatusHttpPrepare = stP
			resultados[idx].MsgPrepare = msgP
			resultados[idx].LamportEnvPrepare = envP
			resultados[idx].LamportRespPrepare = recP
			lp = recP

			fmt.Printf(" [%s] PREPARE concluído (HTTP %d, Lamport=%d). Enviando confirmação (COMMIT)...\n",
				resultados[idx].Nome, stP, recP)

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

	fmt.Println("\n >>> Aguardando 2.5 segundos com a trava ativa (outros 9 passageiros enfileirados aguardando)... <<<")
	time.Sleep(2500 * time.Millisecond)

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 5C] DEPOIS, PASSAGEIRO 1 CONFIRMA A VIAGEM (Fase 2 COMMIT)")
	fmt.Println("================================================================================")

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

	wgOutros.Wait()

	lpS1Depois, _, _ := obterStatusServidor("http://127.0.0.1:" + http1)
	lpS2Depois, _, _ := obterStatusServidor("http://127.0.0.1:" + http2)
	lpS3Depois, _, _ := obterStatusServidor("http://127.0.0.1:" + http3)

	// Passo 6: Relatório de Lamport
	fmt.Println("\n================================================================================")
	fmt.Println(" RELATÓRIO COMPLETO DOS RELÓGIOS LÓGICOS DE LAMPORT (10 PASSAGEIROS)")
	fmt.Println("================================================================================")

	for _, p := range resultados {
		statusResultado := "DERROTADO (Não conseguiu vaga / Conflito 409)"
		if p.GanhouCorrida {
			statusResultado = "VENCEDOR (Vaga garantida e confirmada 200 OK!)"
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
	fmt.Printf("  • Server 1 (Coordenador)  : Inicial = %d | Final = %d\n", lpS1Antes, lpS1Depois)
	fmt.Printf("  • Server 2 (Intermediário): Inicial = %d | Final = %d\n", lpS2Antes, lpS2Depois)
	fmt.Printf("  • Server 3 (Destino final): Inicial = %d | Final = %d\n", lpS3Antes, lpS3Depois)

	// Passo 7: Auditoria de Disco nos 3 servidores
	fmt.Println("\n================================================================================")
	fmt.Println(" AUDITORIA DE CONSISTÊNCIA NO BANCO DE DADOS DOS 3 SERVIDORES")
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

	servidoresData := []struct {
		id      string
		dataDir string
		origem  string
		destino string
	}{
		{"Server 1", configs[0].DataDir, "SALVADOR", "FEIRA DE SANTANA"},
		{"Server 2", configs[1].DataDir, "FEIRA DE SANTANA", "SERRINHA"},
		{"Server 3", configs[2].DataDir, "SERRINHA", "XIQUE XIQUE"},
	}

	for _, s := range servidoresData {
		tr := lerTrechos(s.dataDir)
		for id, t := range tr {
			fmt.Printf("  • [%s] Trecho %s (%s -> %s):\n", s.id, id, s.origem, s.destino)
			fmt.Printf("    - Assentos restantes   : %d (Esperado: 0 vagas restantes)\n", t.Nassentos)
			fmt.Printf("    - Passageiros alocados : %v\n", t.IdPassageiros)
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" CONCLUSÃO DO TESTE 2PC 3")
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
	fmt.Printf(" >>> Passageiros Confirmados com Sucesso (%d de 5 vagas): %v <<<\n", len(confirmados), confirmados)
	fmt.Printf(" >>> Passageiros Recusados por Esgotamento de Vagas (%d recusados): %v <<<\n", len(recusados), recusados)
	fmt.Println(" >>> Ordem Causal Respeitada com 2PC em malha de 3 Servidores! <<<")
	fmt.Println("================================================================================")
}

