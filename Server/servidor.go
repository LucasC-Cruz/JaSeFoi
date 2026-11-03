package main

import (
	"Golang/Lgcc"
	"Golang/Model"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

var serverConfig ServerConfig

type ReqBuscaRotaInter struct {
	Origem              string   `json:"origem"`
	Destino             string   `json:"destino"`
	Data                string   `json:"data"`
	ServidoresVisitados []string `json:"servidoresVisitados"`
}

// Estruturas para o protocolo Two-Phase Commit (2PC) entre servidores HTTP
type Req2PC struct {
	TxID                string         `json:"txId"`
	Passageiro          string         `json:"passageiro"`
	PassageirosAfetados []string       `json:"passageirosAfetados,omitempty"`
	Rota                []Model.Aresta `json:"rota,omitempty"`
	Trechos             []string       `json:"trechos,omitempty"`
	ServidoresVisitados []string       `json:"servidoresVisitados"`
}

// ==========================================
// WRITE-AHEAD LOG (WAL) PARA 2PC
// ==========================================

type WALStatus string

const (
	WALPrepare WALStatus = "PREPARE"
	WALCommit  WALStatus = "COMMIT"
	WALAbort   WALStatus = "ABORT"
)

type WALEntry struct {
	TxID                string         `json:"txId"`
	Status              WALStatus      `json:"status"`
	Timestamp           time.Time      `json:"timestamp"`
	Passageiro          string         `json:"passageiro"`
	Rota                []Model.Aresta `json:"rota,omitempty"`
	Trechos             []string       `json:"trechos,omitempty"`
	ArestasRemotas      []Model.Aresta `json:"arestasRemotas,omitempty"`
	ServidoresVisitados []string       `json:"servidoresVisitados,omitempty"`
	IsCoordinator       bool           `json:"isCoordinator"`
}

var (
	walMu       sync.Mutex
	walLogFile  = "wal.jsonl"
	walAtivos   = make(map[string]WALEntry)
)

func walGravarEntrada(entry WALEntry) {
	walMu.Lock()
	defer walMu.Unlock()

	entry.Timestamp = time.Now()
	if entry.Status == WALAbort {
		delete(walAtivos, entry.TxID)
	} else {
		walAtivos[entry.TxID] = entry
	}

	caminho := getDataPath(walLogFile)
	dados, err := json.Marshal(entry)
	if err == nil {
		f, errF := os.OpenFile(caminho, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if errF == nil {
			f.Write(append(dados, '\n'))
			f.Sync()
			f.Close()
		}
	}
}

func walRemoverTransacao(txID string) {
	walMu.Lock()
	defer walMu.Unlock()
	delete(walAtivos, txID)
}

func walRecuperar() {
	walMu.Lock()
	defer walMu.Unlock()

	caminho := getDataPath(walLogFile)
	dados, err := os.ReadFile(caminho)
	if err != nil {
		return
	}

	linhas := strings.Split(string(dados), "\n")
	for _, l := range linhas {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var e WALEntry
		if errU := json.Unmarshal([]byte(l), &e); errU == nil {
			if e.Status == WALAbort {
				delete(walAtivos, e.TxID)
			} else {
				walAtivos[e.TxID] = e
			}
		}
	}

	if len(walAtivos) > 0 {
		fmt.Printf("[%s] [WAL Recuperação] %d transação(ões) pendente(s) carregada(s) do log WAL:\n", serverConfig.ID, len(walAtivos))
		for txID, e := range walAtivos {
			fmt.Printf("   -> TxID=%s | Status=%s | Passageiro=%s | Trechos=%v\n", txID, e.Status, e.Passageiro, e.Trechos)
		}
	}
}

type Resp2PC struct {
	Sucesso             bool           `json:"sucesso"`
	Status              int            `json:"status"`
	ArestasRestantes    []Model.Aresta `json:"arestasRestantes,omitempty"`
	TrechosRestantes    []string       `json:"trechosRestantes,omitempty"`
	TrechosReservados   []string       `json:"trechosReservados,omitempty"`
	ServidoresVisitados []string       `json:"servidoresVisitados"`
}

// Estruturas para o algoritmo de exclusão mútua distribuída de Ricart-Agrawala
type ReqRicartAgrawala struct {
	ReqID               string   `json:"reqId"`
	ServerID            string   `json:"serverId"`
	Timestamp           int64    `json:"timestamp"`
	ServidoresVisitados []string `json:"servidoresVisitados"`
}

func getDataPath(arquivo string) string {
	return filepath.Join(serverConfig.DataDir, arquivo)
}

func carregarConfig() {
	var arg string
	if len(os.Args) >= 2 {
		arg = os.Args[1]
	} else if envConfig := os.Getenv("CONFIG_FILE"); envConfig != "" {
		arg = envConfig
	}

	if arg != "" && strings.HasSuffix(arg, ".json") {
		dados, err := os.ReadFile(arg)
		if err != nil {
			// Fallback caso seja executado de dentro da pasta Server/
			dados, err = os.ReadFile(filepath.Join("..", arg))
		}
		if err != nil {
			fmt.Printf("Erro ao ler arquivo de config %s: %v\n", arg, err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &serverConfig)
		if err != nil {
			fmt.Printf("Erro ao converter json de config: %v\n", err)
			os.Exit(1)
		}
	} else if arg != "" {
		serverConfig = ServerConfig{
			ID:           "default",
			Porta:        arg,
			DataDir:      "../data",
			VizinhosHttp: []string{},
			Cidades:      []string{},
		}
	} else {
		// Modo padrão / Docker sem arquivo nem porta como argumento CLI
		serverConfig = ServerConfig{
			ID:           "default",
			Porta:        "6741",
			PortaHttp:    "8741",
			DataDir:      "data",
			VizinhosHttp: []string{},
			Cidades:      []string{},
		}
	}

	// Permite sobrescrever campos via variáveis de ambiente (especialmente útil no Docker)
	if envID := os.Getenv("SERVER_ID"); envID != "" {
		serverConfig.ID = envID
	}
	if envPorta := os.Getenv("PORTA"); envPorta != "" {
		serverConfig.Porta = envPorta
	}
	if envPortaHttp := os.Getenv("PORTA_HTTP"); envPortaHttp != "" {
		serverConfig.PortaHttp = envPortaHttp
	}
	if envDataDir := os.Getenv("DATA_DIR"); envDataDir != "" {
		serverConfig.DataDir = envDataDir
	}
	if envVizinhosHttp := os.Getenv("VIZINHOS_HTTP"); envVizinhosHttp != "" {
		var list []string
		for _, v := range strings.Split(envVizinhosHttp, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				list = append(list, v)
			}
		}
		serverConfig.VizinhosHttp = list
	}
	if envVizinhos := os.Getenv("VIZINHOS"); envVizinhos != "" {
		var list []string
		for _, v := range strings.Split(envVizinhos, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				list = append(list, v)
			}
		}
		serverConfig.Vizinhos = list
	}
	if envCidades := os.Getenv("CIDADES"); envCidades != "" {
		var list []string
		for _, c := range strings.Split(envCidades, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				list = append(list, c)
			}
		}
		serverConfig.Cidades = list
	}
	if envCidadesInter := os.Getenv("CIDADES_INTER"); envCidadesInter != "" {
		var list []string
		for _, c := range strings.Split(envCidadesInter, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				list = append(list, c)
			}
		}
		serverConfig.CidadesInter = list
	}

	// Ajusta DataDir se executado a partir da raiz ou dentro do container Docker
	if _, err := os.Stat(serverConfig.DataDir); os.IsNotExist(err) {
		caminhoSemPontoPonto := strings.TrimPrefix(serverConfig.DataDir, "../")
		if _, err2 := os.Stat(caminhoSemPontoPonto); err2 == nil {
			serverConfig.DataDir = caminhoSemPontoPonto
		}
	}

	// Garante que o diretório de dados existe
	os.MkdirAll(serverConfig.DataDir, 0755)
}

// ==========================================
// RELÓGIO LÓGICO DE LAMPORT
// ==========================================

var lamport int64 // acesso somente via atomic

func incLamport() int64 {
	return atomic.AddInt64(&lamport, 1)
}

func getLamport() int64 {
	return atomic.LoadInt64(&lamport)
}

func updateLamport(received int64) int64 {
	for {
		cur := atomic.LoadInt64(&lamport)
		next := cur + 1
		if received >= next {
			next = received + 1
		}
		if atomic.CompareAndSwapInt64(&lamport, cur, next) {
			return next
		}
	}
}

// withLamport é um middleware HTTP que propaga o relógio lógico de Lamport.
func withLamport(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Lamport"); v != "" {
			if lv, err := strconv.ParseInt(v, 10, 64); err == nil {
				updateLamport(lv)
			}
		}
		cur := incLamport()
		w.Header().Set("X-Lamport", strconv.FormatInt(cur, 10))
		next(w, r)
	}
}

// ==========================================
// ALGORITMO DE RICART-AGRAWALA (EXCLUSÃO MÚTUA DISTRIBUÍDA)
// ==========================================

var (
	raTurnMu       sync.Mutex // Serializa goroutines locais que desejam entrar na seção crítica
	raMu           sync.Mutex
	raState        = "RELEASED" // "RELEASED", "WANTED", "HELD"
	raReqTimestamp int64
	raDeferred     []chan struct{}
	raSeenReqs     = make(map[string]time.Time)
)

// temPrioridadeRA retorna true se (tsA, idA) tem prioridade estrita sobre (tsB, idB) no relógio de Lamport
func temPrioridadeRA(tsA int64, idA string, tsB int64, idB string) bool {
	if tsA != tsB {
		return tsA < tsB
	}
	return idA < idB
}

func entrarSecaoCriticaRA() {
	raTurnMu.Lock()

	raMu.Lock()
	raState = "WANTED"
	meuTs := incLamport()
	raReqTimestamp = meuTs
	reqID := fmt.Sprintf("%s:%d:%d", serverConfig.ID, meuTs, time.Now().UnixNano())
	raSeenReqs[reqID] = time.Now()
	raMu.Unlock()

	vizinhos := obterURLsVizinhosAtivos()
	if len(vizinhos) > 0 {
		fmt.Printf("[%s] [Ricart-Agrawala] Solicitando entrada na Seção Crítica (Lamport=%d) para vizinhos %v\n", serverConfig.ID, meuTs, vizinhos)
		var wg sync.WaitGroup
		for _, vURL := range vizinhos {
			wg.Add(1)
			go func(urlDestino string) {
				defer wg.Done()
				enviarReqRA(urlDestino, ReqRicartAgrawala{
					ReqID:               reqID,
					ServerID:            serverConfig.ID,
					Timestamp:           meuTs,
					ServidoresVisitados: []string{serverConfig.ID},
				})
			}(vURL)
		}
		wg.Wait()
	}

	raMu.Lock()
	raState = "HELD"
	raMu.Unlock()
	fmt.Printf("[%s] [Ricart-Agrawala] >>> ENTROU na Seção Crítica (Lamport=%d)\n", serverConfig.ID, getLamport())
}

func sairSecaoCriticaRA() {
	raMu.Lock()
	raState = "RELEASED"
	cur := incLamport()
	deferredList := raDeferred
	raDeferred = nil
	raMu.Unlock()

	for _, ch := range deferredList {
		close(ch)
	}

	fmt.Printf("[%s] [Ricart-Agrawala] <<< SAIU da Seção Crítica (Lamport=%d, liberou %d respostas adiadas)\n", serverConfig.ID, cur, len(deferredList))
	raTurnMu.Unlock()
}

func enviarReqRA(targetURL string, reqRA ReqRicartAgrawala) {
	dados, err := json.Marshal(reqRA)
	if err != nil {
		return
	}
	endpoint := strings.TrimRight(targetURL, "/") + "/ra/request"
	client := &http.Client{Timeout: 5 * time.Second}
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(dados))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))

	resp, err := client.Do(httpReq)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if v := resp.Header.Get("X-Lamport"); v != "" {
		if lv, errL := strconv.ParseInt(v, 10, 64); errL == nil {
			updateLamport(lv)
		}
	}
}

func handleRicartAgrawalaRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}
	var reqRA ReqRicartAgrawala
	if err := json.NewDecoder(r.Body).Decode(&reqRA); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	updateLamport(reqRA.Timestamp)

	raMu.Lock()
	if _, visto := raSeenReqs[reqRA.ReqID]; visto {
		raMu.Unlock()
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"OK"}`)
		return
	}
	raSeenReqs[reqRA.ReqID] = time.Now()

	// Regra de Ricart-Agrawala: adia a resposta se este servidor está em HELD
	// ou se está em WANTED e tem prioridade menor no par (LamportTimestamp, ServerID)
	deveAdiar := false
	if reqRA.ServerID != serverConfig.ID {
		if raState == "HELD" {
			deveAdiar = true
		} else if raState == "WANTED" && temPrioridadeRA(raReqTimestamp, serverConfig.ID, reqRA.Timestamp, reqRA.ServerID) {
			deveAdiar = true
		}
	}

	var waitCh chan struct{}
	if deveAdiar {
		waitCh = make(chan struct{})
		raDeferred = append(raDeferred, waitCh)
		fmt.Printf("[%s] [Ricart-Agrawala] Adiando resposta (DEFER) para %s (Ts=%d, meuEstado=%s, meuTs=%d)\n",
			serverConfig.ID, reqRA.ServerID, reqRA.Timestamp, raState, raReqTimestamp)
	}
	raMu.Unlock()

	if deveAdiar {
		select {
		case <-waitCh:
		case <-time.After(10 * time.Second):
		}
	}

	// Propaga para vizinhos ainda não visitados (para suportar topologias em cadeia ex: S1 <-> S2 <-> S3)
	visitados := append(reqRA.ServidoresVisitados, serverConfig.ID)
	vizinhos := obterURLsVizinhosAtivosNaoVisitados(visitados)
	if len(vizinhos) > 0 {
		var wg sync.WaitGroup
		reqRepasse := reqRA
		reqRepasse.ServidoresVisitados = visitados
		for _, vURL := range vizinhos {
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				enviarReqRA(u, reqRepasse)
			}(vURL)
		}
		wg.Wait()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"OK"}`)
}

// ==========================================
// CONTROLE DE RESERVAS E TWO-PHASE COMMIT (2PC)
// ==========================================

type Pendente struct {
	TxID           string
	Passageiro     string
	Trechos        []string
	ArestasRemotas []Model.Aresta
	Confirmado     bool
	Seq            int64
	resCh          chan bool
	timer          *time.Timer
}

var seqPendente int64

func temVagaDisponivelLocked(trechos []string) bool {
	if len(trechos) == 0 {
		return true
	}
	for _, id := range trechos {
		t, ok := mapaTrechos[id]
		if !ok || t.Nassentos <= 0 {
			return false
		}
	}
	return true
}

