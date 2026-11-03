package main

import (
	"Golang/Apiclient"
	"Golang/Model"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ServerConfig struct {
	ID           string   `json:"id"`
	Porta        string   `json:"porta"`
	PortaHttp    string   `json:"portaHttp"`
	DataDir      string   `json:"dataDir"`
	Vizinhos     []string `json:"vizinhos"`
	VizinhosHttp []string `json:"vizinhosHttp"`
	Cidades      []string `json:"cidades"`
	CidadesInter []string `json:"cidadesInter"`
}

type Req2PC struct {
	TxID                string         `json:"txId"`
	Passageiro          string         `json:"passageiro"`
	PassageirosAfetados []string       `json:"passageirosAfetados,omitempty"`
	Rota                []Model.Aresta `json:"rota,omitempty"`
	Trechos             []string       `json:"trechos,omitempty"`
	ServidoresVisitados []string       `json:"servidoresVisitados"`
}

type Resp2PC struct {
	Sucesso             bool           `json:"sucesso"`
	Status              int            `json:"status"`
	ArestasRestantes    []Model.Aresta `json:"arestasRestantes,omitempty"`
	TrechosRestantes    []string       `json:"trechosRestantes,omitempty"`
	TrechosReservados   []string       `json:"trechosReservados,omitempty"`
	ServidoresVisitados []string       `json:"servidoresVisitados"`
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

func main() {
	fmt.Println("================================================================================")
	fmt.Println(" SIMULAÇÃO 2PC: PREPARE NOS 3 SERVIDORES E QUEDA DO SERVIDOR 3 APÓS COMMIT NO S1")
	fmt.Println(" Rota: SALVADOR (S1) -> FEIRA DE SANTANA (S2) -> SERRINHA (S3) -> VALENCA")
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

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_sim2pc_commit_*")
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
		cmd.Env = os.Environ()
		if i == 0 {
			cmd.Env = append(cmd.Env, "TEST_HOOK_AFTER_LOCAL_COMMIT=true")
		}
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

	fmt.Println("\n[Passo 1] Aguardando os 3 servidores iniciarem e conectarem a malha HTTP...")
	for _, cfg := range configs {
		if !aguardarTCP("127.0.0.1:"+cfg.Porta, 10*time.Second) {
			fmt.Printf("Falha ao aguardar TCP do %s na porta %s\n", cfg.ID, cfg.Porta)
			return
		}
	}
	aguardarVizinhosConectados("http://127.0.0.1:"+http1, 1, 10*time.Second)
	aguardarVizinhosConectados("http://127.0.0.1:"+http2, 2, 10*time.Second)
	aguardarVizinhosConectados("http://127.0.0.1:"+http3, 1, 10*time.Second)
	fmt.Println(" -> Todos os 3 servidores estão ONLINE e conectados (server-1 <-> server-2 <-> server-3)!")

	dataViagem := "28/10"
	vagasIniciais := 4

	corridasPorServidor := []struct {
		srvID   string
		tcpAddr string
		nomeMot string
		origem  string
		destino string
	}{
		{"server-1", "127.0.0.1:" + tcp1, "Motorista_S1", "SALVADOR", "FEIRA DE SANTANA"},
		{"server-2", "127.0.0.1:" + tcp2, "Motorista_S2", "FEIRA DE SANTANA", "SERRINHA"},
		{"server-3", "127.0.0.1:" + tcp3, "Motorista_S3", "SERRINHA", "VALENCA"},
	}

	fmt.Println("\n[Passo 2] Cadastrando e logando 1 motorista em cada servidor e registrando suas corridas...")
	for _, info := range corridasPorServidor {
		motClient, err := Apiclient.Dial(info.tcpAddr)
		if err != nil {
			fmt.Printf("Erro ao conectar em %s: %v\n", info.srvID, err)
			return
		}
		defer motClient.Close()

		if _, err := motClient.RegistraMotorista(info.nomeMot, "123", "Carro-"+info.srvID); err != nil {
			fmt.Printf("Erro ao registrar %s em %s: %v\n", info.nomeMot, info.srvID, err)
			return
		}
		_, motTk, err := motClient.AuthMotorista(info.nomeMot, "123")
		if err != nil {
			fmt.Printf("Erro no login de %s em %s: %v\n", info.nomeMot, info.srvID, err)
			return
		}
		rotaStr := info.origem + "," + info.destino
		if _, err := motClient.CadastrarCorrida(rotaStr, dataViagem, vagasIniciais, 30.0, motTk, true); err != nil {
			fmt.Printf("Erro ao cadastrar corrida %s em %s: %v\n", rotaStr, info.srvID, err)
			return
		}
		fmt.Printf(" -> [%s] Motorista '%s' logado e corrida '%s -> %s' cadastrada (%d vagas)\n",
			info.srvID, info.nomeMot, info.origem, info.destino, vagasIniciais)
	}

	fmt.Println("\n[Passo 3] Cadastrando e logando passageiro no server-1...")
	passClient, err := Apiclient.Dial("127.0.0.1:" + tcp1)
	if err != nil {
		fmt.Printf("Erro ao conectar passageiro no server-1: %v\n", err)
		return
	}
	defer passClient.Close()

	if _, err := passClient.RegistraPassageiro("Passageiro_S1", "123"); err != nil {
		fmt.Printf("Erro ao registrar passageiro: %v\n", err)
		return
	}
	_, passTk, err := passClient.AuthPassageiro("Passageiro_S1", "123")
	if err != nil {
		fmt.Printf("Erro ao autenticar passageiro: %v\n", err)
		return
	}
	fmt.Println(" -> Passageiro 'Passageiro_S1' registrado e autenticado no server-1!")

	fmt.Println("\n[Passo 4] Passageiro buscando rota de SALVADOR para VALENCA no server-1...")
	rotas, menuRotas, err := passClient.BuscarRotas("SALVADOR", "VALENCA", dataViagem, passTk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rota SALVADOR -> VALENCA: %v\n", err)
		return
	}
	fmt.Print(menuRotas)
	rotaEscolhida := rotas[0]

	// =========================================================================
	// PASSO 5: PREPARE nos 3 servidores (Fase 1 do 2PC)
	// =========================================================================
	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 5] Executando PREPARE nos 3 servidores (Fase 1 do 2PC)")
	fmt.Println("================================================================================")
	msgPrepare, errPrepare := passClient.TentaReserva(rotaEscolhida, false, passTk)
	if errPrepare != nil {
		fmt.Printf("Erro inesperado no PREPARE: %v (%s)\n", errPrepare, msgPrepare)
		return
	}
	fmt.Printf(" -> PREPARE concluído com sucesso nos 3 servidores! Resposta: %s\n", strings.TrimSpace(msgPrepare))

	// =========================================================================
	// PASSO 6: O passageiro envia a confirmação (COMMIT). O Server 1 comita
	// localmente primeiro, e logo em seguida derrubamos o Server 3!
	// =========================================================================
	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 6] Enviando COMMIT do passageiro e derrubando Server 3 durante a propagação")
	fmt.Println("================================================================================")

	var wg sync.WaitGroup
	var respCommitStr string
	var errCommit error

	// Goroutine que dispara a confirmação (COMMIT) pelo cliente
	wg.Add(1)
	go func() {
		defer wg.Done()
		respCommitStr, errCommit = passClient.TentaReserva(rotaEscolhida, true, passTk)
	}()

	// Aguarda 300ms: tempo suficiente para o Server 1 executar prepararCommitLocal
	// e entrar na janela antes de propagar para os vizinhos
	time.Sleep(300 * time.Millisecond)

	fmt.Println("\n >>> [QUEDA DO SERVIDOR 3] Derrubando Server 3 após o commit no Server 1! <<<")
	if cmds[2] != nil && cmds[2].Process != nil {
		_ = cmds[2].Process.Kill()
		_, _ = cmds[2].Process.Wait()
	}
	fmt.Println(" >>> Server 3 foi finalizado (kill)! <<<")

	wg.Wait()
	fmt.Printf(" -> Resposta recebida pelo cliente: msg=%q | err=%v\n", strings.TrimSpace(respCommitStr), errCommit)

	// Aguarda propagação do ABORT e reverterCommitLocal
	time.Sleep(1 * time.Second)

	// =========================================================================
	// PASSO 7: VERIFICAÇÃO DE CONSISTÊNCIA E ATOMICIDADE (REVERSÃO / ROLLBACK)
	// =========================================================================
	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 7] Verificando se Server 1 e Server 2 reverteram o COMMIT (ROLLBACK)")
	fmt.Println("================================================================================")

	lerTrechos := func(dataDir string) map[string]Model.Trecho {
		raw, err := os.ReadFile(filepath.Join(dataDir, "trechos.json"))
		if err != nil {
			return nil
		}
		var m map[string]Model.Trecho
		_ = json.Unmarshal(raw, &m)
		return m
	}

	trechosS1 := lerTrechos(configs[0].DataDir)
	trechosS2 := lerTrechos(configs[1].DataDir)

	vagasS1 := -1
	ocupadosS1 := 0
	for _, t := range trechosS1 {
		vagasS1 = t.Nassentos
		for _, p := range t.IdPassageiros {
			if p != "" {
				ocupadosS1++
			}
		}
	}

	vagasS2 := -1
	ocupadosS2 := 0
	for _, t := range trechosS2 {
		vagasS2 = t.Nassentos
		for _, p := range t.IdPassageiros {
			if p != "" {
				ocupadosS2++
			}
		}
	}

	rawPassS1, _ := os.ReadFile(filepath.Join(configs[0].DataDir, "Passageiro.json"))
	var mapPassS1 map[string]Model.Passageiro
	_ = json.Unmarshal(rawPassS1, &mapPassS1)
	trechosDoPassageiro := 0
	for _, p := range mapPassS1 {
		trechosDoPassageiro += len(p.IsTrechosPassageiros)
	}

	fmt.Printf("Estado no server-1 (SALVADOR -> FEIRA DE SANTANA) : Nassentos=%d (Esperado: %d) | Passageiros alocados=%d (Esperado: 0)\n",
		vagasS1, vagasIniciais, ocupadosS1)
	fmt.Printf("Estado no server-2 (FEIRA DE SANTANA -> SERRINHA) : Nassentos=%d (Esperado: %d) | Passageiros alocados=%d (Esperado: 0)\n",
		vagasS2, vagasIniciais, ocupadosS2)
	fmt.Printf("Passagens ativas no registro do Passageiro_S1     : %d (Esperado: 0)\n", trechosDoPassageiro)
	fmt.Println("--------------------------------------------------------------------------------")

	if errCommit != nil && vagasS1 == vagasIniciais && ocupadosS1 == 0 && vagasS2 == vagasIniciais && ocupadosS2 == 0 && trechosDoPassageiro == 0 {
		fmt.Println(">>> SUCESSO ABSOLUTO! Mesmo após o Server 1 ter dado COMMIT local, a queda do Server 3")
		fmt.Println("    ativou a Fase 2 (ABORT/ROLLBACK) do 2PC: o Server 1 reverteu seu commit local,")
		fmt.Println("    o Server 2 desfez qualquer reserva parcial e a atomicidade foi 100% preservada! <<<")
	} else {
		fmt.Println(">>> FALHA: Inconsistência detectada nos servidores! <<<")
		os.Exit(1)
	}
}
