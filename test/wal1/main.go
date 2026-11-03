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

func main() {
	fmt.Println("================================================================================")
	fmt.Println(" TESTE WAL 1: QUEDA DO SERVIDOR 3 NA JANELA DE 10s (SEM VOLTAR)")
	fmt.Println(" Rota Multi-Servidor: SALVADOR (S1) -> FEIRA (S2) -> SERRINHA (S3) -> XIQUE XIQUE (S3)")
	fmt.Println(" Cenário: Passageiro faz PREPARE; durante os 10s o Servidor 3 cai e NÃO volta;")
	fmt.Println("          depois o passageiro tenta dar commit -> Sistema aborta e faz rollback!")
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

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_wal1_*")
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

	for i, cfg := range configs {
		os.MkdirAll(cfg.DataDir, 0755)
		if len(telasBytes) > 0 {
			os.WriteFile(filepath.Join(cfg.DataDir, "telas.json"), telasBytes, 0644)
		}
		cfgPath := filepath.Join(tempBaseDir, fmt.Sprintf("server%d.json", i+1))
		cfgPaths[i] = cfgPath
		cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
		os.WriteFile(cfgPath, cfgBytes, 0644)

		cmd := exec.Command(serverBin, cfgPath)
		cmd.Dir = serverDir
		cmd.Env = append(os.Environ(), "WAL_COMMIT_RETRY_TIMEOUT_SEC=4")
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
	vagas := 5

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

	// Passo 3: Cadastrar e logar passageiro no Server 1
	fmt.Println("\n[Passo 3] Cadastrando e autenticando passageiro no Server 1...")
	pClient, err := Apiclient.Dial("127.0.0.1:"+tcp1, "http://127.0.0.1:"+http1)
	if err != nil {
		fmt.Printf("Erro ao conectar passageiro: %v\n", err)
		return
	}
	defer pClient.Close()

	pClient.RegistraPassageiro("Passageiro_WAL1", "123")
	_, pTk, err := pClient.AuthPassageiro("Passageiro_WAL1", "123")
	if err != nil || pTk == "" {
		fmt.Printf("Erro no login do passageiro: %v\n", err)
		return
	}
	fmt.Printf(" -> Passageiro autenticado com sucesso! Token: %s\n", pTk)

	// Passo 4: Buscar rota de SALVADOR até XIQUE XIQUE
	fmt.Println("\n[Passo 4] Buscando rota de SALVADOR até XIQUE XIQUE via Server 1...")
	rotas, menu, err := pClient.BuscarRotas("SALVADOR", "XIQUE XIQUE", dataViagem, pTk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rotas: %v\n", err)
		return
	}
	fmt.Print(menu)
	rotaEscolhida := rotas[0]

	// Passo 5: Solicitar rota (PREPARE)
	fmt.Println("\n[Passo 5] Passageiro solicita a corrida (1ª Fase 2PC: PREPARE / Trava de 10s)...")
	msgPrep, errPrep := pClient.TentaReserva(rotaEscolhida, false, pTk)
	if errPrep != nil {
		fmt.Printf("Falha no PREPARE: %v (%s)\n", errPrep, msgPrep)
		return
	}
	fmt.Printf(" -> PREPARE concluído! Trava de 10s ativa nos 3 servidores! Resposta: %s\n", strings.TrimSpace(msgPrep))

	// Passo 6: Derrubar o Servidor 3 durante a janela de 10s (SEM VOLTAR)
	fmt.Println("\n================================================================================")
	fmt.Println(" [Passo 6] >>> DERRUBANDO O SERVIDOR 3 DENTRO DA JANELA DE 10s (SEM VOLTAR) <<<")
	fmt.Println("================================================================================")
	if cmds[2] != nil && cmds[2].Process != nil {
		_ = cmds[2].Process.Kill()
		_, _ = cmds[2].Process.Wait()
	}
	fmt.Println(" -> Servidor 3 (Xique Xique) foi encerrado abruptamente e NÃO voltará!")

	// Passo 7: Passageiro tenta confirmar (COMMIT) com o Servidor 3 caído
	fmt.Println("\n[Passo 7] Passageiro envia confirmação (COMMIT) enquanto o Servidor 3 está OFFLINE...")
	msgCom, errCom := pClient.TentaReserva(rotaEscolhida, true, pTk)
	fmt.Printf(" -> Resultado da tentativa de COMMIT pelo passageiro:\n")
	fmt.Printf("    Mensagem: %s\n", strings.TrimSpace(msgCom))
	fmt.Printf("    Erro retornado: %v\n", errCom)

	// Passo 8: Auditoria de Disco nos 3 servidores e nos arquivos WAL
	fmt.Println("\n================================================================================")
	fmt.Println(" AUDITORIA DE CONSISTÊNCIA E LOG WAL (wal.jsonl) NOS SERVIDORES")
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

	lerWAL := func(dataDir string) []string {
		raw, err := os.ReadFile(filepath.Join(dataDir, "wal.jsonl"))
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSpace(string(raw)), "\n")
	}

	servidoresData := []struct {
		id      string
		dataDir string
		origem  string
		destino string
	}{
		{"Server 1 (Coordenador)", configs[0].DataDir, "SALVADOR", "FEIRA DE SANTANA"},
		{"Server 2 (Intermediário)", configs[1].DataDir, "FEIRA DE SANTANA", "SERRINHA"},
		{"Server 3 (Caído)", configs[2].DataDir, "SERRINHA", "XIQUE XIQUE"},
	}

	vagasIntactas := true
	for _, s := range servidoresData {
		tr := lerTrechos(s.dataDir)
		wal := lerWAL(s.dataDir)
		fmt.Printf("  • [%s]:\n", s.id)
		for id, t := range tr {
			fmt.Printf("    - Trecho %s (%s -> %s): %d vagas restantes (Esperado: 5 intactas)\n", id, s.origem, s.destino, t.Nassentos)
			fmt.Printf("    - Passageiros alocados : %v\n", t.IdPassageiros)
			if t.Nassentos != 5 {
				vagasIntactas = false
			}
		}
		fmt.Printf("    - Entradas registradas no WAL (%d linhas):\n", len(wal))
		for _, w := range wal {
			if w != "" {
				fmt.Printf("      %s\n", w)
			}
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" CONCLUSÃO DO TESTE WAL 1")
	fmt.Println("================================================================================")
	if errCom != nil && vagasIntactas {
		fmt.Println(" >>> SUCESSO: Como o Servidor 3 caiu e não voltou, o Coordenador 2PC abortou a operação! <<<")
		fmt.Println(" >>> O rollback garantiu atomicidade: nenhuma vaga foi consumida em falso e as 5 vagas continuam livres! <<<")
	} else {
		fmt.Printf(" >>> FALHA: Inconsistência! errCom=%v | vagasIntactas=%v <<<\n", errCom, vagasIntactas)
	}
	fmt.Println("================================================================================")
}