// compartilhaTrechoOuAresta verifica se duas reservas competem por algum trecho local ou aresta remota
func compartilhaTrechoOuAresta(pend *Pendente, trechos []string, arestasRemotas []Model.Aresta) bool {
	for _, t1 := range pend.Trechos {
		for _, t2 := range trechos {
			if t1 == t2 {
				return true
			}
		}
	}
	for _, a1 := range pend.ArestasRemotas {
		for _, a2 := range arestasRemotas {
			if a1.Origem == a2.Origem && a1.Destino == a2.Destino {
				return true
			}
		}
	}
	return false
}

// existePendenteAnteriorLocked retorna true se há qualquer outro passageiro que pediu a vaga antes (Seq menor)
// nos mesmos trechos/arestas e que ainda está no período de trava (ou aguardando na fila à frente deste)
func existePendenteAnteriorLocked(meuSeq int64, trechos []string, arestasRemotas []Model.Aresta) bool {
	for _, pend := range pendentes {
		if pend.Seq < meuSeq && compartilhaTrechoOuAresta(pend, trechos, arestasRemotas) {
			return true
		}
	}
	return false
}

func pararTimerSePendente(txID string) {
	mutexMapaTrecho.Lock()
	if p, ok := pendentes[txID]; ok && p.timer != nil {
		p.timer.Stop()
	}
	mutexMapaTrecho.Unlock()
}

func efetivarReservaLocked(idPass string, trechos []string) {
	if len(trechos) > 0 {
		for _, id := range trechos {
			t, ok := mapaTrechos[id]
			if !ok {
				continue
			}
			inserido := false
			for i, idp := range t.IdPassageiros {
				if idp == "" {
					t.IdPassageiros[i] = idPass
					inserido = true
					break
				}
			}
			if !inserido {
				t.IdPassageiros = append(t.IdPassageiros, idPass)
			}
			mapaTrechos[id] = t
		}

		dados, _ := json.Marshal(mapaTrechos)
		os.WriteFile(getDataPath("trechos.json"), dados, 0644)
	}

	passageiroMu.Lock()
	if p, ok := passageiros[idPass]; ok && len(trechos) > 0 {
		p.IsTrechosPassageiros = append(p.IsTrechosPassageiros, trechos...)
		passageiros[idPass] = p
		dadosP, _ := json.Marshal(passageiros)
		os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
	}
	passageiroMu.Unlock()
}

func reverterCommitLocal(idPass string, trechos []string) {
	if len(trechos) == 0 {
		return
	}
	passageiroMu.Lock()
	mutexMapaTrecho.Lock()
	for _, id := range trechos {
		if t, ok := mapaTrechos[id]; ok {
			t.Nassentos++
			for i, idp := range t.IdPassageiros {
				if idp == idPass {
					t.IdPassageiros[i] = ""
					break
				}
			}
			mapaTrechos[id] = t
		}
	}
	dados, _ := json.Marshal(mapaTrechos)
	os.WriteFile(getDataPath("trechos.json"), dados, 0644)
	notificarFilaEsperaLocked()
	mutexMapaTrecho.Unlock()

	if p, ok := passageiros[idPass]; ok {
		var novos []string
		for _, tid := range p.IsTrechosPassageiros {
			remover := false
			for _, tr := range trechos {
				if tid == tr {
					remover = true
					break
				}
			}
			if !remover {
				novos = append(novos, tid)
			}
		}
		p.IsTrechosPassageiros = novos
		passageiros[idPass] = p
		dadosP, _ := json.Marshal(passageiros)
		os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
	}
	passageiroMu.Unlock()
}

func registrarTrechosNoPassageiro(idPass string, trechosRemotos []string) {
	if len(trechosRemotos) == 0 {
		return
	}
	passageiroMu.Lock()
	defer passageiroMu.Unlock()
	if p, ok := passageiros[idPass]; ok {
		p.IsTrechosPassageiros = append(p.IsTrechosPassageiros, trechosRemotos...)
		passageiros[idPass] = p
		dadosP, _ := json.Marshal(passageiros)
		os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
	}
}

func notificarFilaEsperaLocked() {
	// Ordena filaConfirmados por ordem estrita de chegada (Seq)
	for i := 0; i < len(filaConfirmados); i++ {
		for j := i + 1; j < len(filaConfirmados); j++ {
			pi, oki := pendentes[filaConfirmados[i]]
			pj, okj := pendentes[filaConfirmados[j]]
			if oki && okj && pj.Seq < pi.Seq {
				filaConfirmados[i], filaConfirmados[j] = filaConfirmados[j], filaConfirmados[i]
			}
		}
	}

	for {
		progrediu := false
		var novaFila []string
		for _, waitTxID := range filaConfirmados {
			p, ok := pendentes[waitTxID]
			if !ok {
				continue
			}
			if !temVagaDisponivelLocked(p.Trechos) {
				delete(pendentes, waitTxID)
				select {
				case p.resCh <- false:
				default:
				}
				progrediu = true
			} else if existePendenteAnteriorLocked(p.Seq, p.Trechos, p.ArestasRemotas) {
				// Ainda existe um passageiro anterior com trava de 10s ativa nos mesmos trechos
				novaFila = append(novaFila, waitTxID)
			} else {
				// Chegou a vez deste passageiro na fila e há vaga disponível
				for _, id := range p.Trechos {
					t := mapaTrechos[id]
					t.Nassentos--
					mapaTrechos[id] = t
				}
				efetivarReservaLocked(p.Passageiro, p.Trechos)
				delete(pendentes, waitTxID)
				select {
				case p.resCh <- true:
				default:
				}
				progrediu = true
			}
		}
		filaConfirmados = novaFila
		if !progrediu {
			break
		}
	}

	// Para passageiros não confirmados que agora assumiram a frente da fila nos seus trechos,
	// renova o timer de 10s a partir do momento em que a trava chegou neles
	for _, pend := range pendentes {
		if !pend.Confirmado && !existePendenteAnteriorLocked(pend.Seq, pend.Trechos, pend.ArestasRemotas) {
			if pend.timer != nil {
				pend.timer.Reset(10 * time.Second)
			}
		}
	}
}

// SINAL 1 (Fase 1 Prepare Local): trava os trechos por até 10 segundos
func travar(txID, idPass string, trechos []string, arestasRemotas []Model.Aresta) bool {
	mutexMapaTrecho.Lock()
	defer mutexMapaTrecho.Unlock()

	if len(trechos) == 0 && len(arestasRemotas) == 0 {
		return false
	}

	delete(expirados, txID)

	// Se já havia uma reserva pendente anterior para esta mesma sessão (txID), limpa primeiro
	if antigo, ja := pendentes[txID]; ja {
		if antigo.timer != nil {
			antigo.timer.Stop()
		}
		delete(pendentes, txID)
	}

	if !temVagaDisponivelLocked(trechos) {
		return false
	}

	seqPendente++
	meuSeq := seqPendente
	temAnterior := existePendenteAnteriorLocked(meuSeq, trechos, arestasRemotas)

	p := &Pendente{
		TxID:           txID,
		Passageiro:     idPass,
		Trechos:        trechos,
		ArestasRemotas: arestasRemotas,
		Seq:            meuSeq,
		resCh:          make(chan bool, 1),
	}
	p.timer = time.AfterFunc(10*time.Second, func() { soltarPorTimeout(txID) })
	pendentes[txID] = p

	if temAnterior {
		fmt.Printf("[%s] [2PC/Reserva HTTP] Passageiro %s (Seq=%d) entrou na fila aguardando trava anterior nos trechos %v\n", serverConfig.ID, idPass, meuSeq, trechos)
	} else {
		fmt.Printf("[%s] [2PC/Reserva HTTP] Trechos %v travados exclusivamente para o passageiro %s (Seq=%d, 10s para confirmar)\n", serverConfig.ID, trechos, idPass, meuSeq)
	}
	return true
}

// Timer de 10s estourou sem resposta do passageiro: libera a rota localmente e nos participantes remotos
func soltarPorTimeout(txID string) {
	mutexMapaTrecho.Lock()
	p, ok := pendentes[txID]
	if !ok || p.Confirmado {
		mutexMapaTrecho.Unlock()
		return
	}
	// Se ainda há outro passageiro anterior segurando a trava à frente deste, aguarda a vez dele
	if existePendenteAnteriorLocked(p.Seq, p.Trechos, p.ArestasRemotas) {
		p.timer = time.AfterFunc(10*time.Second, func() { soltarPorTimeout(txID) })
		mutexMapaTrecho.Unlock()
		return
	}

	idPass := p.Passageiro
	remotas := p.ArestasRemotas
	delete(pendentes, txID)
	expirados[txID] = true

	fmt.Printf("[%s] [2PC/Reserva HTTP] Timeout de 10s esgotado para o passageiro %s! Liberando trava dos trechos %v.\n", serverConfig.ID, idPass, p.Trechos)
	notificarFilaEsperaLocked()
	mutexMapaTrecho.Unlock()

	if len(remotas) > 0 {
		go propagar2PCAbort(txID, idPass, remotas, []string{serverConfig.ID})
	}
}

// Passageiro respondeu "Não" ou 2PC Abort: libera imediatamente a trava
func soltar(txID string) {
	mutexMapaTrecho.Lock()
	defer mutexMapaTrecho.Unlock()

	p, ok := pendentes[txID]
	if !ok {
		return
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	delete(pendentes, txID)
	expirados[txID] = true

	fmt.Printf("[%s] [2PC/Reserva HTTP] Reserva abortada/cancelada para %s. Trava dos trechos %v liberada!\n", serverConfig.ID, p.Passageiro, p.Trechos)
	notificarFilaEsperaLocked()
}

// prepararCommitLocal verifica ou enfileira a confirmação local antes de bloquear em canal (para não reter lock RA)
func prepararCommitLocal(txID, idPass string, trechosFallback []string) (status int, aguardarCh chan bool, trechosConfirmados []string) {
	mutexMapaTrecho.Lock()
	defer mutexMapaTrecho.Unlock()

	p, ok := pendentes[txID]
	if !ok {
		if expirados[txID] {
			delete(expirados, txID)
			return http.StatusGone, nil, nil // 410: expirou os 10 segundos
		}
		// Fallback caso chamada direta sem trava prévia (ex: teste de concorrência)
		if len(trechosFallback) > 0 {
			if !temVagaDisponivelLocked(trechosFallback) {
				return http.StatusConflict, nil, nil
			}
			seqPendente++
			meuSeq := seqPendente
			if existePendenteAnteriorLocked(meuSeq, trechosFallback, nil) {
				pf := &Pendente{
					TxID:       txID,
					Passageiro: idPass,
					Trechos:    trechosFallback,
					Confirmado: true,
					Seq:        meuSeq,
					resCh:      make(chan bool, 1),
				}
				pendentes[txID] = pf
				filaConfirmados = append(filaConfirmados, txID)
				return 0, pf.resCh, trechosFallback
			}
			for _, id := range trechosFallback {
				t := mapaTrechos[id]
				t.Nassentos--
				mapaTrechos[id] = t
			}
			efetivarReservaLocked(idPass, trechosFallback)
			notificarFilaEsperaLocked()
			return http.StatusOK, nil, trechosFallback
		}
		return http.StatusConflict, nil, nil
	}

	if p.timer != nil {
		p.timer.Stop()
	}

	// Se não há mais assentos disponíveis (ex: esgotados por passageiros anteriores)
	if !temVagaDisponivelLocked(p.Trechos) {
		delete(pendentes, txID)
		notificarFilaEsperaLocked()
		return http.StatusConflict, nil, nil
	}

	// Se existe um passageiro anterior ainda no período de 10s (ou na frente da fila),
	// este passageiro NÃO pode passar na frente nem acessar os dados antes do primeiro!
	if existePendenteAnteriorLocked(p.Seq, p.Trechos, p.ArestasRemotas) {
		p.Confirmado = true
		filaConfirmados = append(filaConfirmados, txID)
		fmt.Printf("[%s] [2PC/Reserva HTTP] Passageiro %s (Seq=%d) confirmou e aguarda na fila o término da trava anterior nos trechos %v...\n", serverConfig.ID, idPass, p.Seq, p.Trechos)
		return 0, p.resCh, p.Trechos
	}

	// É o primeiro da fila para estes trechos e há assento disponível: efetiva imediatamente
	for _, id := range p.Trechos {
		t := mapaTrechos[id]
		t.Nassentos--
		mapaTrechos[id] = t
	}
	trechos := p.Trechos
	delete(pendentes, txID)
	efetivarReservaLocked(p.Passageiro, trechos)
	notificarFilaEsperaLocked()
	fmt.Printf("[%s] [2PC/Reserva HTTP] Passageiro %s (Seq=%d) confirmou em até 10s! Trechos locais %v reservados.\n", serverConfig.ID, idPass, p.Seq, trechos)
	return http.StatusOK, nil, trechos
}

func aguardarFilaConfirmados(txID, idPass string, ch chan bool) int {
	select {
	case okRes := <-ch:
		if okRes {
			fmt.Printf("[%s] [2PC/Reserva HTTP] Trava anterior finalizada e vaga atribuída na ordem para o passageiro %s!\n", serverConfig.ID, idPass)
			return http.StatusOK
		}
		return http.StatusConflict
	case <-time.After(30 * time.Second):
		mutexMapaTrecho.Lock()
		delete(pendentes, txID)
		notificarFilaEsperaLocked()
		mutexMapaTrecho.Unlock()
		return http.StatusConflict
	}
}

// ==========================================
// COORDENAÇÃO E PARTICIPAÇÃO 2PC INTER-SERVIDORES
// ==========================================

// separarArestasLocaisERemotas divide as arestas da rota entre as que pertencem a motoristas deste servidor e as externas
func separarArestasLocaisERemotas(rota []Model.Aresta) (locais []Model.Aresta, remotas []Model.Aresta) {
	motoristaMu.Lock()
	defer motoristaMu.Unlock()

	for _, aresta := range rota {
		ehLocal := false
		for _, idMot := range aresta.Ids {
			if _, ok := motoristas[idMot]; ok {
				ehLocal = true
				break
			}
		}
		if ehLocal {
			locais = append(locais, aresta)
		} else {
			remotas = append(remotas, aresta)
		}
	}
	return locais, remotas
}

func selecionarTrechosDaRota(rota []Model.Aresta) []string {
	motoristaMu.Lock()
	mutexMapaTrecho.Lock()
	defer mutexMapaTrecho.Unlock()
	defer motoristaMu.Unlock()

	var trechosLocais []string
	for _, aresta := range rota {
		escolhido := ""
		fallback := ""
		for _, idMotorista := range aresta.Ids {
			m, ok := motoristas[idMotorista]
			if !ok {
				continue
			}
			for _, p := range m.Trechos {
				if p.Origem == aresta.Origem && p.Destino == aresta.Destino {
					if fallback == "" {
						fallback = p.IDtrecho
					}
					if t, existe := mapaTrechos[p.IDtrecho]; existe && t.Nassentos > 0 {
						escolhido = p.IDtrecho
						break
					}
				}
			}
			if escolhido != "" {
				break
			}
		}
		if escolhido != "" {
			trechosLocais = append(trechosLocais, escolhido)
		} else if fallback != "" {
			trechosLocais = append(trechosLocais, fallback)
		}
	}
	return trechosLocais
}

func chamarVizinho2PC(targetURL, fase string, req Req2PC) (*Resp2PC, error) {
	dados, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(targetURL, "/") + "/2pc/" + fase
	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(dados))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if v := resp.Header.Get("X-Lamport"); v != "" {
		if lv, errL := strconv.ParseInt(v, 10, 64); errL == nil {
			updateLamport(lv)
		}
	}

	var r2pc Resp2PC
	if err := json.NewDecoder(resp.Body).Decode(&r2pc); err != nil {
		return nil, err
	}
	return &r2pc, nil
}

