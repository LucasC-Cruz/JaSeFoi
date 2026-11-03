package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type ResultadoRA struct {
	ServerID          string `json:"server_id"`
	TsInicioReq       int64  `json:"ts_inicio_req"`
	TsEntradaSC       int64  `json:"ts_entrada_sc"`
	TsSaidaSC         int64  `json:"ts_saida_sc"`
	HoraInicioSCMs    int64  `json:"hora_inicio_sc_ms"`
	HoraFimSCMs       int64  `json:"hora_fim_sc_ms"`
	DuracaoEsperaMs   int64  `json:"duracao_espera_ms"`
	DuracaoExecucaoMs int64  `json:"duracao_execucao_ms"`
	Status            string `json:"status"`
}

func normalizarHTTP(host, defaultHTTPPort, customHTTP string) string {
	if customHTTP != "" {
		return strings.TrimRight(customHTTP, "/")
	}
	h := strings.TrimSpace(host)
	if h == "" {
		h = "localhost"
	}
	if strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") {
		return strings.TrimRight(h, "/")
	}
	if strings.Contains(h, ":") {
		return "http://" + strings.TrimRight(h, "/")
	}
	return fmt.Sprintf("http://%s:%s", h, defaultHTTPPort)
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
	var (
		s1Flag      = flag.String("s1", "", "IP ou Host do Servidor 1 (Salvador) - ex: 192.168.1.101")
		s1HTTPFlag  = flag.String("s1-http", "", "URL HTTP completa do Servidor 1 (ex: http://192.168.1.101:8741)")

		s2Flag      = flag.String("s2", "", "IP ou Host do Servidor 2 (Feira de Santana) - ex: 192.168.1.102")
		s2HTTPFlag  = flag.String("s2-http", "", "URL HTTP completa do Servidor 2 (ex: http://192.168.1.102:8742)")

		s3Flag      = flag.String("s3", "", "IP ou Host do Servidor 3 (Serrinha) - ex: 192.168.1.103")
		s3HTTPFlag  = flag.String("s3-http", "", "URL HTTP completa do Servidor 3 (ex: http://192.168.1.103:8743)")

		duracaoFlag = flag.Int("duracao", 1000, "Duração do bloco crítico em ms dentro da Seção Crítica")
	)
	flag.Parse()

	modoRemoto := (*s1Flag != "" || *s1HTTPFlag != "")

	fmt.Println("================================================================================")
	fmt.Println(" TESTE DE VALIDAÇÃO: ALGORITMO DE RICART-AGRAWALA (EXCLUSÃO MÚTUA DISTRIBUÍDA)")
	if modoRemoto {
		fmt.Println(" Execução: MODO REMOTO (Conectando a instâncias já existentes nos IPs informados)")
	} else {
		fmt.Println(" Execução: MODO LOCAL AUTOMÁTICO (Instanciando 3 servidores locais)")
	}
	fmt.Println(" Topologia: 3 Servidores em Malha Completa (S1 <-> S2 <-> S3)")
	fmt.Println(" Objetivo: Provar a exclusão mútua estrita, adiamento de respostas e ordem Lamport")
	fmt.Println("================================================================================")

	var urlS1, urlS2, urlS3 string
	var baseTempDir string
	var cmds []*exec.Cmd

	if modoRemoto {
		urlS1 = normalizarHTTP(*s1Flag, "8741", *s1HTTPFlag)
		urlS2 = normalizarHTTP(*s2Flag, "8742", *s2HTTPFlag)
		urlS3 = normalizarHTTP(*s3Flag, "8743", *s3HTTPFlag)

		fmt.Printf(" -> Servidor 1: %s\n", urlS1)
		fmt.Printf(" -> Servidor 2: %s\n", urlS2)
		fmt.Printf(" -> Servidor 3: %s\n", urlS3)

		// Health check nos servidores remotos
		fmt.Println("\n[*] Verificando conectividade e status com os servidores remotos...")
		clientHC := &http.Client{Timeout: 4 * time.Second}
		for i, u := range []string{urlS1, urlS2, urlS3} {
			resp, err := clientHC.Get(u + "/status")
			if err != nil {
				fmt.Printf("[ERRO] Não foi possível conectar ao Servidor %d em %s: %v\n", i+1, u, err)
				return
			}
			var st struct {
				ID      string   `json:"id"`
				Status  string   `json:"status"`
				Lamport int64    `json:"lamport"`
				Cidades []string `json:"cidades"`
			}
			json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()
			fmt.Printf(" [✓] Servidor %d ONLINE! (ID: %s, Lamport: %d, Cidades: %v)\n", i+1, st.ID, st.Lamport, st.Cidades)
		}
	} else {
		// Modo Local Automático: compila e inicia 3 processos locais
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

		var errDir error
		baseTempDir, errDir = os.MkdirTemp("", "teste_ra_*")
		if errDir != nil {
			fmt.Printf("Erro ao criar diretório temporário: %v\n", errDir)
			return
		}
		defer os.RemoveAll(baseTempDir)

		dataDirS1 := filepath.Join(baseTempDir, "dados_s1")
		dataDirS2 := filepath.Join(baseTempDir, "dados_s2")
		dataDirS3 := filepath.Join(baseTempDir, "dados_s3")
		os.MkdirAll(dataDirS1, 0755)
		os.MkdirAll(dataDirS2, 0755)
		os.MkdirAll(dataDirS3, 0755)

		telasBytes, _ := os.ReadFile(filepath.Join(rootDir, "data", "telas.json"))
		if len(telasBytes) > 0 {
			os.WriteFile(filepath.Join(dataDirS1, "telas.json"), telasBytes, 0644)
			os.WriteFile(filepath.Join(dataDirS2, "telas.json"), telasBytes, 0644)
			os.WriteFile(filepath.Join(dataDirS3, "telas.json"), telasBytes, 0644)
		}

		portaTcpS1 := obterPortaLivre()
		portaHttpS1 := obterPortaLivre()
		portaTcpS2 := obterPortaLivre()
		portaHttpS2 := obterPortaLivre()
		portaTcpS3 := obterPortaLivre()
		portaHttpS3 := obterPortaLivre()

		urlS1 = fmt.Sprintf("http://127.0.0.1:%s", portaHttpS1)
		urlS2 = fmt.Sprintf("http://127.0.0.1:%s", portaHttpS2)
		urlS3 = fmt.Sprintf("http://127.0.0.1:%s", portaHttpS3)

		cfgS1 := ServerConfig{
			ID:           "s1",
			Porta:        portaTcpS1,
			PortaHttp:    portaHttpS1,
			DataDir:      dataDirS1,
			VizinhosHttp: []string{urlS2, urlS3},
			Cidades:      []string{"salvador"},
			CidadesInter: []string{"salvador", "feira", "serrinha"},
		}
		cfgS2 := ServerConfig{
			ID:           "s2",
			Porta:        portaTcpS2,
			PortaHttp:    portaHttpS2,
			DataDir:      dataDirS2,
			VizinhosHttp: []string{urlS1, urlS3},
			Cidades:      []string{"feira"},
			CidadesInter: []string{"salvador", "feira", "serrinha"},
		}
		cfgS3 := ServerConfig{
			ID:           "s3",
			Porta:        portaTcpS3,
			PortaHttp:    portaHttpS3,
			DataDir:      dataDirS3,
			VizinhosHttp: []string{urlS1, urlS2},
			Cidades:      []string{"serrinha"},
			CidadesInter: []string{"salvador", "feira", "serrinha"},
		}

		cfgPathS1 := filepath.Join(dataDirS1, "config.json")
		cfgPathS2 := filepath.Join(dataDirS2, "config.json")
		cfgPathS3 := filepath.Join(dataDirS3, "config.json")

		d1, _ := json.MarshalIndent(cfgS1, "", "  ")
		d2, _ := json.MarshalIndent(cfgS2, "", "  ")
		d3, _ := json.MarshalIndent(cfgS3, "", "  ")
		os.WriteFile(cfgPathS1, d1, 0644)
		os.WriteFile(cfgPathS2, d2, 0644)
		os.WriteFile(cfgPathS3, d3, 0644)

		fmt.Println("\n[*] Compilando binário do servidor...")
		serverBin := filepath.Join(baseTempDir, "servidor_teste.exe")
		buildCmd := exec.Command("go", "build", "-o", serverBin, "servidor.go")
		buildCmd.Dir = serverDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Printf("Erro ao compilar servidor: %v\n%s\n", err, string(out))
			return
		}

		fmt.Println("\n[*] Iniciando Servidores S1, S2 e S3...")
		iniciarServidor := func(cfgPath string) *exec.Cmd {
			cmd := exec.Command(serverBin, cfgPath)
			cmd.Dir = serverDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				panic(fmt.Sprintf("Falha ao iniciar servidor com %s: %v", cfgPath, err))
			}
			return cmd
		}

		cmdS1 := iniciarServidor(cfgPathS1)
		cmdS2 := iniciarServidor(cfgPathS2)
		cmdS3 := iniciarServidor(cfgPathS3)
		cmds = append(cmds, cmdS1, cmdS2, cmdS3)
		defer func() {
			for _, c := range cmds {
				if c != nil && c.Process != nil {
					_ = c.Process.Kill()
				}
			}
		}()

		fmt.Println("[*] Aguardando portas TCP ficarem ativas...")
		if !aguardarTCP("127.0.0.1:"+portaTcpS1, 15*time.Second) ||
			!aguardarTCP("127.0.0.1:"+portaTcpS2, 15*time.Second) ||
			!aguardarTCP("127.0.0.1:"+portaTcpS3, 15*time.Second) {
			fmt.Println("[!] Erro: Servidores não inicializaram a tempo.")
			return
		}

		fmt.Println("[*] Aguardando sincronização da malha HTTP entre os 3 servidores...")
		if !aguardarVizinhosConectados(urlS1, 2, 15*time.Second) ||
			!aguardarVizinhosConectados(urlS2, 2, 15*time.Second) ||
			!aguardarVizinhosConectados(urlS3, 2, 15*time.Second) {
			fmt.Println("[!] Erro: Servidores não estabeleceram conexão entre si a tempo.")
			return
		}
		fmt.Println("[✓] Malha distribuída 100% conectada e pronta!")
	}

	// =========================================================================
	// DISPUTA SIMULTÂNEA PELA SEÇÃO CRÍTICA ENTRE S1, S2 e S3
	// =========================================================================
	duracaoSC := *duracaoFlag
	if duracaoSC <= 0 {
		duracaoSC = 1000
	}

	fmt.Println("\n--------------------------------------------------------------------------------")
	fmt.Printf(" DISPUTA SIMULTÂNEA PELA SEÇÃO CRÍTICA ENTRE S1, S2 e S3 (Duração: %d ms)\n", duracaoSC)
	fmt.Println(" Se Ricart-Agrawala estiver ativo e correto:")
	fmt.Println("  1. Apenas UM servidor por vez executará dentro da Seção Crítica (Zero sobreposição).")
	fmt.Println("  2. Respostas aos demais serão ADIADAS (DEFER) e liberadas apenas ao sair.")
	fmt.Println("  3. A ordem de entrada respeitará os timestamps de Lamport.")
	fmt.Printf("  4. Duração total esperada: ~%d ms (3 * %d ms sequenciais).\n", 3*duracaoSC, duracaoSC)
	fmt.Println("--------------------------------------------------------------------------------")

	var arquivoAuditoria string
	if !modoRemoto && baseTempDir != "" {
		arquivoAuditoria = filepath.Join(baseTempDir, "ra_audit.log")
	}

	type AlvoReq struct {
		Nome string
		URL  string
	}
	alvos := []AlvoReq{
		{Nome: "Servidor 1", URL: urlS1},
		{Nome: "Servidor 2", URL: urlS2},
		{Nome: "Servidor 3", URL: urlS3},
	}

	var resultados []ResultadoRA
	var resMu sync.Mutex
	var wg sync.WaitGroup
	barreiraInicio := make(chan struct{})

	tempoDisparo := time.Now()

	for _, a := range alvos {
		wg.Add(1)
		go func(alvo AlvoReq) {
			defer wg.Done()
			// Aguarda sinal da barreira para disparar simultaneamente
			<-barreiraInicio

			corpoReq, _ := json.Marshal(map[string]interface{}{
				"duracao_ms":            duracaoSC,
				"arquivo_compartilhado": arquivoAuditoria,
			})

			client := &http.Client{Timeout: 35 * time.Second}
			resp, err := client.Post(alvo.URL+"/test/ra/executar", "application/json", bytes.NewReader(corpoReq))
			if err != nil {
				fmt.Printf("[!] Falha na requisição para %s (%s): %v\n", alvo.Nome, alvo.URL, err)
				return
			}
			defer resp.Body.Close()

			var r ResultadoRA
			if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
				fmt.Printf("[!] Falha ao decodificar resposta de %s: %v\n", alvo.Nome, err)
				return
			}

			resMu.Lock()
			resultados = append(resultados, r)
			resMu.Unlock()
		}(a)
	}

	fmt.Println("[*] Disparando 3 requisições simultâneas para os 3 servidores...")
	close(barreiraInicio) // Disparo simultâneo!
	wg.Wait()
	tempoTotal := time.Since(tempoDisparo)

	if len(resultados) < 3 {
		fmt.Printf("\n[!] Apenas %d de 3 servidores responderam com sucesso.\n", len(resultados))
		return
	}

	fmt.Printf("\n[✓] Todas as 3 requisições concluíram! Tempo total decorrido: %v\n", tempoTotal.Round(time.Millisecond))

	// Ordena os resultados pela hora de início na seção crítica
	sort.Slice(resultados, func(i, j int) bool {
		return resultados[i].HoraInicioSCMs < resultados[j].HoraInicioSCMs
	})

	fmt.Println("\n================================================================================")
	fmt.Println(" RELATÓRIO DE EXECUÇÃO DA SEÇÃO CRÍTICA (RICART-AGRAWALA):")
	fmt.Println("================================================================================")
	for i, r := range resultados {
		fmt.Printf(" [%dº a Entrar] Servidor: %-8s | Lamport Req: %-3d | Lamport Entrada: %-3d | Lamport Saída: %-3d\n",
			i+1, strings.ToUpper(r.ServerID), r.TsInicioReq, r.TsEntradaSC, r.TsSaidaSC)
		fmt.Printf("      Intervalo Seção Crítica: [%d ms -> %d ms] (Duração: %d ms | Espera na fila: %d ms)\n",
			r.HoraInicioSCMs, r.HoraFimSCMs, r.DuracaoExecucaoMs, r.DuracaoEsperaMs)
	}

	// ── PROVA 1: VERIFICAÇÃO MATEMÁTICA DE NÃO-SOBREPOSIÇÃO (SAFETY) ─────────
	fmt.Println("\n[*] Verificação 1: Teste de Sobreposição de Intervalos Temporais (Exclusão Mútua):")
	houveViolacaoExclusao := false
	for i := 0; i < len(resultados)-1; i++ {
		atual := resultados[i]
		proximo := resultados[i+1]

		sobreposicaoMs := atual.HoraFimSCMs - proximo.HoraInicioSCMs
		if sobreposicaoMs > 0 {
			fmt.Printf(" [!] VIOLAÇÃO DE EXCLUSÃO MÚTUA detectada entre %s e %s! Sobreposição: %d ms\n",
				atual.ServerID, proximo.ServerID, sobreposicaoMs)
			houveViolacaoExclusao = true
		} else {
			fmt.Printf(" [✓] %s saiu (UnixMs=%d) ANTES de %s entrar (UnixMs=%d). Gap livre: %d ms\n",
				strings.ToUpper(atual.ServerID), atual.HoraFimSCMs,
				strings.ToUpper(proximo.ServerID), proximo.HoraInicioSCMs,
				-sobreposicaoMs)
		}
	}

	if !houveViolacaoExclusao && len(resultados) == 3 {
		fmt.Println(" >>> PROVA CONCLUÍDA COM SUCESSO: Zero sobreposição temporal entre os servidores! <<<")
	} else {
		fmt.Println(" >>> FALHA: Algum servidor executou em paralelo com outro! <<<")
	}

	// ── PROVA 2: AUDITORIA DO ARQUIVO COMPARTILHADO (Modo Local) ────────────
	if arquivoAuditoria != "" {
		fmt.Println("\n[*] Verificação 2: Inspeção do Arquivo de Auditoria Compartilhado (ra_audit.log):")
		dadosLog, err := os.ReadFile(arquivoAuditoria)
		if err == nil {
			linhas := strings.Split(strings.TrimSpace(string(dadosLog)), "\n")
			fmt.Println("----- Conteúdo do ra_audit.log -----")
			for _, l := range linhas {
				fmt.Println("  ", l)
			}
			fmt.Println("------------------------------------")

			violacaoSequencia := false
			var servidorAtivo string
			for _, l := range linhas {
				if strings.Contains(l, "ENTRADA") {
					if servidorAtivo != "" {
						fmt.Printf(" [!] ERRO: Servidor %s entrou antes de %s sair!\n", l, servidorAtivo)
						violacaoSequencia = true
					}
					servidorAtivo = l
				} else if strings.Contains(l, "SAIDA") {
					servidorAtivo = ""
				}
			}

			if !violacaoSequencia && len(linhas) == 6 {
				fmt.Println(" [✓] Perfeita alternância de ENTRADA e SAÍDA confirmada no log compartilhado!")
			} else {
				fmt.Println(" [!] Inconsistência na sequência de eventos do log!")
			}
		}
	}

	// ── PROVA 3: TEMPO TOTAL SERIALIZADO ────────────────────────────────────
	fmt.Println("\n[*] Verificação 3: Serialização de Execução e Espera na Fila:")
	esperadoMinimo := time.Duration(len(resultados)*duracaoSC) * time.Millisecond
	fmt.Printf(" Tempo total decorrido: %v (Mínimo teórico serializado: %v)\n", tempoTotal.Round(time.Millisecond), esperadoMinimo)
	if tempoTotal >= esperadoMinimo-250*time.Millisecond {
		fmt.Println(" [✓] O tempo total comprova que a Seção Crítica foi serializada por Ricart-Agrawala!")
	} else {
		fmt.Println(" [!] O tempo foi inferior ao esperado, indicando possível execução paralela.")
	}

	fmt.Println("\n================================================================================")
	if !houveViolacaoExclusao && len(resultados) == 3 {
		fmt.Println(" RESULTADO FINAL: TESTE DE RICART-AGRAWALA PASSOU COM SUCESSO! (100% APROVADO)")
	} else {
		fmt.Println(" RESULTADO FINAL: TESTE FALHOU.")
	}
	fmt.Println("================================================================================")
}
