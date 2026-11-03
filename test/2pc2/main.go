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
	fmt.Println(" TESTE 2PC 2: RESERVA E CANCELAMENTO (ABORT) EM 3 SERVIDORES")
	fmt.Println(" Rota Multi-Servidor: SALVADOR (S1) -> FEIRA (S2) -> SERRINHA (S3) -> XIQUE XIQUE (S3)")
	fmt.Println(" Cenário: 1 Passageiro solicita rota completa (PREPARE) e depois cancela (ABORT)")
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

	tempBaseDir, err := os.MkdirTemp("", "jasefoi_2pc2_*")
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
	vagas := 5

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

	// Passo 3: Cadastrar e logar 1 passageiro no Server 1
	fmt.Println("\n[Passo 3] Cadastrando e autenticando passageiro no Server 1...")
	pClient, err := Apiclient.Dial("127.0.0.1:"+tcp1, "http://127.0.0.1:"+http1)
	if err != nil {
		fmt.Printf("Erro ao conectar passageiro: %v\n", err)
		return
	}
	defer pClient.Close()

	pClient.RegistraPassageiro("Passageiro_2PC2", "123")
	_, pTk, err := pClient.AuthPassageiro("Passageiro_2PC2", "123")
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
	fmt.Printf(" -> Rota encontrada abrangendo 3 servidores com %d trecho(s):\n", len(rotaEscolhida))
	for idx, a := range rotaEscolhida {
		fmt.Printf("    Trecho %d: %s -> %s (Motoristas: %v)\n", idx+1, a.Origem, a.Destino, a.Ids)
	}

	// Passo 5: Solicitar reserva (PREPARE)
	fmt.Println("\n[Passo 5] Solicitando reserva (1ª Fase 2PC: PREPARE / Trava 10s nos 3 servidores)...")
	msgPrep, errPrep := pClient.TentaReserva(rotaEscolhida, false, pTk)
	if errPrep != nil {
		fmt.Printf("Falha no PREPARE: %v (%s)\n", errPrep, msgPrep)
		return
	}
	fmt.Printf(" -> PREPARE concedido! Resposta: %s\n", strings.TrimSpace(msgPrep))

	// Passo 6: Cancelar reserva pendente (2PC ABORT)
	fmt.Println("\n[Passo 6] Passageiro decide CANCELAR a reserva (2PC ABORT coordenado)...")
	msgCanc, errCanc := pClient.CancelarReservaPendente(rotaEscolhida, pTk)
	if errCanc != nil {
		fmt.Printf("Falha no cancelamento: %v (%s)\n", errCanc, msgCanc)
		return
	}
	fmt.Printf(" -> Cancelamento (2PC ABORT) concluído com sucesso em todos os 3 servidores! Resposta: %s\n", strings.TrimSpace(msgCanc))

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

	sucessoTotal := true
	for _, s := range servidoresData {
		tr := lerTrechos(s.dataDir)
		for id, t := range tr {
			fmt.Printf("  • [%s] Trecho %s (%s -> %s):\n", s.id, id, s.origem, s.destino)
			fmt.Printf("    - Assentos restantes   : %d (Esperado: 5 intactos)\n", t.Nassentos)
			fmt.Printf("    - Passageiros alocados : %v\n", t.IdPassageiros)
			if t.Nassentos != 5 {
				sucessoTotal = false
			}
			for _, pass := range t.IdPassageiros {
				if pass != "" {
					sucessoTotal = false
				}
			}
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" CONCLUSÃO DO TESTE 2PC 2")
	fmt.Println("================================================================================")
	if sucessoTotal {
		fmt.Println(" >>> SUCESSO: O cancelamento (2PC ABORT) liberou as travas em todos os 3 servidores! <<<")
		fmt.Println(" >>> Todas as 5 vagas permanecem 100% disponíveis e nenhum assento foi consumido! <<<")
	} else {
		fmt.Println(" >>> FALHA: Inconsistência após cancelamento! <<<")
	}
	fmt.Println("================================================================================")
}