func propagar2PCPrepare(txID, idPass string, arestasRemotas []Model.Aresta, visitados []string) (bool, []Model.Aresta, []string) {
	restantes := arestasRemotas
	vis := append([]string{}, visitados...)
	if !jaVisitouServidor(vis, serverConfig.ID) {
		vis = append(vis, serverConfig.ID)
	}

	vizinhos := obterURLsVizinhosConfiguradosNaoVisitados(vis)
	for _, vURL := range vizinhos {
		if len(restantes) == 0 {
			break
		}
		fmt.Printf("[%s] [2PC PREPARE] Enviando PREPARE de %d aresta(s) para %s\n", serverConfig.ID, len(restantes), vURL)
		resp, err := chamarVizinho2PC(vURL, "prepare", Req2PC{
			TxID:                txID,
			Passageiro:          idPass,
			Rota:                restantes,
			ServidoresVisitados: vis,
		})
		if err != nil {
			fmt.Printf("[%s] [2PC PREPARE] Erro ao contatar %s: %v\n", serverConfig.ID, vURL, err)
			continue
		}
		if len(resp.ServidoresVisitados) > 0 {
			vis = resp.ServidoresVisitados
		}
		if !resp.Sucesso {
			return false, restantes, vis
		}
		restantes = resp.ArestasRestantes
	}
	return true, restantes, vis
}

func chamarVizinho2PCComRetry(targetURL, fase string, req Req2PC, timeoutRetry time.Duration) (*Resp2PC, error) {
	deadline := time.Now().Add(timeoutRetry)
	primeiro := true
	for {
		resp, err := chamarVizinho2PC(targetURL, fase, req)
		if err == nil {
			return resp, nil
		}

		if primeiro {
			fmt.Printf("[%s] [WAL 2PC] Servidor %s indisponível durante %s: %v. Aguardando recuperação via Heartbeat (limite: %v)...\n",
				serverConfig.ID, targetURL, fase, err, timeoutRetry)
			primeiro = false
		}

		if time.Now().After(deadline) {
			fmt.Printf("[%s] [WAL 2PC] Tempo limite de espera (%v) esgotado para %s! Disparando rollback...\n",
				serverConfig.ID, timeoutRetry, targetURL)
			return nil, err
		}

		time.Sleep(1 * time.Second)
	}
}

func propagar2PCCommit(txID, idPass string, arestasRemotas []Model.Aresta, visitados []string) (bool, int, []Model.Aresta, []string, []string) {
	restantes := arestasRemotas
	var trechosRemotos []string
	vis := append([]string{}, visitados...)
	if !jaVisitouServidor(vis, serverConfig.ID) {
		vis = append(vis, serverConfig.ID)
	}

	tempoEsperaRetry := 15 * time.Second
	if val := os.Getenv("WAL_COMMIT_RETRY_TIMEOUT_SEC"); val != "" {
		if s, errS := strconv.Atoi(val); errS == nil {
			tempoEsperaRetry = time.Duration(s) * time.Second
		}
	}

	vizinhos := obterURLsVizinhosConfiguradosNaoVisitados(vis)
	for _, vURL := range vizinhos {
		if len(restantes) == 0 {
			break
		}
		fmt.Printf("[%s] [2PC COMMIT] Enviando COMMIT de %d aresta(s) para %s\n", serverConfig.ID, len(restantes), vURL)
		resp, err := chamarVizinho2PCComRetry(vURL, "commit", Req2PC{
			TxID:                txID,
			Passageiro:          idPass,
			Rota:                restantes,
			ServidoresVisitados: vis,
		}, tempoEsperaRetry)
		if err != nil {
			return false, http.StatusConflict, restantes, trechosRemotos, vis
		}
		if len(resp.ServidoresVisitados) > 0 {
			vis = resp.ServidoresVisitados
		}
		if !resp.Sucesso {
			st := resp.Status
			if st == 0 {
				st = http.StatusConflict
			}
			return false, st, restantes, trechosRemotos, vis
		}
		trechosRemotos = append(trechosRemotos, resp.TrechosReservados...)
		restantes = resp.ArestasRestantes
	}
	return true, http.StatusOK, restantes, trechosRemotos, vis
}

func propagar2PCAbort(txID, idPass string, arestasRemotas []Model.Aresta, visitados []string) []string {
	vis := append([]string{}, visitados...)
	if !jaVisitouServidor(vis, serverConfig.ID) {
		vis = append(vis, serverConfig.ID)
	}

	vizinhos := obterURLsVizinhosConfiguradosNaoVisitados(vis)
	for _, vURL := range vizinhos {
		fmt.Printf("[%s] [2PC ABORT] Enviando ABORT para %s (Passageiro %s)\n", serverConfig.ID, vURL, idPass)
		resp, err := chamarVizinho2PC(vURL, "abort", Req2PC{
			TxID:                txID,
			Passageiro:          idPass,
			Rota:                arestasRemotas,
			ServidoresVisitados: vis,
		})
		if err == nil && len(resp.ServidoresVisitados) > 0 {
			vis = resp.ServidoresVisitados
		}
	}
	return vis
}

// ==========================================
// 2PC PARA CANCELAMENTO DE PASSAGEM E CORRIDA MULTI-SERVIDOR
// ==========================================

type CancelamentoPendente struct {
	Passageiros []string
	Trechos     []string
}

var cancelamentosPendentes = make(map[string]CancelamentoPendente)

// separarTrechosLocaisERemotos divide os IDs de trechos entre os que estão neste servidor e os que estão em outros servidores HTTP
func separarTrechosLocaisERemotos(trechos []string) (locais []string, remotos []string) {
	mutexMapaTrecho.Lock()
	defer mutexMapaTrecho.Unlock()

	for _, idT := range trechos {
		if _, ok := mapaTrechos[idT]; ok {
			locais = append(locais, idT)
		} else {
			remotos = append(remotos, idT)
		}
	}
	return locais, remotos
}

func normalizarListaPassageiros(idPass string, afetados []string) []string {
	vistos := make(map[string]bool)
	var lista []string
	if idPass != "" {
		vistos[idPass] = true
		lista = append(lista, idPass)
	}
	for _, p := range afetados {
		if p != "" && !vistos[p] {
			vistos[p] = true
			lista = append(lista, p)
		}
	}
	return lista
}

// prepararCancelamentoLocal (Fase 1 Prepare do 2PC de Cancelamento) valida os trechos locais e prepara a remoção dos passageiros afetados
func prepararCancelamentoLocal(txID, idPass string, passageirosAfetados []string, trechosLocais []string) bool {
	mutexMapaTrecho.Lock()
	defer mutexMapaTrecho.Unlock()

	for _, idT := range trechosLocais {
		if _, ok := mapaTrechos[idT]; !ok {
			return false
		}
	}

	listaPass := normalizarListaPassageiros(idPass, passageirosAfetados)
	trechosSet := make(map[string]bool)
	var todosTrechosLocais []string
	for _, idT := range trechosLocais {
		if !trechosSet[idT] {
			trechosSet[idT] = true
			todosTrechosLocais = append(todosTrechosLocais, idT)
		}
	}

	// Se foram informados passageiros afetados (ex: cancelamento de corrida por motorista),
	// descobre também quaisquer outros trechos locais em que esses passageiros estejam alocados
	if len(listaPass) > 0 {
		passSet := make(map[string]bool, len(listaPass))
		for _, p := range listaPass {
			passSet[p] = true
		}
		for idT, t := range mapaTrechos {
			if trechosSet[idT] {
				continue
			}
			for _, idp := range t.IdPassageiros {
				if passSet[idp] {
					trechosSet[idT] = true
					todosTrechosLocais = append(todosTrechosLocais, idT)
					break
				}
			}
		}
	}

	cancelamentosPendentes[txID] = CancelamentoPendente{
		Passageiros: listaPass,
		Trechos:     todosTrechosLocais,
	}
	return true
}

// efetivarCancelamentoLocal (Fase 2 Commit do 2PC de Cancelamento) libera as vagas localmente, limpa os passageiros afetados, persiste no disco e notifica filas
func efetivarCancelamentoLocal(txID, idPass string, passageirosAfetados []string, trechosFallback []string) {
	passageiroMu.Lock()
	mutexMapaTrecho.Lock()

	pend, ok := cancelamentosPendentes[txID]
	delete(cancelamentosPendentes, txID)

	listaPass := normalizarListaPassageiros(idPass, passageirosAfetados)
	if ok && len(pend.Passageiros) > 0 {
		listaPass = normalizarListaPassageiros("", append(listaPass, pend.Passageiros...))
	}
	passSet := make(map[string]bool, len(listaPass))
	for _, p := range listaPass {
		passSet[p] = true
	}

	trechosAlvo := trechosFallback
	if ok && len(pend.Trechos) > 0 {
		trechosAlvo = pend.Trechos
	}

	trechosVisitados := make(map[string]bool)
	alterouTrechos := false

	removerDeTrecho := func(idTrecho string) {
		if trechosVisitados[idTrecho] {
			return
		}
		trechosVisitados[idTrecho] = true
		t, existe := mapaTrechos[idTrecho]
		if !existe {
			return
		}
		removidos := 0
		for i, idp := range t.IdPassageiros {
			if idp != "" && passSet[idp] {
				t.IdPassageiros[i] = ""
				removidos++
			}
		}
		if removidos > 0 {
			t.Nassentos += removidos
			mapaTrechos[idTrecho] = t
			alterouTrechos = true
		}
	}

	for _, idTrecho := range trechosAlvo {
		removerDeTrecho(idTrecho)
	}

	// Caso o cancelamento venha de uma corrida de motorista (PassageirosAfetados preenchido),
	// garante que qualquer trecho deste servidor que contenha o ID de algum passageiro afetado também seja limpo
	if len(passageirosAfetados) > 0 {
		for idTrecho := range mapaTrechos {
			removerDeTrecho(idTrecho)
		}
	}

	if alterouTrechos {
		dadosTrechos, _ := json.Marshal(mapaTrechos)
		os.WriteFile(getDataPath("trechos.json"), dadosTrechos, 0644)
		notificarFilaEsperaLocked()
	}
	mutexMapaTrecho.Unlock()

	// Se o registro do passageiro afetado estiver neste servidor, limpa também seu slice de trechos reservados
	alterouPassageiros := false
	for _, idP := range listaPass {
		if p, existeP := passageiros[idP]; existeP && len(p.IsTrechosPassageiros) > 0 {
			p.IsTrechosPassageiros = []string{}
			passageiros[idP] = p
			alterouPassageiros = true
		}
	}
	if alterouPassageiros {
		dadosP, _ := json.Marshal(passageiros)
		os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
	}
	passageiroMu.Unlock()
}

// abortarCancelamentoLocal (Fase 2 Abort do 2PC de Cancelamento) descarta a preparação de cancelamento
func abortarCancelamentoLocal(txID string) {
	mutexMapaTrecho.Lock()
	delete(cancelamentosPendentes, txID)
	mutexMapaTrecho.Unlock()
}

func propagar2PCCancelarPrepare(txID, idPass string, passageirosAfetados []string, trechosRemotos []string, visitados []string) (bool, []string, []string) {
	restantes := append([]string{}, trechosRemotos...)
	vis := append([]string{}, visitados...)
	if !jaVisitouServidor(vis, serverConfig.ID) {
		vis = append(vis, serverConfig.ID)
	}

	vizinhos := obterURLsVizinhosConfiguradosNaoVisitados(vis)
	for _, vURL := range vizinhos {
		if len(restantes) == 0 && len(passageirosAfetados) == 0 {
			break
		}
		fmt.Printf("[%s] [2PC CANCEL PREPARE] Enviando PREPARE de cancelamento (trechos: %d, passageiros afetados: %d) para %s\n",
			serverConfig.ID, len(restantes), len(passageirosAfetados), vURL)
		resp, err := chamarVizinho2PC(vURL, "cancelar/prepare", Req2PC{
			TxID:                txID,
			Passageiro:          idPass,
			PassageirosAfetados: passageirosAfetados,
			Trechos:             restantes,
			ServidoresVisitados: vis,
		})
		if err != nil {
			fmt.Printf("[%s] [2PC CANCEL PREPARE] Erro ao contatar %s: %v\n", serverConfig.ID, vURL, err)
			continue
		}
		if len(resp.ServidoresVisitados) > 0 {
			vis = resp.ServidoresVisitados
		}
		if !resp.Sucesso {
			return false, restantes, vis
		}
		restantes = resp.TrechosRestantes
	}
	return true, restantes, vis
}

func propagar2PCCancelarCommit(txID, idPass string, passageirosAfetados []string, trechosRemotos []string, visitados []string) (bool, int, []string, []string) {
	restantes := append([]string{}, trechosRemotos...)
	vis := append([]string{}, visitados...)
	if !jaVisitouServidor(vis, serverConfig.ID) {
		vis = append(vis, serverConfig.ID)
	}

	vizinhos := obterURLsVizinhosConfiguradosNaoVisitados(vis)
	for _, vURL := range vizinhos {
		if len(restantes) == 0 && len(passageirosAfetados) == 0 {
			break
		}
		fmt.Printf("[%s] [2PC CANCEL COMMIT] Enviando COMMIT de cancelamento (trechos: %d, passageiros afetados: %d) para %s\n",
			serverConfig.ID, len(restantes), len(passageirosAfetados), vURL)
		resp, err := chamarVizinho2PC(vURL, "cancelar/commit", Req2PC{
			TxID:                txID,
			Passageiro:          idPass,
			PassageirosAfetados: passageirosAfetados,
			Trechos:             restantes,
			ServidoresVisitados: vis,
		})
		if err != nil {
			continue
		}
		if len(resp.ServidoresVisitados) > 0 {
			vis = resp.ServidoresVisitados
		}
		if !resp.Sucesso {
			st := resp.Status
			if st == 0 {
				st = http.StatusConflict
			}
			return false, st, restantes, vis
		}
		restantes = resp.TrechosRestantes
	}
	return true, http.StatusOK, restantes, vis
}

