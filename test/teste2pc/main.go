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
	fmt.Println(" SIMULAÇÃO 2PC MULTI-SERVIDOR: QUEDA DO SERVIDOR 3 NA 1ª FASE DO 2PC")
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

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_sim2pc_*")
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

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 5A] Iniciando 1ª Fase do 2PC (PREPARE / Trava de 10s nos 3 servidores)")
	fmt.Println("================================================================================")
	msgPrepare, errPrepare := passClient.TentaReserva(rotaEscolhida, false, passTk)
	if errPrepare != nil {
		fmt.Printf("Erro inesperado no PREPARE inicial: %v (%s)\n", errPrepare, msgPrepare)
		return
	}
	fmt.Printf(" -> 1ª Fase do 2PC (PREPARE) ativa! Resposta do servidor: %s\n", strings.TrimSpace(msgPrepare))

	fmt.Println("\n >>> [QUEDA DO SERVIDOR 3] Derrubando o server-3 ENQUANTO a reserva está na 1ª Fase do 2PC! <<<")
	if cmds[2] != nil && cmds[2].Process != nil {
		_ = cmds[2].Process.Kill()
		_, _ = cmds[2].Process.Wait()
	}
	fmt.Println(" >>> server-3 foi encerrado abruptamente! <<<")

	fmt.Println("\n[Passo 5B] Passageiro tenta confirmar a reserva (2ª Fase COMMIT) após a queda do server-3...")
	msgCommit, errCommit := passClient.TentaReserva(rotaEscolhida, true, passTk)
	fmt.Printf(" -> Resultado da confirmação pelo cliente: msg=%q | err=%v\n", strings.TrimSpace(msgCommit), errCommit)

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 6] Tentando iniciar nova 1ª Fase do 2PC (PREPARE) com server-3 caído")
	fmt.Println("================================================================================")
	msgPrepareComS3Caido, errPrepareComS3Caido := passClient.TentaReserva(rotaEscolhida, false, passTk)
	fmt.Printf(" -> Resultado do PREPARE com server-3 offline: msg=%q | err=%v\n",
		strings.TrimSpace(msgPrepareComS3Caido), errPrepareComS3Caido)

	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 7] Verificando consistência dos dados em server-1 e server-2 após ABORT")
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

	fmt.Printf("Estado final no server-1 (SALVADOR -> FEIRA DE SANTANA) : Nassentos=%d (Esperado: %d) | Passageiros alocados=%d (Esperado: 0)\n",
		vagasS1, vagasIniciais, ocupadosS1)
	fmt.Printf("Estado final no server-2 (FEIRA DE SANTANA -> SERRINHA) : Nassentos=%d (Esperado: %d) | Passageiros alocados=%d (Esperado: 0)\n",
		vagasS2, vagasIniciais, ocupadosS2)
	fmt.Printf("Passagens ativas no registro do Passageiro_S1           : %d (Esperado: 0)\n", trechosDoPassageiro)
	fmt.Println("--------------------------------------------------------------------------------")

	if errCommit != nil && errPrepareComS3Caido != nil && vagasS1 == vagasIniciais && ocupadosS1 == 0 && vagasS2 == vagasIniciais && ocupadosS2 == 0 && trechosDoPassageiro == 0 {
		fmt.Println(">>> CONCLUSÃO DA SIMULAÇÃO: O protocolo 2PC detectou a queda do server-3, abortou a transação")
		fmt.Println("    e desfez todas as travas/reservas em server-1 e server-2 mantendo 100% da atomicidade! <<<")
	} else {
		fmt.Println(">>> FALHA: Inconsistência detectada após queda do server-3! <<<")
		os.Exit(1)
	}
}

