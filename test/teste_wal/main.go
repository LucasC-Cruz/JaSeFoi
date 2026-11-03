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
	fmt.Println(" TESTE WAL + HEARTBEAT NO 2PC:")
	fmt.Println(" Servidor 3 cai no meio do COMMIT -> Coordenador espera via Heartbeat ->")
	fmt.Println(" Servidor 3 volta -> COMMIT é completado com sucesso via WAL/Retry!")
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

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_wal_test_*")
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
	cfgPaths := make([]string, 3)

	iniciarServidor := func(idx int) *exec.Cmd {
		cfgPath := cfgPaths[idx]
		cmd := exec.Command(serverBin, cfgPath)
		cmd.Dir = serverDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "WAL_COMMIT_RETRY_TIMEOUT_SEC=15")
		if idx == 0 {
			cmd.Env = append(cmd.Env, "TEST_HOOK_AFTER_LOCAL_COMMIT=true")
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		return cmd
	}

	for i, cfg := range configs {
		os.MkdirAll(cfg.DataDir, 0755)
		if len(telasBytes) > 0 {
			os.WriteFile(filepath.Join(cfg.DataDir, "telas.json"), telasBytes, 0644)
		}
		cfgPath := filepath.Join(tempBaseDir, fmt.Sprintf("server%d.json", i+1))
		cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
		os.WriteFile(cfgPath, cfgBytes, 0644)
		cfgPaths[i] = cfgPath

		cmds[i] = iniciarServidor(i)
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
	fmt.Println(" -> Todos os 3 servidores estão ONLINE e conectados!")

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

	fmt.Println("\n[Passo 2] Cadastrando e logando 1 motorista em cada servidor e registrando corridas...")
	for _, info := range corridasPorServidor {
		motClient, err := Apiclient.Dial(info.tcpAddr)
		if err != nil {
			fmt.Printf("Erro ao conectar em %s: %v\n", info.srvID, err)
			return
		}
		defer motClient.Close()

		motClient.RegistraMotorista(info.nomeMot, "123", "Carro-"+info.srvID)
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
	}

	fmt.Println("\n[Passo 3] Cadastrando e logando passageiro no server-1...")
	passClient, err := Apiclient.Dial("127.0.0.1:" + tcp1)
	if err != nil {
		fmt.Printf("Erro ao conectar passageiro no server-1: %v\n", err)
		return
	}
	defer passClient.Close()

	passClient.RegistraPassageiro("Passageiro_WAL", "123")
	_, passTk, err := passClient.AuthPassageiro("Passageiro_WAL", "123")
	if err != nil {
		fmt.Printf("Erro ao autenticar passageiro: %v\n", err)
		return
	}

	fmt.Println("\n[Passo 4] Buscando rota SALVADOR -> VALENCA...")
	rotas, _, err := passClient.BuscarRotas("SALVADOR", "VALENCA", dataViagem, passTk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rota: %v\n", err)
		return
	}
	rotaEscolhida := rotas[0]

	fmt.Println("\n[Passo 5] Executando PREPARE nos 3 servidores...")
	msgPrepare, errPrepare := passClient.TentaReserva(rotaEscolhida, false, passTk)
	if errPrepare != nil {
		fmt.Printf("Erro no PREPARE: %v\n", errPrepare)
		return
	}
	fmt.Printf(" -> PREPARE OK: %s\n", strings.TrimSpace(msgPrepare))

	// Inicia a confirmação do cliente em goroutine
	fmt.Println("\n[Passo 6] Enviando confirmação (COMMIT) pelo cliente...")
	var wg sync.WaitGroup
	var respCommitStr string
	var errCommit error

	wg.Add(1)
	go func() {
		defer wg.Done()
		respCommitStr, errCommit = passClient.TentaReserva(rotaEscolhida, true, passTk)
	}()

	// Aguarda o Server 1 comitar localmente
	time.Sleep(300 * time.Millisecond)

	fmt.Println("\n >>> [QUEDA TEMPORÁRIA DO SERVIDOR 3] Matando Server 3 durante a fase de commit! <<<")
	if cmds[2] != nil && cmds[2].Process != nil {
		_ = cmds[2].Process.Kill()
		_, _ = cmds[2].Process.Wait()
	}
	fmt.Println(" >>> Server 3 caiu! O Server 2/Server 1 detectam e aguardam via Heartbeat... <<<")

	// Deixa o servidor 3 offline por 3 segundos (enquanto Server 2 aguarda e tenta reconectar com timeout de 15s)
	time.Sleep(3 * time.Second)

	fmt.Println("\n >>> [RECUPERAÇÃO DO SERVIDOR 3] Reiniciando Server 3! <<<")
	cmds[2] = iniciarServidor(2)

	fmt.Println(" >>> Aguardando Server 3 voltar e o COMMIT ser finalizado... <<<")
	wg.Wait()

	fmt.Printf("\n -> Resposta final recebida pelo passageiro: msg=%q | err=%v\n",
		strings.TrimSpace(respCommitStr), errCommit)

	// =========================================================================
	// PASSO 7: VERIFICAÇÃO DE CONSISTÊNCIA
	// =========================================================================
	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 7] Verificando consistência: Todos os 3 servidores devem ter comitado!")
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
	trechosS3 := lerTrechos(configs[2].DataDir)

	contarVagas := func(m map[string]Model.Trecho) (int, int) {
		vagas := -1
		ocup := 0
		for _, t := range m {
			vagas = t.Nassentos
			for _, p := range t.IdPassageiros {
				if p != "" {
					ocup++
				}
			}
		}
		return vagas, ocup
	}

	v1, o1 := contarVagas(trechosS1)
	v2, o2 := contarVagas(trechosS2)
	v3, o3 := contarVagas(trechosS3)

	fmt.Printf("Server 1 (SALVADOR -> FEIRA DE SANTANA) : Nassentos=%d (Esperado: 3) | Ocupados=%d (Esperado: 1)\n", v1, o1)
	fmt.Printf("Server 2 (FEIRA DE SANTANA -> SERRINHA) : Nassentos=%d (Esperado: 3) | Ocupados=%d (Esperado: 1)\n", v2, o2)
	fmt.Printf("Server 3 (SERRINHA -> VALENCA)          : Nassentos=%d (Esperado: 3) | Ocupados=%d (Esperado: 1)\n", v3, o3)
	fmt.Println("--------------------------------------------------------------------------------")

	// Verifica também o arquivo WAL gerado
	walData1, _ := os.ReadFile(filepath.Join(configs[0].DataDir, "wal.jsonl"))
	fmt.Printf("Log WAL gerado em server-1:\n%s\n", strings.TrimSpace(string(walData1)))

	if errCommit == nil && v1 == 3 && o1 == 1 && v2 == 3 && o2 == 1 && v3 == 3 && o3 == 1 {
		fmt.Println(">>> SUCESSO ABSOLUTO DO WAL + HEARTBEAT! O Servidor 3 caiu, o sistema esperou")
		fmt.Println("    ele voltar através de checagem/retry com Heartbeat, concluiu o COMMIT")
		fmt.Println("    e todos os 3 servidores efetivaram a corrida sem perda de dados! <<<")
	} else {
		fmt.Println(">>> FALHA NO TESTE WAL! <<<")
		os.Exit(1)
	}
}
