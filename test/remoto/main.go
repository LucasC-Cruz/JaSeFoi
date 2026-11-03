package main

import (
	"Golang/Apiclient"
	"Golang/Model"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ServerStatusResponse struct {
	ID       string                 `json:"id"`
	Status   string                 `json:"status"`
	Cidades  []string               `json:"cidades"`
	Lamport  int64                  `json:"lamport"`
	Trechos  map[string]Model.Trecho `json:"trechos"`
	Vizinhos map[string]interface{} `json:"vizinhos"`
}

type NodeAddress struct {
	Name    string
	Host    string
	TCP     string
	HTTP    string
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

func normalizarEndereco(host, defaultTCPPort, defaultHTTPPort, customTCP, customHTTP string) (string, string) {
	tcp := customTCP
	if tcp == "" {
		if strings.Contains(host, ":") {
			tcp = host
		} else {
			tcp = host + ":" + defaultTCPPort
		}
	}

	httpAddr := customHTTP
	if httpAddr == "" {
		h := host
		if strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") {
			httpAddr = h
		} else {
			if strings.Contains(h, ":") {
				httpAddr = "http://" + h
			} else {
				httpAddr = "http://" + h + ":" + defaultHTTPPort
			}
		}
	}
	return tcp, strings.TrimRight(httpAddr, "/")
}

func testarConectividadeTCP(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func obterStatusRemoto(httpURL string) (*ServerStatusResponse, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(httpURL + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status HTTP %d", resp.StatusCode)
	}

	var st ServerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

func fazerRequisicaoReserva(httpBase, passNome, passTk string, rota []Model.Aresta, confirma, cancelar bool, lamportAtual int64) (int, string, int64, int64, error) {
	httpClient := &http.Client{Timeout: 35 * time.Second}

	reqData := Model.InfoConfirmarReserva{
		Rota:        rota,
		Confirma:    confirma,
		Cancelar:    cancelar,
		TkAssociado: passTk,
	}
	bodyBytes, _ := json.Marshal(reqData)

	urlReserva := fmt.Sprintf("%s/passageiro/tentar-reserva", httpBase)
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

func main() {
	var (
		s1Host     = flag.String("s1", "localhost", "IP ou Host do Servidor 1 (Salvador)")
		s1TCP      = flag.String("s1-tcp", "", "Endereço TCP completo do Servidor 1 (ex: 192.168.1.101:6741)")
		s1HTTP     = flag.String("s1-http", "", "URL HTTP completa do Servidor 1 (ex: http://192.168.1.101:8741)")

		s2Host     = flag.String("s2", "localhost", "IP ou Host do Servidor 2 (Feira / Serrinha)")
		s2TCP      = flag.String("s2-tcp", "", "Endereço TCP completo do Servidor 2 (ex: 192.168.1.102:6742)")
		s2HTTP     = flag.String("s2-http", "", "URL HTTP completa do Servidor 2 (ex: http://192.168.1.102:8742)")

		s3Host     = flag.String("s3", "localhost", "IP ou Host do Servidor 3 (Serrinha / Xique Xique)")
		s3TCP      = flag.String("s3-tcp", "", "Endereço TCP completo do Servidor 3 (ex: 192.168.1.103:6743)")
		s3HTTP     = flag.String("s3-http", "", "URL HTTP completa do Servidor 3 (ex: http://192.168.1.103:8743)")

		modoTeste  = flag.String("modo", "commit", "Modo do teste: 'commit' (2PC Commit), 'abort' (2PC Abort), 'concorrencia' (Disputa de vagas com Lamport), 'crash' (Tolerância a falhas interativa)")
		numVagas   = flag.Int("vagas", 5, "Número de vagas em cada trecho")
		numPass    = flag.Int("pass", 5, "Número de passageiros na concorrência (modo 'concorrencia')")
		dataViagem = flag.String("data", "28/10", "Data da viagem (formato DD/MM)")
	)
	flag.Parse()

	tcp1, http1 := normalizarEndereco(*s1Host, "6741", "8741", *s1TCP, *s1HTTP)
	tcp2, http2 := normalizarEndereco(*s2Host, "6742", "8742", *s2TCP, *s2HTTP)
	tcp3, http3 := normalizarEndereco(*s3Host, "6743", "8743", *s3TCP, *s3HTTP)

	nos := []NodeAddress{
		{"Servidor 1 (Salvador)", *s1Host, tcp1, http1},
		{"Servidor 2 (Feira/Serrinha)", *s2Host, tcp2, http2},
		{"Servidor 3 (Serrinha/Xique Xique)", *s3Host, tcp3, http3},
	}

	fmt.Println("================================================================================")
	fmt.Println(" SUÍTE DE TESTES DISTRIBUÍDOS REMOTOS (MULTI-MÁQUINAS / DOCKER)")
	fmt.Printf(" Modo selecionado : %s | Vagas: %d | Data: %s\n", strings.ToUpper(*modoTeste), *numVagas, *dataViagem)
	fmt.Println("================================================================================")
	fmt.Printf("  • S1: TCP=%s | HTTP=%s\n", tcp1, http1)
	fmt.Printf("  • S2: TCP=%s | HTTP=%s\n", tcp2, http2)
	fmt.Printf("  • S3: TCP=%s | HTTP=%s\n", tcp3, http3)
	fmt.Println("================================================================================")

	// 1. Health check de conectividade com as máquinas remotas
	fmt.Println("\n[Passo 1] Verificando conectividade e status com as máquinas remotas...")
	for _, n := range nos {
		fmt.Printf(" -> Testando %s (%s)...", n.Name, n.HTTP)
		if errTCP := testarConectividadeTCP(n.TCP, 3*time.Second); errTCP != nil {
			fmt.Printf(" FALHA TCP (%v)\n", errTCP)
			fmt.Printf("\n[ERRO] Não foi possível conectar na porta TCP %s do %s!\n", n.TCP, n.Name)
			fmt.Println("Certifique-se de que o servidor está rodando e a porta/firewall está liberada.")
			return
		}
		st, errHTTP := obterStatusRemoto(n.HTTP)
		if errHTTP != nil {
			fmt.Printf(" FALHA HTTP (%v)\n", errHTTP)
			fmt.Printf("\n[ERRO] Não foi possível consultar /status no endpoint HTTP %s!\n", n.HTTP)
			return
		}
		fmt.Printf(" OK! (ID: %s, Lamport: %d, Cidades: %v)\n", st.ID, st.Lamport, st.Cidades)
	}
	fmt.Println(" -> Todas as máquinas remotas estão ONLINE e respondendo!")

	// 2. Cadastrar motoristas e corridas nos 3 servidores
	tagUnica := time.Now().UnixNano() % 100000
	corridas := []struct {
		srv     NodeAddress
		nome    string
		origem  string
		destino string
		preco   float64
	}{
		{nos[0], fmt.Sprintf("MotS1_%d", tagUnica), "SALVADOR", "FEIRA DE SANTANA", 30.0},
		{nos[1], fmt.Sprintf("MotS2_%d", tagUnica), "FEIRA DE SANTANA", "SERRINHA", 25.0},
		{nos[2], fmt.Sprintf("MotS3_%d", tagUnica), "SERRINHA", "XIQUE XIQUE", 40.0},
	}

	fmt.Printf("\n[Passo 2] Cadastrando motoristas e corridas (%d vagas) nas 3 máquinas remotas...\n", *numVagas)
	for _, c := range corridas {
		mClient, err := Apiclient.Dial(c.srv.TCP)
		if err != nil {
			fmt.Printf("Erro ao conectar motorista em %s: %v\n", c.srv.Name, err)
			return
		}
		defer mClient.Close()
		mClient.RegistraMotorista(c.nome, "123", "Carro-"+c.srv.Name)
		_, tk, err := mClient.AuthMotorista(c.nome, "123")
		if err != nil {
			fmt.Printf("Erro login motorista em %s: %v\n", c.srv.Name, err)
			return
		}
		rotaStr := c.origem + "," + c.destino
		if _, err := mClient.CadastrarCorrida(rotaStr, *dataViagem, *numVagas, float32(c.preco), tk, true); err != nil {
			fmt.Printf("Erro ao cadastrar corrida em %s: %v\n", c.srv.Name, err)
			return
		}
		fmt.Printf(" -> [%s] Corrida '%s -> %s' cadastrada com sucesso!\n", c.srv.Name, c.origem, c.destino)
	}

	switch strings.ToLower(*modoTeste) {
	case "commit", "2pc1":
		executarModoCommit(nos, *dataViagem, tagUnica)
	case "abort", "2pc2":
		executarModoAbort(nos, *dataViagem, tagUnica)
	case "concorrencia", "2pc3", "lamport":
		executarModoConcorrencia(nos, *dataViagem, *numVagas, *numPass, tagUnica)
	case "crash":
		executarModoCrashInterativo(nos, *dataViagem, tagUnica)
	default:
		fmt.Printf("Modo desconhecido '%s'. Use 'commit', 'abort', 'concorrencia' ou 'crash'.\n", *modoTeste)
	}
}

func executarModoCommit(nos []NodeAddress, dataViagem string, tag int64) {
	fmt.Println("\n================================================================================")
	fmt.Println(" EXECUTANDO TESTE: 2PC COMMIT DISTRIBUÍDO EM 3 MÁQUINAS")
	fmt.Println("================================================================================")

	pClient, err := Apiclient.Dial(nos[0].TCP, nos[0].HTTP)
	if err != nil {
		fmt.Printf("Erro ao conectar passageiro no S1: %v\n", err)
		return
	}
	defer pClient.Close()

	passNome := fmt.Sprintf("PassRemoto_%d", tag)
	pClient.RegistraPassageiro(passNome, "123")
	_, pTk, err := pClient.AuthPassageiro(passNome, "123")
	if err != nil || pTk == "" {
		fmt.Printf("Erro no login do passageiro: %v\n", err)
		return
	}
	fmt.Printf(" -> Passageiro '%s' autenticado no Servidor 1!\n", passNome)

	rotas, menu, err := pClient.BuscarRotas("SALVADOR", "XIQUE XIQUE", dataViagem, pTk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rotas: %v\n", err)
		return
	}
	fmt.Print(menu)
	rota := rotas[0]

	fmt.Println("\n -> Solicitando reserva remota (1ª Fase PREPARE)...")
	msgPrep, errPrep := pClient.TentaReserva(rota, false, pTk)
	if errPrep != nil {
		fmt.Printf("Falha no PREPARE: %v (%s)\n", errPrep, msgPrep)
		return
	}
	fmt.Printf(" -> PREPARE OK nos 3 servidores! Resposta: %s\n", strings.TrimSpace(msgPrep))

	fmt.Println("\n -> Confirmando reserva remota (2ª Fase COMMIT)...")
	msgCom, errCom := pClient.TentaReserva(rota, true, pTk)
	if errCom != nil {
		fmt.Printf("Falha no COMMIT: %v (%s)\n", errCom, msgCom)
		return
	}
	fmt.Printf(" -> COMMIT OK nos 3 servidores! Resposta: %s\n", strings.TrimSpace(msgCom))

	fmt.Println("\n================================================================================")
	fmt.Println(" AUDITORIA REMOTA DOS TRECHOS VIA HTTP (/status)")
	fmt.Println("================================================================================")
	for _, n := range nos {
		st, err := obterStatusRemoto(n.HTTP)
		if err != nil {
			fmt.Printf("  • [%s]: Erro ao obter status (%v)\n", n.Name, err)
			continue
		}
		fmt.Printf("  • [%s] (Lamport: %d):\n", n.Name, st.Lamport)
		for id, t := range st.Trechos {
			if t.Data == dataViagem {
				fmt.Printf("    - Trecho %s (Data: %s): %d vagas restantes | Passageiros: %v\n", id, t.Data, t.Nassentos, t.IdPassageiros)
			}
		}
	}
	fmt.Println("\n >>> TESTE CONCLUÍDO COM SUCESSO NAS MÁQUINAS REMOTAS! <<<")
}

func executarModoAbort(nos []NodeAddress, dataViagem string, tag int64) {
	fmt.Println("\n================================================================================")
	fmt.Println(" EXECUTANDO TESTE: 2PC ABORT (CANCELAMENTO) EM 3 MÁQUINAS")
	fmt.Println("================================================================================")

	pClient, err := Apiclient.Dial(nos[0].TCP, nos[0].HTTP)
	if err != nil {
		fmt.Printf("Erro ao conectar passageiro no S1: %v\n", err)
		return
	}
	defer pClient.Close()

	passNome := fmt.Sprintf("PassCanc_%d", tag)
	pClient.RegistraPassageiro(passNome, "123")
	_, pTk, err := pClient.AuthPassageiro(passNome, "123")
	if err != nil || pTk == "" {
		fmt.Printf("Erro login: %v\n", err)
		return
	}

	rotas, _, err := pClient.BuscarRotas("SALVADOR", "XIQUE XIQUE", dataViagem, pTk)
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro ao buscar rota: %v\n", err)
		return
	}
	rota := rotas[0]

	fmt.Println("\n -> Solicitando PREPARE nos 3 servidores...")
	msgPrep, errPrep := pClient.TentaReserva(rota, false, pTk)
	if errPrep != nil {
		fmt.Printf("Falha PREPARE: %v\n", errPrep)
		return
	}
	fmt.Printf(" -> PREPARE concedido! Resposta: %s\n", strings.TrimSpace(msgPrep))

	fmt.Println("\n -> Cancelando reserva (2PC ABORT)...")
	msgCanc, errCanc := pClient.CancelarReservaPendente(rota, pTk)
	if errCanc != nil {
		fmt.Printf("Falha no cancelamento: %v\n", errCanc)
		return
	}
	fmt.Printf(" -> Cancelamento concluído! Resposta: %s\n", strings.TrimSpace(msgCanc))

	fmt.Println("\n================================================================================")
	fmt.Println(" AUDITORIA REMOTA DOS TRECHOS VIA HTTP (/status)")
	fmt.Println("================================================================================")
	for _, n := range nos {
		st, _ := obterStatusRemoto(n.HTTP)
		fmt.Printf("  • [%s] (Lamport: %d):\n", n.Name, st.Lamport)
		for id, t := range st.Trechos {
			if t.Data == dataViagem {
				fmt.Printf("    - Trecho %s (Data: %s): %d vagas restantes (Intactas) | Passageiros: %v\n", id, t.Data, t.Nassentos, t.IdPassageiros)
			}
		}
	}
	fmt.Println("\n >>> TESTE CONCLUÍDO COM SUCESSO: ABORT LIBEROU TODAS AS TRAVAS REMOTAS! <<<")
}

func executarModoConcorrencia(nos []NodeAddress, dataViagem string, vagas, numPass int, tag int64) {
	fmt.Println("\n================================================================================")
	fmt.Printf(" EXECUTANDO TESTE: %d PASSAGEIROS DISPUTAM %d VAGAS EM 3 MÁQUINAS COM LAMPORT\n", numPass, vagas)
	fmt.Println("================================================================================")

	tokens := make([]string, numPass)
	for i := 0; i < numPass; i++ {
		pNome := fmt.Sprintf("PassConc_%d_%d", tag, i+1)
		c, err := Apiclient.Dial(nos[0].TCP, nos[0].HTTP)
		if err != nil {
			fmt.Printf("Erro ao conectar %s: %v\n", pNome, err)
			return
		}
		c.RegistraPassageiro(pNome, "123")
		_, tk, _ := c.AuthPassageiro(pNome, "123")
		tokens[i] = tk
		c.Close()
	}
	fmt.Printf(" -> %d passageiros autenticados no Servidor 1!\n", numPass)

	clientP1, _ := Apiclient.Dial(nos[0].TCP, nos[0].HTTP)
	defer clientP1.Close()
	rotas, _, err := clientP1.BuscarRotas("SALVADOR", "XIQUE XIQUE", dataViagem, tokens[0])
	if err != nil || len(rotas) == 0 {
		fmt.Printf("Erro busca rotas: %v\n", err)
		return
	}
	rota := rotas[0]

	resultados := make([]ResultadoPassageiro, numPass)
	for i := 0; i < numPass; i++ {
		resultados[i] = ResultadoPassageiro{
			Nome:           fmt.Sprintf("Passageiro %d", i+1),
			Token:          tokens[i],
			LamportInicial: 1,
		}
	}

	fmt.Println("\n[Etapa 1] Passageiro 1 dá PREPARE primeiro (ativa trava de 10 segundos)...")
	stP1, msgP1, envP1, recP1, _ := fazerRequisicaoReserva(nos[0].HTTP, "Passageiro 1", tokens[0], rota, false, false, 1)
	resultados[0].StatusHttpPrepare = stP1
	resultados[0].MsgPrepare = msgP1
	resultados[0].LamportEnvPrepare = envP1
	resultados[0].LamportRespPrepare = recP1
	fmt.Printf(" -> Passageiro 1 PREPARE OK (HTTP %d, Lamport=%d). Trava de 10s ativa!\n", stP1, recP1)

	fmt.Println("\n[Etapa 2] Durante os 10s: os outros passageiros tentam confirmar em paralelo...")
	var wg sync.WaitGroup
	for i := 1; i < numPass; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			lp := resultados[idx].LamportInicial
			stP, msgP, envP, recP, _ := fazerRequisicaoReserva(nos[0].HTTP, resultados[idx].Nome, tokens[idx], rota, false, false, lp)
			resultados[idx].StatusHttpPrepare = stP
			resultados[idx].MsgPrepare = msgP
			resultados[idx].LamportEnvPrepare = envP
			resultados[idx].LamportRespPrepare = recP
			lp = recP

			stC, msgC, envC, recC, _ := fazerRequisicaoReserva(nos[0].HTTP, resultados[idx].Nome, tokens[idx], rota, true, false, lp)
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

	fmt.Println(" -> Aguardando 2 segundos com a trava de 10s ativa (demais passageiros enfileirados no servidor)...")
	time.Sleep(2 * time.Second)

	fmt.Println("\n[Etapa 3] Depois, o Passageiro 1 confirma a viagem (COMMIT)...")
	lp1 := resultados[0].LamportRespPrepare
	stC1, msgC1, envC1, recC1, _ := fazerRequisicaoReserva(nos[0].HTTP, "Passageiro 1", tokens[0], rota, true, false, lp1)
	resultados[0].StatusHttpCommit = stC1
	resultados[0].MsgCommit = msgC1
	resultados[0].LamportEnvCommit = envC1
	resultados[0].LamportRespCommit = recC1
	resultados[0].LamportFinal = recC1
	if stC1 == http.StatusOK {
		resultados[0].GanhouCorrida = true
	}
	fmt.Printf(" -> Passageiro 1 COMMIT concluído! HTTP %d | Lamport: %d -> %d\n", stC1, envC1, recC1)

	wg.Wait()

	fmt.Println("\n================================================================================")
	fmt.Println(" TABELA DE RELÓGIOS LÓGICOS DE LAMPORT DOS PASSAGEIROS REMOTOS")
	fmt.Println("================================================================================")
	for _, p := range resultados {
		status := "RECUSADO (409 Conflict)"
		if p.GanhouCorrida {
			status = "CONFIRMADO (200 OK)"
		}
		fmt.Printf("  • %-14s : %-22s | PREPARE: L=%-3d | COMMIT: L=%-3d | Final: L=%d\n",
			p.Nome, status, p.LamportRespPrepare, p.LamportRespCommit, p.LamportFinal)
	}

	fmt.Println("\n================================================================================")
	fmt.Println(" AUDITORIA REMOTA DOS TRECHOS VIA HTTP (/status)")
	fmt.Println("================================================================================")
	for _, n := range nos {
		st, _ := obterStatusRemoto(n.HTTP)
		fmt.Printf("  • [%s] (Lamport: %d):\n", n.Name, st.Lamport)
		for id, t := range st.Trechos {
			if t.Data == dataViagem {
				fmt.Printf("    - Trecho %s (Data: %s): %d vagas restantes | Passageiros: %v\n", id, t.Data, t.Nassentos, t.IdPassageiros)
			}
		}
	}
}

func executarModoCrashInterativo(nos []NodeAddress, dataViagem string, tag int64) {
	fmt.Println("\n================================================================================")
	fmt.Println(" MODO INTERATIVO DE TOLERÂNCIA A FALHAS E WAL EM MÁQUINAS REMOTAS")
	fmt.Println("================================================================================")
	fmt.Println("Este modo permite demonstrar a queda física ou pausa de containers ao vivo!")

	pClient, err := Apiclient.Dial(nos[0].TCP, nos[0].HTTP)
	if err != nil {
		fmt.Printf("Erro ao conectar: %v\n", err)
		return
	}
	defer pClient.Close()

	pNome := fmt.Sprintf("PassCrash_%d", tag)
	pClient.RegistraPassageiro(pNome, "123")
	_, pTk, _ := pClient.AuthPassageiro(pNome, "123")

	rotas, _, _ := pClient.BuscarRotas("SALVADOR", "XIQUE XIQUE", dataViagem, pTk)
	if len(rotas) == 0 {
		fmt.Println("Nenhuma rota encontrada.")
		return
	}
	rota := rotas[0]

	fmt.Println("\n[Etapa 1] Passageiro solicita PREPARE (Trava de 10s ativada nos 3 servidores)...")
	msgP, errP := pClient.TentaReserva(rota, false, pTk)
	if errP != nil {
		fmt.Printf("Erro no prepare: %v\n", errP)
		return
	}
	fmt.Printf(" -> PREPARE OK! Resposta: %s\n", strings.TrimSpace(msgP))

	fmt.Println("\n--------------------------------------------------------------------------------")
	fmt.Printf(" >>> AÇÃO REQUERIDA NO SERVIDOR 3 (%s): <<<\n", nos[2].HTTP)
	fmt.Println("     Vá até o computador/container do Servidor 3 e derrube o servidor agora")
	fmt.Println("     (ex: Ctrl+C, docker stop jasefoi-s3, etc.)")
	fmt.Print("     Pressione ENTER assim que o Servidor 3 estiver caído >> ")
	var enter string
	fmt.Scanln(&enter)
	fmt.Println("--------------------------------------------------------------------------------")

	fmt.Println("\n[Etapa 2] Passageiro envia confirmação (COMMIT) com o Servidor 3 offline...")
	fmt.Println(" -> O Coordenador detectará a falha do Servidor 3 e tentará retries.")
	fmt.Println(" -> Se você NÃO ligar o Servidor 3: o Coordenador esgotará o retry e fará ROLLBACK via WAL.")
	fmt.Println(" -> Se você LIGAR o Servidor 3 agora: o Servidor 3 recupera o WAL e o COMMIT será concluído!")
	
	startCom := time.Now()
	msgC, errC := pClient.TentaReserva(rota, true, pTk)
	dur := time.Since(startCom)

	fmt.Printf("\n -> Resposta final recebida após %v:\n", dur)
	fmt.Printf("    Mensagem: %s\n", strings.TrimSpace(msgC))
	fmt.Printf("    Erro: %v\n", errC)

	fmt.Println("\n[Etapa 3] Auditoria nos servidores remotos ativos:")
	for _, n := range nos {
		st, err := obterStatusRemoto(n.HTTP)
		if err != nil {
			fmt.Printf("  • [%s]: OFFLINE ou inacessível (%v)\n", n.Name, err)
			continue
		}
		fmt.Printf("  • [%s] (ONLINE, Lamport: %d):\n", n.Name, st.Lamport)
		for id, t := range st.Trechos {
			if t.Data == dataViagem {
				fmt.Printf("    - Trecho %s: %d vagas restantes | Passageiros: %v\n", id, t.Nassentos, t.IdPassageiros)
			}
		}
	}
}