func propagar2PCCancelarAbort(txID, idPass string, passageirosAfetados []string, trechosRemotos []string, visitados []string) []string {
	vis := append([]string{}, visitados...)
	if !jaVisitouServidor(vis, serverConfig.ID) {
		vis = append(vis, serverConfig.ID)
	}

	vizinhos := obterURLsVizinhosConfiguradosNaoVisitados(vis)
	for _, vURL := range vizinhos {
		fmt.Printf("[%s] [2PC CANCEL ABORT] Enviando ABORT de cancelamento para %s\n", serverConfig.ID, vURL)
		resp, err := chamarVizinho2PC(vURL, "cancelar/abort", Req2PC{
			TxID:                txID,
			Passageiro:          idPass,
			PassageirosAfetados: passageirosAfetados,
			Trechos:             trechosRemotos,
			ServidoresVisitados: vis,
		})
		if err == nil && len(resp.ServidoresVisitados) > 0 {
			vis = resp.ServidoresVisitados
		}
	}
	return vis
}

var (
	pendentes           map[string]*Pendente
	expirados           map[string]bool
	filaConfirmados     []string
	filaReqPass         []Model.InfoConfirmarReserva
	filaResAvisar       []Model.InfoConfirmarReserva
	telas               map[string]string
	passageiros         map[string]Model.Passageiro
	checkMotoristas     map[string]Model.InfoLoginMotorista
	checkPassageiros    map[string]Model.InfoLoginPassageiro
	motoristas          map[string]Model.Motorista
	mapaTrechos         map[string]Model.Trecho
	motoristasOnline    map[string]string
	passageirosOnline   map[string]string
	passageirosOnlineMu sync.Mutex
	grafo               *Model.Grafo

	motoristasOnlineMu    sync.Mutex
	mutexCheckMotoristas  sync.Mutex
	mutexCheckPassageiros sync.Mutex
	mutexMapaTrecho       sync.Mutex
	caronasMu             sync.Mutex
	passageiroMu          sync.Mutex
	motoristaMu           sync.Mutex
	grafoMu               sync.Mutex

	once   sync.Once
	once2  sync.Once
	once3  sync.Once
	once4  sync.Once
	once5  sync.Once
	once6  sync.Once
	once7  sync.Once
	once8  sync.Once
	once9  sync.Once
	once10 sync.Once
)

// ==========================================
// SERVIDOR E CLIENTE HTTP INTER-SERVIDORES
// ==========================================

type VizinhoHttpClient struct {
	URL        string       `json:"url"`
	ID         string       `json:"id,omitempty"`
	Conectado  bool         `json:"conectado"`
	UltimoPing time.Time    `json:"ultimoPing,omitempty"`
	Client     *http.Client `json:"-"`
}

var (
	vizinhosHttpMu  sync.RWMutex
	vizinhosHttpMap = make(map[string]*VizinhoHttpClient)
	interHttpClient = &http.Client{
		Timeout: 3 * time.Second,
	}
)

func obterURLsVizinhosAtivos() []string {
	vizinhosHttpMu.RLock()
	defer vizinhosHttpMu.RUnlock()
	var urls []string
	for _, v := range vizinhosHttpMap {
		if v.Conectado {
			urls = append(urls, v.URL)
		}
	}
	return urls
}

func obterURLsVizinhosAtivosNaoVisitados(visitados []string) []string {
	vizinhosHttpMu.RLock()
	defer vizinhosHttpMu.RUnlock()
	var urls []string
	for _, v := range vizinhosHttpMap {
		if !v.Conectado {
			continue
		}
		if v.ID != "" && jaVisitouServidor(visitados, v.ID) {
			continue
		}
		urls = append(urls, v.URL)
	}
	return urls
}

func obterURLsVizinhosConfiguradosNaoVisitados(visitados []string) []string {
	vizinhosHttpMu.RLock()
	defer vizinhosHttpMu.RUnlock()
	var urls []string
	for _, vURL := range serverConfig.VizinhosHttp {
		v, ok := vizinhosHttpMap[vURL]
		if ok && v.ID != "" && jaVisitouServidor(visitados, v.ID) {
			continue
		}
		urls = append(urls, vURL)
	}
	return urls
}

// ==========================================
// BUSCA DE ROTAS LOCAL E DISTRIBUÍDA (Cidades e CidadesInter)
// ==========================================

func normalizarNomeCidade(c string) string {
	c = strings.ReplaceAll(c, "_", " ")
	return strings.ToUpper(strings.Join(strings.Fields(c), " "))
}

func contemCidade(lista []string, cidade string) bool {
	alvo := normalizarNomeCidade(cidade)
	for _, c := range lista {
		if normalizarNomeCidade(c) == alvo {
			return true
		}
	}
	return false
}

// validarVerticesCidades verifica se todos os vértices informados pelo motorista fazem parte de serverConfig.Cidades
func validarVerticesCidades(vertices []string) (string, bool) {
	if len(serverConfig.Cidades) == 0 {
		return "", true
	}
	for _, v := range vertices {
		if !contemCidade(serverConfig.Cidades, v) {
			return v, false
		}
	}
	return "", true
}

func jaVisitouServidor(visitados []string, id string) bool {
	for _, v := range visitados {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(id)) {
			return true
		}
	}
	return false
}

func buscarRotasLocais(origem, destino, data string) [][]Model.Aresta {
	grafoMu.Lock()
	motoristaMu.Lock()
	mutexMapaTrecho.Lock()

	trechosSnapshot := make(map[string]Model.Trecho, len(mapaTrechos))
	for k, v := range mapaTrechos {
		trechosSnapshot[k] = v
	}

	rotas, err := grafo.BuscarRotas(origem, destino, data, motoristas, trechosSnapshot)
	mutexMapaTrecho.Unlock()
	motoristaMu.Unlock()
	grafoMu.Unlock()

	if err != nil {
		return nil
	}
	return rotas
}

func consultarVizinhoBusca(targetURL, origem, destino, data string, visitados []string) [][]Model.Aresta {
	reqBody := ReqBuscaRotaInter{
		Origem:              origem,
		Destino:             destino,
		Data:                data,
		ServidoresVisitados: visitados,
	}
	dados, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(targetURL, "/") + "/inter/buscar-rotas"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(dados))
	if err != nil {
		return nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))

	resp, err := interHttpClient.Do(httpReq)
	if err != nil {
		fmt.Printf("[%s] [Busca Inter-HTTP] Erro/Timeout ao consultar vizinho %s: %v\n", serverConfig.ID, targetURL, err)
		return nil
	}
	defer resp.Body.Close()

	if v := resp.Header.Get("X-Lamport"); v != "" {
		if lv, errL := strconv.ParseInt(v, 10, 64); errL == nil {
			updateLamport(lv)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var rotas [][]Model.Aresta
	if err := json.NewDecoder(resp.Body).Decode(&rotas); err != nil {
		return nil
	}
	return rotas
}

// buscarRotasDistribuidas avalia serverConfig.Cidades e serverConfig.CidadesInter
// para decidir se a busca será local ou se precisará pedir para outros servidores HTTP.
func buscarRotasDistribuidas(origem, destino, data string, visitados []string) [][]Model.Aresta {
	origem = strings.ToUpper(strings.TrimSpace(origem))
	destino = strings.ToUpper(strings.TrimSpace(destino))

	if !jaVisitouServidor(visitados, serverConfig.ID) {
		visitados = append(visitados, serverConfig.ID)
	}

	var todasRotas [][]Model.Aresta

	origemLocal := len(serverConfig.Cidades) == 0 || contemCidade(serverConfig.Cidades, origem)
	destinoLocal := len(serverConfig.Cidades) == 0 || contemCidade(serverConfig.Cidades, destino)

	// 1. Se origem e destino pertencem às cidades deste servidor, realiza busca local
	if origemLocal && destinoLocal {
		fmt.Printf("[%s] [Busca HTTP] Origem (%s) e Destino (%s) estão em Cidades locais. Buscando localmente...\n", serverConfig.ID, origem, destino)
		rotasLocais := buscarRotasLocais(origem, destino, data)
		if len(rotasLocais) > 0 {
			todasRotas = append(todasRotas, rotasLocais...)
			return todasRotas
		}
	}

	vizinhosAtivos := obterURLsVizinhosAtivosNaoVisitados(visitados)
	if len(vizinhosAtivos) == 0 {
		return todasRotas
	}

	// 2. Se a origem NÃO pertence a este servidor, repassa a busca para os servidores HTTP vizinhos
	if !origemLocal {
		fmt.Printf("[%s] [Busca HTTP] Origem (%s) não pertence a %v. Repassando busca para vizinhos HTTP: %v\n",
			serverConfig.ID, origem, serverConfig.Cidades, vizinhosAtivos)
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, vURL := range vizinhosAtivos {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				remotas := consultarVizinhoBusca(url, origem, destino, data, visitados)
				if len(remotas) > 0 {
					mu.Lock()
					todasRotas = append(todasRotas, remotas...)
					mu.Unlock()
				}
			}(vURL)
		}
		wg.Wait()
		return todasRotas
	}

	// 3. Origem é local, mas destino está em outro servidor: utiliza CidadesInter (cidades de fronteira)
	fmt.Printf("[%s] [Busca HTTP] Origem (%s) é local e Destino (%s) requer rota inter-servidor via CidadesInter %v\n",
		serverConfig.ID, origem, destino, serverConfig.CidadesInter)

	candidatasInter := append([]string{}, serverConfig.CidadesInter...)
	for _, c := range serverConfig.Cidades {
		if !contemCidade(candidatasInter, c) {
			candidatasInter = append(candidatasInter, c)
		}
	}

	for _, cInter := range candidatasInter {
		cInter = strings.ToUpper(strings.TrimSpace(cInter))
		if strings.EqualFold(cInter, destino) {
			continue
		}

		// Caso a própria cidade de origem já seja a cidade de fronteira (CidadesInter)
		if strings.EqualFold(cInter, origem) {
			var wg sync.WaitGroup
			var mu sync.Mutex
			for _, vURL := range vizinhosAtivos {
				wg.Add(1)
				go func(url string) {
					defer wg.Done()
					remotas := consultarVizinhoBusca(url, origem, destino, data, visitados)
					if len(remotas) > 0 {
						mu.Lock()
						todasRotas = append(todasRotas, remotas...)
						mu.Unlock()
					}
				}(vURL)
			}
			wg.Wait()
			continue
		}

		// Busca o trecho local da origem até a cidade de fronteira (cInter)
		rotasAteInter := buscarRotasLocais(origem, cInter, data)
		if len(rotasAteInter) == 0 {
			continue
		}

		// Pede aos vizinhos HTTP as rotas a partir de cInter até o destino final
		var wg sync.WaitGroup
		var mu sync.Mutex
		var rotasDeInterAteDestino [][]Model.Aresta

		for _, vURL := range vizinhosAtivos {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				remotas := consultarVizinhoBusca(url, cInter, destino, data, visitados)
				if len(remotas) > 0 {
					mu.Lock()
					rotasDeInterAteDestino = append(rotasDeInterAteDestino, remotas...)
					mu.Unlock()
				}
			}(vURL)
		}
		wg.Wait()

		// Concatena cada rota local (origem -> cInter) com cada rota remota (cInter -> destino)
		for _, rLocal := range rotasAteInter {
			for _, rRemota := range rotasDeInterAteDestino {
				rotaComposta := make([]Model.Aresta, 0, len(rLocal)+len(rRemota))
				rotaComposta = append(rotaComposta, rLocal...)
				rotaComposta = append(rotaComposta, rRemota...)
				todasRotas = append(todasRotas, rotaComposta)
			}
		}
	}

	return todasRotas
}

func iniciarServidorHttp() {
	if serverConfig.PortaHttp == "" {
		if len(serverConfig.Porta) >= 4 {
			serverConfig.PortaHttp = "8" + serverConfig.Porta[1:]
		} else {
			serverConfig.PortaHttp = "8080"
		}
	}

	mux := http.NewServeMux()

	// Endpoint Heartbeat
	mux.HandleFunc("/heartbeat", withLamport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":        serverConfig.ID,
			"status":    "UP",
			"porta":     serverConfig.Porta,
			"portaHttp": serverConfig.PortaHttp,
			"cidades":   serverConfig.Cidades,
			"timestamp": time.Now(),
		}
		json.NewEncoder(w).Encode(resp)
	}))

	mux.HandleFunc("/ping", withLamport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":        serverConfig.ID,
			"status":    "UP",
			"porta":     serverConfig.Porta,
			"portaHttp": serverConfig.PortaHttp,
			"cidades":   serverConfig.Cidades,
			"timestamp": time.Now(),
		}
		json.NewEncoder(w).Encode(resp)
	}))

	// Endpoint de Status Geral do Servidor e Vizinhos
	mux.HandleFunc("/status", withLamport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		vizinhosHttpMu.RLock()
		vizinhosCopia := make(map[string]*VizinhoHttpClient, len(vizinhosHttpMap))
		for k, v := range vizinhosHttpMap {
			vizinhosCopia[k] = v
		}
		vizinhosHttpMu.RUnlock()

		mutexMapaTrecho.Lock()
		trechosCopia := make(map[string]Model.Trecho, len(mapaTrechos))
		for k, v := range mapaTrechos {
			trechosCopia[k] = v
		}
		mutexMapaTrecho.Unlock()

		resp := map[string]interface{}{
			"id":       serverConfig.ID,
			"status":   "UP",
			"cidades":  serverConfig.Cidades,
			"vizinhos": vizinhosCopia,
			"lamport":  getLamport(),
			"trechos":  trechosCopia,
		}
		json.NewEncoder(w).Encode(resp)
	}))

	// Endpoint Ricart-Agrawala para exclusão mútua distribuída
	mux.HandleFunc("/ra/request", withLamport(handleRicartAgrawalaRequest))

	// Endpoint de Busca de Rotas Inter-Servidores (chamado por outros servidores HTTP)
	mux.HandleFunc("/inter/buscar-rotas", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req ReqBuscaRotaInter
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		rotas := buscarRotasDistribuidas(req.Origem, req.Destino, req.Data, req.ServidoresVisitados)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rotas)
	}))

	// ── Endpoints Participantes do Two-Phase Commit (2PC) ────────────────────

	// Fase 1 do 2PC: PREPARE (Votação e Trava dos trechos no participante)
	mux.HandleFunc("/2pc/prepare", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req Req2PC
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		txID := req.TxID
		if txID == "" {
			txID = req.Passageiro
		}

		vis := append(req.ServidoresVisitados, serverConfig.ID)
		arestasLocais, arestasRemotas := separarArestasLocaisERemotas(req.Rota)

		var trechosLocais []string
		if len(arestasLocais) > 0 {
			trechosLocais = selecionarTrechosDaRota(arestasLocais)
			if len(trechosLocais) != len(arestasLocais) || !travar(txID, req.Passageiro, trechosLocais, arestasRemotas) {
				fmt.Printf("[%s] [2PC Participant] Votou NÃO no PREPARE para %s\n", serverConfig.ID, req.Passageiro)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              http.StatusConflict,
					ArestasRestantes:    req.Rota,
					ServidoresVisitados: vis,
				})
				return
			}
			walGravarEntrada(WALEntry{
				TxID:                txID,
				Status:              WALPrepare,
				Passageiro:          req.Passageiro,
				Rota:                req.Rota,
				Trechos:             trechosLocais,
				ArestasRemotas:      arestasRemotas,
				ServidoresVisitados: vis,
				IsCoordinator:       false,
			})
			fmt.Printf("[%s] [2PC Participant] Votou SIM no PREPARE (trechos locais travados: %v)\n", serverConfig.ID, trechosLocais)
		}

		restantes := arestasRemotas
		if len(restantes) > 0 {
			okRemoto, restAposProp, visAtualizado := propagar2PCPrepare(txID, req.Passageiro, restantes, vis)
			vis = visAtualizado
			if !okRemoto || len(restAposProp) == len(restantes) {
				if len(trechosLocais) > 0 {
					soltar(txID)
		walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: req.Passageiro})
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              http.StatusConflict,
					ArestasRestantes:    restAposProp,
					ServidoresVisitados: vis,
				})
				return
			}
			restantes = restAposProp
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Resp2PC{
			Sucesso:             true,
			Status:              http.StatusOK,
			ArestasRestantes:    restantes,
			TrechosReservados:   trechosLocais,
			ServidoresVisitados: vis,
		})
	}))

	// Fase 2 do 2PC: COMMIT (Efetivação da reserva nos participantes)
	mux.HandleFunc("/2pc/commit", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req Req2PC
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		txID := req.TxID
		if txID == "" {
			txID = req.Passageiro
		}

		pararTimerSePendente(txID)

		vis := append(req.ServidoresVisitados, serverConfig.ID)
		arestasLocais, arestasRemotas := separarArestasLocaisERemotas(req.Rota)

		var trechosConfirmados []string
		if len(arestasLocais) > 0 {
			trechosFallback := selecionarTrechosDaRota(arestasLocais)
			st, chEspera, trLocais := prepararCommitLocal(txID, req.Passageiro, trechosFallback)
			if chEspera != nil {
				st = aguardarFilaConfirmados(txID, req.Passageiro, chEspera)
			}
			if st != http.StatusOK {
				walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: req.Passageiro})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(st)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              st,
					ArestasRestantes:    req.Rota,
					ServidoresVisitados: vis,
				})
				return
			}
			walGravarEntrada(WALEntry{
				TxID:                txID,
				Status:              WALCommit,
				Passageiro:          req.Passageiro,
				Rota:                req.Rota,
				Trechos:             trLocais,
				ArestasRemotas:      arestasRemotas,
				ServidoresVisitados: vis,
				IsCoordinator:       false,
			})
			trechosConfirmados = append(trechosConfirmados, trLocais...)
			fmt.Printf("[%s] [2PC Participant] COMMIT concluído com sucesso para %s (trechos: %v)\n", serverConfig.ID, req.Passageiro, trLocais)
		}

		restantes := arestasRemotas
		if len(restantes) > 0 {
			okRem, stRem, restApos, trRemotos, visAtualizado := propagar2PCCommit(txID, req.Passageiro, restantes, vis)
			vis = visAtualizado
			if !okRem || len(restApos) == len(restantes) {
				if stRem == 0 || stRem == http.StatusOK {
					stRem = http.StatusConflict
				}
				walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: req.Passageiro})
				if len(trechosConfirmados) > 0 {
					fmt.Printf("[%s] [2PC Participant] Falha ao propagar COMMIT para servidores seguintes após retries (restantes: %d). Revertendo commit local dos trechos %v...\n",
						serverConfig.ID, len(restApos), trechosConfirmados)
					reverterCommitLocal(req.Passageiro, trechosConfirmados)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(stRem)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              stRem,
					ArestasRestantes:    restApos,
					ServidoresVisitados: vis,
				})
				return
			}
			trechosConfirmados = append(trechosConfirmados, trRemotos...)
			restantes = restApos
		}

		walRemoverTransacao(txID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Resp2PC{
			Sucesso:             true,
			Status:              http.StatusOK,
			ArestasRestantes:    restantes,
			TrechosReservados:   trechosConfirmados,
			ServidoresVisitados: vis,
		})
	}))

	// Fase 2 do 2PC: ABORT (Liberação imediata das travas nos participantes)
	mux.HandleFunc("/2pc/abort", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req Req2PC
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		txID := req.TxID
		if txID == "" {
			txID = req.Passageiro
		}

		soltar(txID)
		walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: req.Passageiro})
		if req.Passageiro != "" && len(req.Rota) > 0 {
			arestasLocais, _ := separarArestasLocaisERemotas(req.Rota)
			if len(arestasLocais) > 0 {
				var idsParaReverter []string
				mutexMapaTrecho.Lock()
				for idT, t := range mapaTrechos {
					for _, idp := range t.IdPassageiros {
						if idp == req.Passageiro {
							idsParaReverter = append(idsParaReverter, idT)
							break
						}
					}
				}
				mutexMapaTrecho.Unlock()
				if len(idsParaReverter) > 0 {
					reverterCommitLocal(req.Passageiro, idsParaReverter)
				}
			}
		}
		vis := propagar2PCAbort(txID, req.Passageiro, req.Rota, req.ServidoresVisitados)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Resp2PC{
			Sucesso:             true,
			Status:              http.StatusOK,
			ServidoresVisitados: vis,
		})
	}))

	// ── Endpoints Participantes do 2PC de Cancelamento de Passagem e Corrida ─

	// Fase 1 do 2PC de Cancelamento: PREPARE
	mux.HandleFunc("/2pc/cancelar/prepare", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req Req2PC
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		txID := req.TxID
		if txID == "" {
			txID = req.Passageiro
		}

		vis := append(req.ServidoresVisitados, serverConfig.ID)
		trechosLocais, trechosRemotos := separarTrechosLocaisERemotos(req.Trechos)

		if len(trechosLocais) > 0 || len(req.PassageirosAfetados) > 0 {
			if !prepararCancelamentoLocal(txID, req.Passageiro, req.PassageirosAfetados, trechosLocais) {
				fmt.Printf("[%s] [2PC Cancel Participant] Votou NÃO no PREPARE de cancelamento (txID: %s)\n", serverConfig.ID, txID)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              http.StatusConflict,
					TrechosRestantes:    req.Trechos,
					ServidoresVisitados: vis,
				})
				return
			}
			fmt.Printf("[%s] [2PC Cancel Participant] Votou SIM no PREPARE de cancelamento (trechos locais: %v, passageiros afetados: %v)\n",
				serverConfig.ID, trechosLocais, req.PassageirosAfetados)
		}

		restantes := trechosRemotos
		if len(restantes) > 0 || len(req.PassageirosAfetados) > 0 {
			okRemoto, restAposProp, visAtualizado := propagar2PCCancelarPrepare(txID, req.Passageiro, req.PassageirosAfetados, restantes, vis)
			vis = visAtualizado
			if !okRemoto {
				abortarCancelamentoLocal(txID)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              http.StatusConflict,
					TrechosRestantes:    restAposProp,
					ServidoresVisitados: vis,
				})
				return
			}
			restantes = restAposProp
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Resp2PC{
			Sucesso:             true,
			Status:              http.StatusOK,
			TrechosRestantes:    restantes,
			ServidoresVisitados: vis,
		})
	}))

	// Fase 2 do 2PC de Cancelamento: COMMIT
	mux.HandleFunc("/2pc/cancelar/commit", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req Req2PC
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		txID := req.TxID
		if txID == "" {
			txID = req.Passageiro
		}

		vis := append(req.ServidoresVisitados, serverConfig.ID)
		trechosLocais, trechosRemotos := separarTrechosLocaisERemotos(req.Trechos)

		if len(trechosLocais) > 0 || len(req.PassageirosAfetados) > 0 {
			efetivarCancelamentoLocal(txID, req.Passageiro, req.PassageirosAfetados, trechosLocais)
			fmt.Printf("[%s] [2PC Cancel Participant] COMMIT de cancelamento concluído (txID: %s, trechos locais: %v, passageiros afetados: %v)\n",
				serverConfig.ID, txID, trechosLocais, req.PassageirosAfetados)
		}

		restantes := trechosRemotos
		if len(restantes) > 0 || len(req.PassageirosAfetados) > 0 {
			okRem, stRem, restApos, visAtualizado := propagar2PCCancelarCommit(txID, req.Passageiro, req.PassageirosAfetados, restantes, vis)
			vis = visAtualizado
			if !okRem {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(stRem)
				json.NewEncoder(w).Encode(Resp2PC{
					Sucesso:             false,
					Status:              stRem,
					TrechosRestantes:    restApos,
					ServidoresVisitados: vis,
				})
				return
			}
			restantes = restApos
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Resp2PC{
			Sucesso:             true,
			Status:              http.StatusOK,
			TrechosRestantes:    restantes,
			ServidoresVisitados: vis,
		})
	}))

	// Fase 2 do 2PC de Cancelamento: ABORT
	mux.HandleFunc("/2pc/cancelar/abort", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		var req Req2PC
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		txID := req.TxID
		if txID == "" {
			txID = req.Passageiro
		}

		abortarCancelamentoLocal(txID)
		vis := propagar2PCCancelarAbort(txID, req.Passageiro, req.PassageirosAfetados, req.Trechos, req.ServidoresVisitados)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Resp2PC{
			Sucesso:             true,
			Status:              http.StatusOK,
			ServidoresVisitados: vis,
		})
	}))

	// ── Endpoints para o cliente (passageiro) via HTTP (ou via intermediário TCP) ──

	// GET /passageiro/buscar-rotas?origem=X&destino=Y&data=Z
	// Header: Authorization: Bearer <tk>
	mux.HandleFunc("/passageiro/buscar-rotas", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		tk := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		passageirosOnlineMu.Lock()
		_, autenticadoPass := passageirosOnline[tk]
		passageirosOnlineMu.Unlock()
		motoristasOnlineMu.Lock()
		_, autenticadoMot := motoristasOnline[tk]
		motoristasOnlineMu.Unlock()
		if !autenticadoPass && !autenticadoMot {
			http.Error(w, "Não autorizado", http.StatusUnauthorized)
			return
		}
		origem := r.URL.Query().Get("origem")
		destino := r.URL.Query().Get("destino")
		data := r.URL.Query().Get("data")
		if origem == "" || destino == "" {
			http.Error(w, "Parâmetros origem e destino são obrigatórios", http.StatusBadRequest)
			return
		}
		rotas := buscarRotasDistribuidas(origem, destino, data, []string{serverConfig.ID})
		if len(rotas) == 0 {
			http.Error(w, "Nenhuma rota encontrada", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rotas)
	}))

	// POST /passageiro/tentar-reserva
	// Header: Authorization: Bearer <tk>
	// Body: Model.InfoConfirmarReserva
	// Coordena a reserva usando Ricart-Agrawala (exclusão mútua distribuída) + 2PC (Two-Phase Commit)
	mux.HandleFunc("/passageiro/tentar-reserva", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		tk := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		passageirosOnlineMu.Lock()
		idPassageiro, autenticado := passageirosOnline[tk]
		passageirosOnlineMu.Unlock()
		if !autenticado || idPassageiro == "" {
			http.Error(w, "Não autorizado", http.StatusUnauthorized)
			return
		}
		var req Model.InfoConfirmarReserva
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}

		txID := tk
		if txID == "" {
			txID = idPassageiro
		}

		w.Header().Set("Content-Type", "application/json")
		arestasLocais, arestasRemotas := separarArestasLocaisERemotas(req.Rota)

		// 1. Caso o passageiro tenha respondido "Não" na confirmação: 2PC Fase 2 (ABORT)
		if req.Cancelar {
			pararTimerSePendente(txID)
			entrarSecaoCriticaRA()
			soltar(txID)
			walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: idPassageiro})
			sairSecaoCriticaRA()

			if len(arestasRemotas) > 0 {
				propagar2PCAbort(txID, idPassageiro, arestasRemotas, []string{serverConfig.ID})
			}

			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"cancelado"}`)
			return
		}

		if len(req.Rota) == 0 {
			http.Error(w, "Rota vazia", http.StatusBadRequest)
			return
		}

		trechosLocais := selecionarTrechosDaRota(arestasLocais)

		// 2. Caso o passageiro tenha acabado de escolher a rota: 2PC Fase 1 (PREPARE / Trava por 10s)
		if !req.Confirma {
			entrarSecaoCriticaRA()

			if len(arestasLocais) > 0 && len(trechosLocais) != len(arestasLocais) {
				sairSecaoCriticaRA()
				http.Error(w, `{"erro":"assentos indisponíveis"}`, http.StatusConflict)
				return
			}

			if !travar(txID, idPassageiro, trechosLocais, arestasRemotas) {
				sairSecaoCriticaRA()
				http.Error(w, `{"erro":"assentos indisponíveis"}`, http.StatusConflict)
				return
			}

			// Grava no WAL o estado PREPARE
			walGravarEntrada(WALEntry{
				TxID:                txID,
				Status:              WALPrepare,
				Passageiro:          idPassageiro,
				Rota:                req.Rota,
				Trechos:             trechosLocais,
				ArestasRemotas:      arestasRemotas,
				ServidoresVisitados: []string{serverConfig.ID},
				IsCoordinator:       true,
			})

			if len(arestasRemotas) > 0 {
				fmt.Printf("[%s] [2PC Coordinator] Iniciando Fase 1 (PREPARE) para %d aresta(s) remota(s)\n", serverConfig.ID, len(arestasRemotas))
				okRemoto, restantes, _ := propagar2PCPrepare(txID, idPassageiro, arestasRemotas, []string{serverConfig.ID})
				if !okRemoto || len(restantes) > 0 {
					fmt.Printf("[%s] [2PC Coordinator] Falha no PREPARE remoto! Executando ABORT global...\n", serverConfig.ID)
					soltar(txID)
					sairSecaoCriticaRA()
					walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: idPassageiro})
					propagar2PCAbort(txID, idPassageiro, arestasRemotas, []string{serverConfig.ID})
					http.Error(w, `{"erro":"assentos indisponíveis em servidor remoto"}`, http.StatusConflict)
					return
				}
				fmt.Printf("[%s] [2PC Coordinator] Fase 1 (PREPARE) concluída com sucesso em todos os servidores!\n", serverConfig.ID)
			}

			sairSecaoCriticaRA()
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"travado"}`)
			return
		}

		// 3. Caso o passageiro tenha respondido "Sim": 2PC Fase 2 (COMMIT)
		// Para o timer de 10s imediatamente ao receber a resposta dentro do prazo
		pararTimerSePendente(txID)

		// Grava no WAL o início do COMMIT
		walGravarEntrada(WALEntry{
			TxID:                txID,
			Status:              WALCommit,
			Passageiro:          idPassageiro,
			Rota:                req.Rota,
			Trechos:             trechosLocais,
			ArestasRemotas:      arestasRemotas,
			ServidoresVisitados: []string{serverConfig.ID},
			IsCoordinator:       true,
		})

		entrarSecaoCriticaRA()
		statusLocal, chEspera, trLocaisConfirmados := prepararCommitLocal(txID, idPassageiro, trechosLocais)
		sairSecaoCriticaRA()

		if chEspera != nil {
			// Aguarda fora da seção crítica de Ricart-Agrawala para não bloquear o passageiro da frente
			statusLocal = aguardarFilaConfirmados(txID, idPassageiro, chEspera)
		}

		if statusLocal != http.StatusOK {
			walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: idPassageiro})
			if len(arestasRemotas) > 0 {
				propagar2PCAbort(txID, idPassageiro, arestasRemotas, []string{serverConfig.ID})
			}
			if statusLocal == http.StatusGone {
				http.Error(w, `{"erro":"timeout de 10s expirado"}`, http.StatusGone)
			} else {
				http.Error(w, `{"erro":"assentos indisponíveis"}`, http.StatusConflict)
			}
			return
		}

		if len(arestasRemotas) > 0 {
			if os.Getenv("TEST_HOOK_AFTER_LOCAL_COMMIT") == "true" {
				time.Sleep(1500 * time.Millisecond)
			}
			fmt.Printf("[%s] [2PC Coordinator] Iniciando Fase 2 (COMMIT) para %d aresta(s) remota(s)\n", serverConfig.ID, len(arestasRemotas))
			okRem, stRem, restantes, trechosRemotos, _ := propagar2PCCommit(txID, idPassageiro, arestasRemotas, []string{serverConfig.ID})
			if !okRem || len(restantes) > 0 {
				fmt.Printf("[%s] [2PC Coordinator] Falha no COMMIT remoto após retries! Executando ABORT e Rollback...\n", serverConfig.ID)
				walGravarEntrada(WALEntry{TxID: txID, Status: WALAbort, Passageiro: idPassageiro})
				if len(trLocaisConfirmados) > 0 {
					reverterCommitLocal(idPassageiro, trLocaisConfirmados)
				}
				propagar2PCAbort(txID, idPassageiro, arestasRemotas, []string{serverConfig.ID})
				if stRem == http.StatusGone {
					http.Error(w, `{"erro":"timeout de 10s expirado"}`, http.StatusGone)
				} else {
					http.Error(w, `{"erro":"falha ao confirmar em servidor remoto"}`, http.StatusConflict)
				}
				return
			}
			registrarTrechosNoPassageiro(idPassageiro, trechosRemotos)
			fmt.Printf("[%s] [2PC Coordinator] Fase 2 (COMMIT) concluída com sucesso em todos os servidores!\n", serverConfig.ID)
		}

		walRemoverTransacao(txID)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"confirmado"}`)
	}))

	// POST /passageiro/cancelar-passagem
	// Header: Authorization: Bearer <tk>
	// Coordena o cancelamento da passagem em múltiplos servidores usando Ricart-Agrawala + 2PC
	mux.HandleFunc("/passageiro/cancelar-passagem", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		tk := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		passageirosOnlineMu.Lock()
		idPassageiro, autenticado := passageirosOnline[tk]
		passageirosOnlineMu.Unlock()
		if !autenticado || idPassageiro == "" {
			http.Error(w, "Não autorizado", http.StatusUnauthorized)
			return
		}

		passageiroMu.Lock()
		p, existePassageiro := passageiros[idPassageiro]
		if !existePassageiro || len(p.IsTrechosPassageiros) == 0 {
			passageiroMu.Unlock()
			fmt.Printf("[%s] Passageiro %s não possui passagens para cancelar\n", serverConfig.ID, idPassageiro)
			http.Error(w, `{"erro":"nenhuma passagem ativa"}`, http.StatusNotFound)
			return
		}
		trechosParaLiberar := append([]string{}, p.IsTrechosPassageiros...)
		passageiroMu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		entrarSecaoCriticaRA()
		defer sairSecaoCriticaRA()

		trechosLocais, trechosRemotos := separarTrechosLocaisERemotos(trechosParaLiberar)
		txID := fmt.Sprintf("cancel-%s-%d", idPassageiro, incLamport())

		// Fase 1 do 2PC (PREPARE): verifica se todos os servidores participantes podem cancelar os trechos
		if len(trechosLocais) > 0 {
			if !prepararCancelamentoLocal(txID, idPassageiro, nil, trechosLocais) {
				http.Error(w, `{"erro":"falha ao preparar cancelamento local"}`, http.StatusConflict)
				return
			}
		}

		if len(trechosRemotos) > 0 {
			fmt.Printf("[%s] [2PC Cancel Coordinator] Iniciando Fase 1 (PREPARE) para cancelar %d trecho(s) remoto(s) do passageiro %s\n",
				serverConfig.ID, len(trechosRemotos), idPassageiro)
			okRemoto, restantes, _ := propagar2PCCancelarPrepare(txID, idPassageiro, nil, trechosRemotos, []string{serverConfig.ID})
			if !okRemoto || len(restantes) > 0 {
				fmt.Printf("[%s] [2PC Cancel Coordinator] Falha no PREPARE remoto de cancelamento (restantes: %v)! Executando ABORT global...\n",
					serverConfig.ID, restantes)
				abortarCancelamentoLocal(txID)
				propagar2PCCancelarAbort(txID, idPassageiro, nil, trechosRemotos, []string{serverConfig.ID})
				http.Error(w, `{"erro":"falha ao preparar cancelamento nos servidores remotos"}`, http.StatusConflict)
				return
			}
			fmt.Printf("[%s] [2PC Cancel Coordinator] Fase 1 (PREPARE) de cancelamento concluída com sucesso em todos os servidores!\n", serverConfig.ID)
		}

		// Fase 2 do 2PC (COMMIT): efetiva o cancelamento localmente e em todos os servidores participantes
		if len(trechosLocais) > 0 {
			efetivarCancelamentoLocal(txID, idPassageiro, nil, trechosLocais)
		}

		if len(trechosRemotos) > 0 {
			fmt.Printf("[%s] [2PC Cancel Coordinator] Iniciando Fase 2 (COMMIT) para cancelar %d trecho(s) remoto(s)\n",
				serverConfig.ID, len(trechosRemotos))
			okCommit, _, restCommit, _ := propagar2PCCancelarCommit(txID, idPassageiro, nil, trechosRemotos, []string{serverConfig.ID})
			if !okCommit || len(restCommit) > 0 {
				fmt.Printf("[%s] [2PC Cancel Coordinator] Erro no COMMIT remoto de cancelamento (restantes: %v)\n", serverConfig.ID, restCommit)
				http.Error(w, `{"erro":"falha ao confirmar cancelamento em servidor remoto"}`, http.StatusConflict)
				return
			}
			fmt.Printf("[%s] [2PC Cancel Coordinator] Fase 2 (COMMIT) de cancelamento concluída com sucesso em todos os servidores!\n", serverConfig.ID)
		}

		passageiroMu.Lock()
		if pAtual, ok := passageiros[idPassageiro]; ok {
			pAtual.IsTrechosPassageiros = []string{}
			passageiros[idPassageiro] = pAtual
			dadosP, _ := json.Marshal(passageiros)
			os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
		}
		passageiroMu.Unlock()

		fmt.Printf("[%s] Passagem cancelada com sucesso (local + remoto via 2PC) para o passageiro %s!\n", serverConfig.ID, idPassageiro)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"cancelado"}`)
	}))

	// POST /motorista/cancelar-corrida
	// Header: Authorization: Bearer <tk>
	// Coordena o cancelamento da corrida do motorista em múltiplos servidores usando Ricart-Agrawala + 2PC,
	// removendo os passageiros afetados tanto dos trechos locais quanto dos trechos de outros servidores HTTP.
	mux.HandleFunc("/motorista/cancelar-corrida", withLamport(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		tk := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		motoristasOnlineMu.Lock()
		idMotorista, online := motoristasOnline[tk]
		motoristasOnlineMu.Unlock()
		if !online || idMotorista == "" {
			http.Error(w, "Não autorizado", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		entrarSecaoCriticaRA()
		defer sairSecaoCriticaRA()

		motoristaMu.Lock()
		motorista, existe := motoristas[idMotorista]
		if !existe || len(motorista.Trechos) == 0 {
			motoristaMu.Unlock()
			fmt.Printf("[%s] Motorista %s não tem corrida ativa para cancelar\n", serverConfig.ID, idMotorista)
			http.Error(w, `{"erro":"nenhuma corrida ativa"}`, http.StatusNotFound)
			return
		}
		pairTrechos := append([]Model.PairTrecho{}, motorista.Trechos...)
		motoristaMu.Unlock()

		// 1. Identifica os passageiros atendidos pelos trechos deste motorista e eventuais trechos remotos conhecidos
		passageiroMu.Lock()
		mutexMapaTrecho.Lock()

		trechosDoMotorista := make(map[string]bool, len(pairTrechos))
		passageirosAfetadosMap := make(map[string]bool)
		var passageirosAfetados []string

		for _, pair := range pairTrechos {
			trechosDoMotorista[pair.IDtrecho] = true
			if trecho, existeTrecho := mapaTrechos[pair.IDtrecho]; existeTrecho {
				for _, idPass := range trecho.IdPassageiros {
					if idPass != "" && !passageirosAfetadosMap[idPass] {
						passageirosAfetadosMap[idPass] = true
						passageirosAfetados = append(passageirosAfetados, idPass)
					}
				}
			}
		}

		trechosRemotosSet := make(map[string]bool)
		var trechosRemotos []string
		for _, idPass := range passageirosAfetados {
			if p, ok := passageiros[idPass]; ok {
				for _, idT := range p.IsTrechosPassageiros {
					if _, ehLocal := mapaTrechos[idT]; !ehLocal && !trechosRemotosSet[idT] {
						trechosRemotosSet[idT] = true
						trechosRemotos = append(trechosRemotos, idT)
					}
				}
			}
		}

		mutexMapaTrecho.Unlock()
		passageiroMu.Unlock()

		txID := fmt.Sprintf("cancel-mot-%s-%d", idMotorista, incLamport())

		// 2. Se há passageiros afetados na corrida do motorista, coordena o 2PC (PREPARE -> COMMIT/ABORT)
		// com todos os servidores HTTP vizinhos para remover o ID desses passageiros dos trechos de outros servidores
		if len(passageirosAfetados) > 0 {
			fmt.Printf("[%s] [2PC Driver Cancel Coordinator] Iniciando Fase 1 (PREPARE) para remover %d passageiro(s) afetado(s) (%v) dos trechos remotos (%v)\n",
				serverConfig.ID, len(passageirosAfetados), passageirosAfetados, trechosRemotos)

			okRemoto, restantes, _ := propagar2PCCancelarPrepare(txID, "", passageirosAfetados, trechosRemotos, []string{serverConfig.ID})
			if !okRemoto || len(restantes) > 0 {
				fmt.Printf("[%s] [2PC Driver Cancel Coordinator] Falha no PREPARE remoto (restantes: %v)! Executando ABORT global...\n",
					serverConfig.ID, restantes)
				propagar2PCCancelarAbort(txID, "", passageirosAfetados, trechosRemotos, []string{serverConfig.ID})
				http.Error(w, `{"erro":"falha ao preparar cancelamento dos passageiros nos servidores remotos"}`, http.StatusConflict)
				return
			}
			fmt.Printf("[%s] [2PC Driver Cancel Coordinator] Fase 1 (PREPARE) concluída com sucesso!\n", serverConfig.ID)

			fmt.Printf("[%s] [2PC Driver Cancel Coordinator] Iniciando Fase 2 (COMMIT) para remover %d passageiro(s) afetado(s) dos servidores remotos\n",
				serverConfig.ID, len(passageirosAfetados))
			okCommit, _, restCommit, _ := propagar2PCCancelarCommit(txID, "", passageirosAfetados, trechosRemotos, []string{serverConfig.ID})
			if !okCommit || len(restCommit) > 0 {
				fmt.Printf("[%s] [2PC Driver Cancel Coordinator] Erro no COMMIT remoto (restantes: %v)\n", serverConfig.ID, restCommit)
				http.Error(w, `{"erro":"falha ao confirmar cancelamento dos passageiros em servidor remoto"}`, http.StatusConflict)
				return
			}
			fmt.Printf("[%s] [2PC Driver Cancel Coordinator] Fase 2 (COMMIT) concluída com sucesso em todos os servidores!\n", serverConfig.ID)
		}

		// 3. Efetiva as alterações locais (remove passageiros de outros trechos locais, exclui trechos do motorista, limpa Passageiro.json, grafo.json e Motoristas.json)
		motoristaMu.Lock()
		passageiroMu.Lock()
		mutexMapaTrecho.Lock()

		for idT, tPass := range mapaTrechos {
			if trechosDoMotorista[idT] {
				continue
			}
			removidos := 0
			for i, idp := range tPass.IdPassageiros {
				if idp != "" && passageirosAfetadosMap[idp] {
					tPass.IdPassageiros[i] = ""
					removidos++
				}
			}
			if removidos > 0 {
				tPass.Nassentos += removidos
				mapaTrechos[idT] = tPass
			}
		}

		for _, idPass := range passageirosAfetados {
			if p, ok := passageiros[idPass]; ok {
				p.IsTrechosPassageiros = []string{}
				passageiros[idPass] = p
			}
		}

		for _, pair := range pairTrechos {
			delete(mapaTrechos, pair.IDtrecho)
		}

		dadosT, _ := json.Marshal(mapaTrechos)
		os.WriteFile(getDataPath("trechos.json"), dadosT, 0644)
		notificarFilaEsperaLocked()
		mutexMapaTrecho.Unlock()

		dadosP, _ := json.Marshal(passageiros)
		os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
		passageiroMu.Unlock()

		grafoMu.Lock()
		for _, pair := range pairTrechos {
			grafo.RmvIdTrecho(pair.Origem, pair.Destino, idMotorista)
		}
		dadosGrafo, _ := json.Marshal(grafo)
		os.WriteFile(getDataPath("grafo.json"), dadosGrafo, 0644)
		grafoMu.Unlock()

		motorista.Trechos = []Model.PairTrecho{}
		motoristas[idMotorista] = motorista
		dadosM, _ := json.Marshal(motoristas)
		os.WriteFile(getDataPath("Motoristas.json"), dadosM, 0644)
		motoristaMu.Unlock()

		fmt.Printf("[%s] Corrida do motorista %s cancelada com sucesso via 2PC! Trechos removidos: %d, Passageiros afetados: %d\n",
			serverConfig.ID, idMotorista, len(pairTrechos), len(passageirosAfetados))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"cancelado"}`)
	}))

	portaHttp := fmt.Sprintf(":%s", serverConfig.PortaHttp)
	listenerHttp, err := net.Listen("tcp", portaHttp)
	if err != nil {
		fmt.Printf("[%s] ERRO ao criar ouvinte HTTP na porta %s: %v\n", serverConfig.ID, portaHttp, err)
		return
	}

	fmt.Printf("[%s] Servidor HTTP Inter-Servidor escutando na porta %s\n", serverConfig.ID, portaHttp)
	go func() {
		if err := http.Serve(listenerHttp, mux); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[%s] Erro na execução do servidor HTTP: %v\n", serverConfig.ID, err)
		}
	}()
}

func conectarVizinhosHttp() {
	if len(serverConfig.VizinhosHttp) == 0 {
		fmt.Printf("[%s] [HTTP Cliente] Nenhum vizinho HTTP configurado no slice.\n", serverConfig.ID)
		return
	}

	fmt.Printf("[%s] [HTTP Cliente] Iniciando conexão cliente com vizinhos: %v (timeout de 15s)\n", serverConfig.ID, serverConfig.VizinhosHttp)
	for _, vURL := range serverConfig.VizinhosHttp {
		go tentarConectarVizinho(vURL, 15*time.Second)
	}
}

func tentarConectarVizinho(vizinhoURL string, timeout time.Duration) {
	fmt.Printf("[%s] [HTTP Cliente] Tentando conectar ao vizinho %s (timeout: %v)...\n", serverConfig.ID, vizinhoURL, timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	endpoint := strings.TrimRight(vizinhoURL, "/") + "/ping"

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			req.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))
			resp, errDo := client.Do(req)
			if errDo == nil {
				if v := resp.Header.Get("X-Lamport"); v != "" {
					if lv, err := strconv.ParseInt(v, 10, 64); err == nil {
						updateLamport(lv)
					}
				}
				var info struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				}
				json.NewDecoder(resp.Body).Decode(&info)
				resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					vizinhosHttpMu.Lock()
					vizinhosHttpMap[vizinhoURL] = &VizinhoHttpClient{
						URL:        vizinhoURL,
						ID:         info.ID,
						Conectado:  true,
						UltimoPing: time.Now(),
						Client:     client,
					}
					vizinhosHttpMu.Unlock()
					fmt.Printf("[%s] [HTTP Cliente] -> Conexão estabelecida com sucesso com o vizinho %s (ID: %s)!\n", serverConfig.ID, vizinhoURL, info.ID)
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			vizinhosHttpMu.Lock()
			vizinhosHttpMap[vizinhoURL] = &VizinhoHttpClient{
				URL:       vizinhoURL,
				Conectado: false,
				Client:    client,
			}
			vizinhosHttpMu.Unlock()
			fmt.Printf("[%s] [HTTP Cliente] -> Timeout de %v atingido: não foi possível conectar ao vizinho %s (OFFLINE)\n", serverConfig.ID, timeout, vizinhoURL)
			return
		case <-ticker.C:
			// aguarda proximo segundo para tentar reconectar até o timeout
		}
	}
}

func iniciarHeartbeat() {
	if len(serverConfig.VizinhosHttp) == 0 {
		return
	}

	go func() {
		// Cliente HTTP com timeout estrito de 5s: se não responder em 5s é considerado morto
		hbClient := &http.Client{
			Timeout: 5 * time.Second,
		}

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			for _, vURL := range serverConfig.VizinhosHttp {
				go func(targetURL string) {
					endpoint := strings.TrimRight(targetURL, "/") + "/heartbeat"
					req, err := http.NewRequest(http.MethodGet, endpoint, nil)
					var resp *http.Response
					if err == nil {
						req.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))
						resp, err = hbClient.Do(req)
					}
					if err != nil {
						endpoint = strings.TrimRight(targetURL, "/") + "/ping"
						req, err = http.NewRequest(http.MethodGet, endpoint, nil)
						if err == nil {
							req.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))
							resp, err = hbClient.Do(req)
						}
					}

					ativo := false
					var infoID string
					if err == nil && resp != nil {
						if v := resp.Header.Get("X-Lamport"); v != "" {
							if lv, errL := strconv.ParseInt(v, 10, 64); errL == nil {
								updateLamport(lv)
							}
						}
						if resp.StatusCode == http.StatusOK {
							ativo = true
							var info struct {
								ID string `json:"id"`
							}
							json.NewDecoder(resp.Body).Decode(&info)
							infoID = info.ID
						}
						resp.Body.Close()
					}

					vizinhosHttpMu.Lock()
					v, existe := vizinhosHttpMap[targetURL]
					if !existe {
						v = &VizinhoHttpClient{
							URL:       targetURL,
							Conectado: false,
							Client:    hbClient,
						}
						vizinhosHttpMap[targetURL] = v
					}

					// Detecta transição de estado
					if v.Conectado != ativo {
						if ativo {
							fmt.Printf("[%s] [Heartbeat] -> Vizinho %s voltou a responder! Considerado VIVO (ONLINE)!\n", serverConfig.ID, targetURL)
						} else {
							fmt.Printf("[%s] [Heartbeat] -> Vizinho %s não respondeu em 5s. Considerado MORTO (OFFLINE)!\n", serverConfig.ID, targetURL)
						}
					}

					v.Conectado = ativo
					if ativo {
						v.UltimoPing = time.Now()
						if infoID != "" {
							v.ID = infoID
						}
					}
					vizinhosHttpMu.Unlock()
				}(vURL)
			}
		}
	}()
}

func main() {
	carregarConfig()

	porta := fmt.Sprintf(":%s", serverConfig.Porta)

	listener, err := net.Listen("tcp", porta)
	if err != nil {
		fmt.Println("ERRO ao criar socket ouvidor, err: ", err)
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Printf("[%s] Escutando no endereço %s | DataDir: %s\n", serverConfig.ID, listener.Addr(), serverConfig.DataDir)
	if len(serverConfig.Cidades) > 0 {
		fmt.Printf("[%s] Cidades sob jurisdição: %v | Fronteira (CidadesInter): %v\n", serverConfig.ID, serverConfig.Cidades, serverConfig.CidadesInter)
	}

	iniciarArqTela()
	iniciarArqPassageiros()
	iniciarArqMotoristas()
	iniciarArqGrafo()
	iniciarArqCheckMotoristas()
	iniciarArqCheckPassageiro()
	iniciarArqTrechos()
	walRecuperar()
	motoristasOnline = make(map[string]string)
	passageirosOnline = make(map[string]string)
	pendentes = make(map[string]*Pendente)
	expirados = make(map[string]bool)
	iniciarServidorHttp()
	conectarVizinhosHttp()
	iniciarHeartbeat()

	mux := InicializaMux(&telas)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Failed to accept connection, err", err)
			continue
		}

		go handleConnection(conn, &mux)
	}
}

func iniciarArqTela() {
	once.Do(func() {
		caminho := getDataPath("telas.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			dados, err = os.ReadFile("../data/telas.json")
			if err != nil {
				fmt.Println("Erro ao ler arquivo telas.json:", err)
				os.Exit(1)
			}
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &telas)
		if err != nil {
			fmt.Println("Erro na conversão de json para map telas:", err)
			os.Exit(1)
		}
	})
}

func iniciarArqPassageiros() {
	once3.Do(func() {
		caminho := getDataPath("Passageiro.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			if os.IsNotExist(err) {
				passageiros = make(map[string]Model.Passageiro)
				os.WriteFile(caminho, []byte("{}"), 0644)
				return
			}
			fmt.Println("Erro ao ler arquivo Passageiro.json:", err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &passageiros)
		if err != nil {
			fmt.Println("Erro na conversão de json para passageiros:", err)
			os.Exit(1)
		}
		if passageiros == nil {
			passageiros = make(map[string]Model.Passageiro)
		}
	})
}

func iniciarArqMotoristas() {
	once4.Do(func() {
		caminho := getDataPath("Motoristas.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			if os.IsNotExist(err) {
				motoristas = make(map[string]Model.Motorista)
				os.WriteFile(caminho, []byte("{}"), 0644)
				return
			}
			fmt.Println("Erro ao ler arquivo Motoristas.json:", err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &motoristas)
		if err != nil {
			fmt.Println("Erro na conversão de json para motoristas:", err)
			os.Exit(1)
		}
		if motoristas == nil {
			motoristas = make(map[string]Model.Motorista)
		}
	})
}

func iniciarArqTrechos() {
	once9.Do(func() {
		caminho := getDataPath("trechos.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			if os.IsNotExist(err) {
				mapaTrechos = make(map[string]Model.Trecho)
				os.WriteFile(caminho, []byte("{}"), 0644)
				return
			}
			fmt.Println("Erro ao ler arquivo trechos.json:", err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &mapaTrechos)
		if err != nil {
			fmt.Println("Erro na conversão de json para trechos:", err)
			os.Exit(1)
		}
		if mapaTrechos == nil {
			mapaTrechos = make(map[string]Model.Trecho)
		}
	})
}

func iniciarArqCheckMotoristas() {
	once8.Do(func() {
		caminho := getDataPath("checkMotoristas.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			if os.IsNotExist(err) {
				checkMotoristas = make(map[string]Model.InfoLoginMotorista)
				os.WriteFile(caminho, []byte("{}"), 0644)
				return
			}
			fmt.Println("Erro ao ler arquivo checkMotoristas.json:", err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &checkMotoristas)
		if err != nil {
			fmt.Println("Erro na conversão de json para check motoristas:", err)
			os.Exit(1)
		}
		if checkMotoristas == nil {
			checkMotoristas = make(map[string]Model.InfoLoginMotorista)
		}
	})
}

func iniciarArqCheckPassageiro() {
	once10.Do(func() {
		caminho := getDataPath("checkPassageiros.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			if os.IsNotExist(err) {
				checkPassageiros = make(map[string]Model.InfoLoginPassageiro)
				os.WriteFile(caminho, []byte("{}"), 0644)
				return
			}
			fmt.Println("Erro ao ler arquivo checkPassageiros.json:", err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		err = json.Unmarshal(dados, &checkPassageiros)
		if err != nil {
			fmt.Println("Erro na conversão de json para check passageiros:", err)
			os.Exit(1)
		}
		if checkPassageiros == nil {
			checkPassageiros = make(map[string]Model.InfoLoginPassageiro)
		}
	})
}

func iniciarArqGrafo() {
	once5.Do(func() {
		if grafo == nil {
			grafo = Model.NovoGrafo()
		}
		caminho := getDataPath("grafo.json")
		dados, err := os.ReadFile(caminho)
		if err != nil {
			if os.IsNotExist(err) {
				os.WriteFile(caminho, []byte("{}"), 0644)
				return
			}
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf"))
		if len(dados) == 0 {
			return
		}
		json.Unmarshal(dados, &grafo)
	})
}

func handleConnection(conn net.Conn, mux *map[string]func(net.Conn, []byte, string)) {
	defer conn.Close()

	for {
		reader := bufio.NewReader(conn)
		headers := make(map[string]string)
		var err error

		headers, err = Lgcc.OneForAllParser(reader, headers)
		if err != nil {
			fmt.Println("Deu ruim o parser")
		}

		if headers["KeepAlive"] == "true" {
			switch headers["Protocolo"] {
			case "get":
				(*mux)[headers["Caminho"]](conn, nil, headers["Token"])
			case "post":
				n, _ := strconv.Atoi(headers["Tamanho-Conteudo"])
				body := make([]byte, n)
				if _, err := io.ReadFull(reader, body); err != nil {
					fmt.Println("Deu ruim a leitura no server post")
				}
				(*mux)[headers["Caminho"]](conn, body, headers["Token"])
			default:
				fmt.Println("é o default ")
			}
		} else {
			motoristasOnlineMu.Lock()
			delete(motoristasOnline, headers["Token"])
			motoristasOnlineMu.Unlock()
			passageirosOnlineMu.Lock()
			delete(passageirosOnline, headers["Token"])
			passageirosOnlineMu.Unlock()
			fmt.Println("Encerrando conexão...")
			break
		}
	}
}

func InicializaMux(tela *map[string]string) map[string]func(net.Conn, []byte, string) {
	var mux map[string]func(net.Conn, []byte, string)

	once2.Do(func() {
		telas := *tela

		mux = map[string]func(net.Conn, []byte, string){
			"/": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/"]
				if existe {
					body, tamanho, tipo, _ := Lgcc.GeraBody(valor)
					header := Lgcc.GeraHeaderFull("OK", 200, tipo, tamanho, "")
					Lgcc.EscritaResp(conn, header, body)
				} else {
					conn.Close()
				}
			},
			"/tela/login": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/login"]
				if existe {
					Lgcc.DevolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/cadastroPassageiro": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/cadastroPassageiro"]
				if existe {
					Lgcc.DevolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/cadastroMotorista": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/cadastroMotorista"]
				if existe {
					Lgcc.DevolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/menuMotorista": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/menuMotorista"]
				if existe {
					Lgcc.DevolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/menuMotorista/cidades": func(conn net.Conn, dados []byte, tk string) {
				if len(serverConfig.Cidades) > 0 {
					cidadesMsg := fmt.Sprintf("\nCidades disponíveis neste servidor (%s): %s\n", serverConfig.ID, strings.Join(serverConfig.Cidades, ", "))
					Lgcc.DevolutivaGet(cidadesMsg, "200", 200, tk, conn)
					return
				}
				valor, existe := telas["/tela/menuMotorista/cidades"]
				if existe {
					Lgcc.DevolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/cadastro/passageiro": func(conn net.Conn, dados2 []byte, tk string) {
				var p Model.Passageiro
				err := json.Unmarshal(dados2, &p)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				mutexCheckPassageiros.Lock()
				_, existe := checkPassageiros[p.Nome]
				if existe {
					mutexCheckPassageiros.Unlock()
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				} else {
					p.Id = Model.GerarID()
					passageiroMu.Lock()
					passageiros[p.Id] = p
					checkPassageiros[p.Nome] = Model.InfoLoginPassageiro{Nome: p.Nome, Senha: p.Senha, Id: p.Id}
					dados, _ := json.Marshal(passageiros)
					dados2, _ := json.Marshal(checkPassageiros)
					os.WriteFile(getDataPath("Passageiro.json"), dados, 0644)
					os.WriteFile(getDataPath("checkPassageiros.json"), dados2, 0644)
					passageiroMu.Unlock()
					mutexCheckPassageiros.Unlock()

					header := Lgcc.GeraHeaderRespPost("OK", 200, "")
					Lgcc.EscritaResp(conn, header, nil)
				}
			},
			"/cadastro/motorista": func(conn net.Conn, dados2 []byte, tk string) {
				var m Model.Motorista
				err := json.Unmarshal(dados2, &m)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				mutexCheckMotoristas.Lock()
				_, existe := checkMotoristas[m.Nome]
				if existe {
					mutexCheckMotoristas.Unlock()
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				} else {
					m.Id = Model.GerarID()
					motoristaMu.Lock()
					motoristas[m.Id] = m
					checkMotoristas[m.Nome] = Model.InfoLoginMotorista{Nome: m.Nome, Senha: m.Senha, Id: m.Id}
					dados, _ := json.Marshal(motoristas)
					dados2, _ := json.Marshal(checkMotoristas)
					os.WriteFile(getDataPath("Motoristas.json"), dados, 0644)
					os.WriteFile(getDataPath("checkMotoristas.json"), dados2, 0644)
					motoristaMu.Unlock()
					mutexCheckMotoristas.Unlock()

					header := Lgcc.GeraHeaderRespPost("OK", 200, "")
					Lgcc.EscritaResp(conn, header, nil)
				}
			},
			"/login/passageiro": func(conn net.Conn, dados2 []byte, tka string) {
				var p Model.InfoLoginPassageiro
				err := json.Unmarshal(dados2, &p)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				mutexCheckPassageiros.Lock()
				a, existe := checkPassageiros[p.Nome]
				if existe {
					if a.Senha == p.Senha {
						tk, _ := Lgcc.GerarToken(16)
						passageirosOnlineMu.Lock()
						passageirosOnline[tk] = a.Id
						passageirosOnlineMu.Unlock()
						mutexCheckPassageiros.Unlock()
						header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
						Lgcc.EscritaResp(conn, header, nil)
					} else {
						fmt.Println("Senha rejeitada")
						Lgcc.GeraRespostaErro(999, conn, "")
						mutexCheckPassageiros.Unlock()
						return
					}
				} else {
					fmt.Println("Usuário sequer existe")
					mutexCheckPassageiros.Unlock()
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
			},
			"/logout/passageiro": func(conn net.Conn, dados2 []byte, tk string) {
				passageirosOnlineMu.Lock()
				idPassageiro, existe := passageirosOnline[tk]
				delete(passageirosOnline, tk)
				passageirosOnlineMu.Unlock()

				if existe {
					fmt.Printf("Passageiro %s deslogado com sucesso (token %s)!\n", idPassageiro, tk)
				}
				header := Lgcc.GeraHeaderRespPost("OK", 200, "")
				Lgcc.EscritaResp(conn, header, nil)
			},
			"/login/motorista": func(conn net.Conn, dados2 []byte, tk string) {
				var m Model.InfoLoginMotorista
				err := json.Unmarshal(dados2, &m)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				mutexCheckMotoristas.Lock()
				a, existe := checkMotoristas[m.Nome]
				mutexCheckMotoristas.Unlock()
				if existe {
					if a.Senha == m.Senha {
						tk, _ := Lgcc.GerarToken(16)
						motoristasOnlineMu.Lock()
						motoristasOnline[tk] = a.Id
						motoristasOnlineMu.Unlock()
						header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
						Lgcc.EscritaResp(conn, header, nil)
					} else {
						fmt.Println("Senha Incorreta")
						Lgcc.GeraRespostaErro(999, conn, "")
						return
					}
				} else {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
			},
			"/logout/motorista": func(conn net.Conn, dados2 []byte, tk string) {
				motoristasOnlineMu.Lock()
				idMotorista, existe := motoristasOnline[tk]
				delete(motoristasOnline, tk)
				motoristasOnlineMu.Unlock()

				if existe {
					fmt.Printf("Motorista %s deslogado com sucesso (token %s)!\n", idMotorista, tk)
				}
				header := Lgcc.GeraHeaderRespPost("OK", 200, "")
				Lgcc.EscritaResp(conn, header, nil)
			},
			"/passageiro/listarCorridasCadastradas": func(conn net.Conn, dados2 []byte, tk string) {
				passageirosOnlineMu.Lock()
				idPassageiro, existe := passageirosOnline[tk]
				passageirosOnlineMu.Unlock()
				if !existe || idPassageiro == "" {
					Lgcc.GeraRespostaErro(401, conn, "")
					return
				}

				passageiroMu.Lock()
				p, tem := passageiros[idPassageiro]
				passageiroMu.Unlock()

				if !tem || len(p.IsTrechosPassageiros) == 0 {
					Lgcc.DevolutivaGet("Nenhuma carona reservada no momento.\n", "OK", 200, tk, conn)
					return
				}

				mutexMapaTrecho.Lock()
				var sb strings.Builder
				sb.WriteString("Seus trechos reservados:\n")
				for _, idTrecho := range p.IsTrechosPassageiros {
					t, ok := mapaTrechos[idTrecho]
					if ok {
						sb.WriteString(fmt.Sprintf("- Trecho: %s | Data: %s | Valor: R$ %.2f\n", t.Id, t.Data, t.Preco))
					} else {
						sb.WriteString(fmt.Sprintf("- Trecho ID (Servidor Remoto): %s\n", idTrecho))
					}
				}
				mutexMapaTrecho.Unlock()
				Lgcc.DevolutivaGet(sb.String(), "OK", 200, tk, conn)
			},
			// Servidor TCP atua como intermediário repassando a busca ao servidor HTTP
			"/passageiro/buscaRotasPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				var pa Model.InfoPassagem
				err := json.Unmarshal(dados2, &pa)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				params := url.Values{}
				params.Set("origem", pa.Origem)
				params.Set("destino", pa.Destino)
				params.Set("data", pa.Data)
				urlHttp := fmt.Sprintf("http://127.0.0.1:%s/passageiro/buscar-rotas?%s", serverConfig.PortaHttp, params.Encode())

				httpReq, err := http.NewRequest(http.MethodGet, urlHttp, nil)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				httpReq.Header.Set("Authorization", "Bearer "+tk)
				httpReq.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))

				httpClient := &http.Client{Timeout: 15 * time.Second}
				resp, err := httpClient.Do(httpReq)
				if err != nil {
					fmt.Println("Erro ao consultar servidor HTTP local:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				defer resp.Body.Close()

				if v := resp.Header.Get("X-Lamport"); v != "" {
					if lv, err := strconv.ParseInt(v, 10, 64); err == nil {
						updateLamport(lv)
					}
				}

				if resp.StatusCode != http.StatusOK {
					Lgcc.GeraRespostaErro(resp.StatusCode, conn, "")
					return
				}

				dadosRotas, err := io.ReadAll(resp.Body)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				body, tamanho, tipo, err := Lgcc.GeraBody(dadosRotas)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				header := Lgcc.GeraHeaderFull("OK", 200, tipo, tamanho, tk)
				Lgcc.EscritaResp(conn, header, body)
			},
			// Servidor TCP atua como intermediário repassando a reserva ao servidor HTTP (/passageiro/tentar-reserva)
			"/passageiro/tentaReserva": func(conn net.Conn, dados2 []byte, tk string) {
				urlHttp := fmt.Sprintf("http://127.0.0.1:%s/passageiro/tentar-reserva", serverConfig.PortaHttp)
				httpReq, err := http.NewRequest(http.MethodPost, urlHttp, bytes.NewReader(dados2))
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				httpReq.Header.Set("Content-Type", "application/json")
				httpReq.Header.Set("Authorization", "Bearer "+tk)
				httpReq.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))

				httpClient := &http.Client{Timeout: 35 * time.Second}
				resp, err := httpClient.Do(httpReq)
				if err != nil {
					fmt.Println("Erro ao encaminhar reserva para servidor HTTP:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				defer resp.Body.Close()

				if v := resp.Header.Get("X-Lamport"); v != "" {
					if lv, err := strconv.ParseInt(v, 10, 64); err == nil {
						updateLamport(lv)
					}
				}

				if resp.StatusCode == http.StatusOK {
					header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
					Lgcc.EscritaResp(conn, header, nil)
					return
				}

				Lgcc.GeraRespostaErro(resp.StatusCode, conn, tk)
			},
			// Servidor TCP atua como intermediário repassando o cancelamento ao servidor HTTP (/passageiro/cancelar-passagem)
			"/passageiro/cancelarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				urlHttp := fmt.Sprintf("http://127.0.0.1:%s/passageiro/cancelar-passagem", serverConfig.PortaHttp)
				httpReq, err := http.NewRequest(http.MethodPost, urlHttp, nil)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				httpReq.Header.Set("Authorization", "Bearer "+tk)
				httpReq.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))

				httpClient := &http.Client{Timeout: 35 * time.Second}
				resp, err := httpClient.Do(httpReq)
				if err != nil {
					fmt.Println("Erro ao encaminhar cancelamento para servidor HTTP:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				defer resp.Body.Close()

				if v := resp.Header.Get("X-Lamport"); v != "" {
					if lv, err := strconv.ParseInt(v, 10, 64); err == nil {
						updateLamport(lv)
					}
				}

				if resp.StatusCode == http.StatusOK {
					header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
					Lgcc.EscritaResp(conn, header, nil)
					return
				}

				Lgcc.GeraRespostaErro(resp.StatusCode, conn, tk)
			},
			"/motorista/tentaCadastrarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
				var infoCarona Model.InfoCarona
				err := json.Unmarshal(dados2, &infoCarona)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "JSON inválido")
					return
				}

				motoristasOnlineMu.Lock()
				idMotorista, existeOnline := motoristasOnline[tk]
				motoristasOnlineMu.Unlock()
				if !existeOnline || idMotorista == "" {
					Lgcc.GeraRespostaErro(401, conn, "Motorista não autenticado")
					return
				}

				motoristaMu.Lock()
				motorista, existeMotorista := motoristas[idMotorista]
				if !existeMotorista {
					motoristaMu.Unlock()
					Lgcc.GeraRespostaErro(404, conn, "Motorista não encontrado")
					return
				}
				if len(motorista.Trechos) != 0 {
					motoristaMu.Unlock()
					Lgcc.GeraRespostaErro(409, conn, "Motorista já possui corrida cadastrada")
					return
				}
				motoristaMu.Unlock()

				// Valida os vértices mandados pelo motorista
				vertices, err := Model.ProcessarLista(infoCarona.Rota)
				if err != nil || len(vertices) < 2 {
					fmt.Printf("[tentaCadastrarCorrida] Formato de rota inválido: '%s' (erro: %v)\n", infoCarona.Rota, err)
					Lgcc.GeraRespostaErro(400, conn, "Formato de rota inválido. Use pelo menos 2 cidades separadas por vírgula.")
					return
				}

				// Checa se os vértices fazem parte do slice de cidades do servidor (serverConfig.Cidades)
				if cidadeInvalida, ok := validarVerticesCidades(vertices); !ok {
					msgErro := fmt.Sprintf("Erro: a cidade '%s' não faz parte das cidades deste servidor (%v)", cidadeInvalida, serverConfig.Cidades)
					fmt.Printf("[tentaCadastrarCorrida] %s\n", msgErro)
					Lgcc.GeraRespostaErro(403, conn, msgErro)
					return
				}

				fmt.Printf("[tentaCadastrarCorrida] Rota %v validada com sucesso para o servidor %s\n", vertices, serverConfig.ID)
				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
			"/motorista/cadastrarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
				var infoCarona Model.InfoCarona
				err := json.Unmarshal(dados2, &infoCarona)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "JSON inválido")
					return
				}

				motoristasOnlineMu.Lock()
				idMotorista, existeOnline := motoristasOnline[tk]
				motoristasOnlineMu.Unlock()
				if !existeOnline || idMotorista == "" {
					Lgcc.GeraRespostaErro(401, conn, "Motorista não autenticado")
					return
				}

				trechos, vertices, err := Model.ProcessarCadastroCorrida(infoCarona)
				if err != nil || len(vertices) < 2 {
					fmt.Println("ERRO NO PROCESSAR CADASTRO CORRIDA:", err)
					Lgcc.GeraRespostaErro(400, conn, "Formato de rota inválido. Use pelo menos 2 cidades separadas por vírgula.")
					return
				}

				// Verifica se todos os vértices fazem parte do slice de cidades do servidor (serverConfig.Cidades)
				if cidadeInvalida, ok := validarVerticesCidades(vertices); !ok {
					msgErro := fmt.Sprintf("Erro: a cidade '%s' não faz parte das cidades deste servidor (%v)", cidadeInvalida, serverConfig.Cidades)
					fmt.Printf("[cadastrarCorrida] %s\n", msgErro)
					Lgcc.GeraRespostaErro(403, conn, msgErro)
					return
				}

				listaPairTrechos, err := Model.CriarPairTrechos(trechos, vertices)
				if err != nil {
					fmt.Println("ERRO NÃO TRATAADOOOO")
					Lgcc.GeraRespostaErro(999, conn, "Erro ao criar trechos")
					return
				}

				motoristaMu.Lock()
				motorista := motoristas[idMotorista]
				motorista.Trechos = listaPairTrechos
				motoristas[idMotorista] = motorista

				dados, _ := json.Marshal(motoristas)
				os.WriteFile(getDataPath("Motoristas.json"), dados, 0644)
				m := motoristas[idMotorista]
				motoristaMu.Unlock()

				grafoMu.Lock()
				err = Model.PreencheTrechosGrafo(m, grafo)
				if err != nil {
					fmt.Println("Erro no preenchemento do grafo")
				}
				grafito, err := json.Marshal(grafo)
				if err != nil {
					fmt.Println("Erro ao converter grafo para json")
				}
				os.WriteFile(getDataPath("grafo.json"), grafito, 0644)
				grafoMu.Unlock()
				fmt.Println("Grafo atualizado com sucesso!")

				mutexMapaTrecho.Lock()
				for i := range trechos {
					mapaTrechos[trechos[i].Id] = trechos[i]
				}

				a, _ := json.Marshal(mapaTrechos)
				os.WriteFile(getDataPath("trechos.json"), a, 0644)
				mutexMapaTrecho.Unlock()

				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
			// Intermediário TCP -> repassa para o servidor HTTP local (/motorista/cancelar-corrida),
			// que coordena o 2PC e remove os passageiros afetados dos trechos locais e remotos
			"/motorista/cancelarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
				httpURL := fmt.Sprintf("http://127.0.0.1:%s/motorista/cancelar-corrida", serverConfig.PortaHttp)
				reqHttp, err := http.NewRequest(http.MethodPost, httpURL, nil)
				if err != nil {
					Lgcc.GeraRespostaErro(400, conn, "")
					return
				}
				reqHttp.Header.Set("Authorization", "Bearer "+tk)
				reqHttp.Header.Set("X-Lamport", strconv.FormatInt(incLamport(), 10))
				respHttp, err := (&http.Client{Timeout: 30 * time.Second}).Do(reqHttp)
				if err != nil {
					Lgcc.GeraRespostaErro(500, conn, "")
					return
				}
				defer respHttp.Body.Close()

				if v := respHttp.Header.Get("X-Lamport"); v != "" {
					if lv, errL := strconv.ParseInt(v, 10, 64); errL == nil {
						updateLamport(lv)
					}
				}

				if respHttp.StatusCode != http.StatusOK {
					Lgcc.GeraRespostaErro(respHttp.StatusCode, conn, "")
					return
				}
				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
		}
	})
	return mux
}
