package main

import (
	"Golang/Lgcc"
	"Golang/Model"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type ServerConfig struct {
	ID            string   `json:"id"`
	Porta         string   `json:"porta"`
	DataDir       string   `json:"dataDir"`
	Vizinhos      []string `json:"vizinhos"`
	Cidades       []string `json:"cidades"`
	CidadesInter  []string `json:"cidadesInter"`
}

var serverConfig ServerConfig

func getDataPath(arquivo string) string {
	return filepath.Join(serverConfig.DataDir, arquivo)
}

func carregarConfig() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: go run servidor.go <config.json> ou go run servidor.go <porta>")
		os.Exit(1)
	}

	arg := os.Args[1]
	if strings.HasSuffix(arg, ".json") {
		dados, err := os.ReadFile(arg)
		if err != nil {
			// Fallback caso seja executado de dentro da pasta Server/
			dados, err = os.ReadFile(filepath.Join("..", arg))
		}
		if err != nil {
			fmt.Printf("Erro ao ler arquivo de config %s: %v\n", arg, err)
			os.Exit(1)
		}
		dados = bytes.TrimPrefix(dados, []byte("\xef\xbb\xbf")) //isso é pra possíveis erros do windows
		err = json.Unmarshal(dados, &serverConfig)
		if err != nil {
			fmt.Printf("Erro ao converter json de config: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Fallback compatível com porta simples (ex: go run servidor.go 6742)
		//fallback para caso não seja um arquivo compatível
		serverConfig = ServerConfig{
			ID:       "default",
			Porta:    arg,
			DataDir:  "../data",
			Vizinhos: []string{},
			Cidades:  []string{},
		}
	}

	// Garante que o diretório de dados existe
	os.MkdirAll(serverConfig.DataDir, 0755)
}


var (
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
	if len(serverConfig.Vizinhos) > 0 {
		fmt.Printf("[%s] Servidores vizinhos: %v\n", serverConfig.ID, serverConfig.Vizinhos)
	}
	if len(serverConfig.Cidades) > 0 {
		fmt.Printf("[%s] Cidades sob jurisdição: %v\n", serverConfig.ID, serverConfig.Cidades)
	}

	iniciarArqTela()
	iniciarArqPassageiros()
	iniciarArqMotoristas()
	iniciarArqGrafo()
	iniciarArqCheckMotoristas()
	iniciarArqCheckPassageiro()
	iniciarArqTrechos()
	motoristasOnline = make(map[string]string)
	passageirosOnline = make(map[string]string)
	mux := InicializaMux(&telas)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Failed to accept connection, err", err)
			continue
		}

		//precisamos ter um conjunto de n threads por exemplo que vão tratar um outro conjunto
		//v de mensagens que chegam

		//a gente pode ter 1 thread para leitura e n-1 threads que vão tratar as requisições da
		//minha fila de requisições
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
	var mux map[string]func(net.Conn, []byte, string) // isso daqui não seria alocado em memória?

	//apesar de ser inicializado somente uma vez
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
						header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
						Lgcc.EscritaResp(conn, header, nil)
					} else {
						fmt.Println("Senha rejeitada")
						Lgcc.GeraRespostaErro(999, conn, "")
						return
					}
				} else {
					fmt.Println("Usuário sequer existe")
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				mutexCheckPassageiros.Unlock()

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
				a, existe := checkMotoristas[m.Nome]
				if existe {
					if a.Senha == m.Senha {
						tk, _ := Lgcc.GerarToken(16)
						motoristasOnline[tk] = a.Id
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
						sb.WriteString(fmt.Sprintf("- Trecho ID: %s\n", idTrecho))
					}
				}
				mutexMapaTrecho.Unlock()
				Lgcc.DevolutivaGet(sb.String(), "OK", 200, tk, conn)
			},
			"/passageiro/tentaReservarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				var pa Model.InfoPassagem
				err := json.Unmarshal(dados2, &pa)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				grafoMu.Lock()
				motoristaMu.Lock()
				mutexMapaTrecho.Lock()
				rotas, err := grafo.BuscarRotas(pa.Origem, pa.Destino, pa.Data, motoristas, mapaTrechos)
				mutexMapaTrecho.Unlock()
				motoristaMu.Unlock()
				grafoMu.Unlock()

				if err != nil {
					fmt.Println("Erro ao buscar rotas:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				dadosRotas, err := json.Marshal(rotas)
				if err != nil {
					fmt.Println("Erro ao serializar rotas:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				body, tamanho, tipo, err := Lgcc.GeraBody(dadosRotas)
				if err != nil {
					fmt.Println("Erro ao gerar body:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				header := Lgcc.GeraHeaderFull("OK", 200, tipo, tamanho, tk)
				Lgcc.EscritaResp(conn, header, body)

			},
			"/passageiro/reservarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				var req Model.InfoConfirmarReserva
				err := json.Unmarshal(dados2, &req)
				if err != nil || len(req.Rota) == 0 {
					fmt.Println("Erro no unmarshal de reservarPassagem:", err)
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				passageirosOnlineMu.Lock()
				idPassageiro, existe := passageirosOnline[tk]
				passageirosOnlineMu.Unlock()
				if !existe || idPassageiro == "" {
					fmt.Println("Passageiro não autenticado para reservar passagem")
					Lgcc.GeraRespostaErro(401, conn, "")
					return
				}

				motoristaMu.Lock()
				mutexMapaTrecho.Lock()

				var trechosParaReservar []string
				rotaValida := true

				for _, aresta := range req.Rota {
					achouTrechoAresta := false
					for _, idMotorista := range aresta.Ids {
						m, ok := motoristas[idMotorista]
						if !ok {
							continue
						}
						for _, p := range m.Trechos {
							if p.Origem == aresta.Origem && p.Destino == aresta.Destino {
								t, ok := mapaTrechos[p.IDtrecho]
								if ok && t.Data == req.Data && t.Nassentos > 0 {
									trechosParaReservar = append(trechosParaReservar, p.IDtrecho)
									achouTrechoAresta = true
									break
								}
							}
						}
						if achouTrechoAresta {
							break
						}
					}
					if !achouTrechoAresta {
						rotaValida = false
						break
					}
				}

				if !rotaValida || len(trechosParaReservar) != len(req.Rota) {
					mutexMapaTrecho.Unlock()
					motoristaMu.Unlock()
					fmt.Println("Não há assentos disponíveis em todos os trechos da rota escolhida")
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				for _, idT := range trechosParaReservar {
					t := mapaTrechos[idT]
					t.Nassentos--

					// Atualiza o slice de IdPassageiros preenchendo a vaga com o id do passageiro
					inserido := false
					for i, idp := range t.IdPassageiros {
						if idp == "" {
							t.IdPassageiros[i] = idPassageiro
							inserido = true
							break
						}
					}
					// Se o slice não tinha slots pré-alocados com string vazia, adiciona no slice
					if !inserido {
						t.IdPassageiros = append(t.IdPassageiros, idPassageiro)
					}

					mapaTrechos[idT] = t
				}

				dadosTrechos, _ := json.Marshal(mapaTrechos)
				os.WriteFile(getDataPath("trechos.json"), dadosTrechos, 0644)

				mutexMapaTrecho.Unlock()
				motoristaMu.Unlock()

				// Atribui os ids dos trechos reservados ao passageiro
				passageiroMu.Lock()
				p, ok := passageiros[idPassageiro]
				if ok {
					p.IsTrechosPassageiros = trechosParaReservar
					passageiros[idPassageiro] = p
					dadosP, _ := json.Marshal(passageiros)
					os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
				}
				passageiroMu.Unlock()

				fmt.Printf("Passagem reservada com sucesso para o passageiro %s! Trechos: %v\n", idPassageiro, trechosParaReservar)
				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
			"/passageiro/cancelarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				passageirosOnlineMu.Lock()
				idPassageiro, existe := passageirosOnline[tk]
				passageirosOnlineMu.Unlock()
				if !existe || idPassageiro == "" {
					Lgcc.GeraRespostaErro(401, conn, "")
					return
				}

				passageiroMu.Lock()
				p, existePassageiro := passageiros[idPassageiro]
				if !existePassageiro || len(p.IsTrechosPassageiros) == 0 {
					passageiroMu.Unlock()
					fmt.Printf("Passageiro %s não possui passagens para cancelar\n", idPassageiro)
					Lgcc.GeraRespostaErro(404, conn, "")
					return
				}

				trechosParaLiberar := p.IsTrechosPassageiros

				mutexMapaTrecho.Lock()
				for _, idTrecho := range trechosParaLiberar {
					if t, ok := mapaTrechos[idTrecho]; ok {
						t.Nassentos++
						for i, idp := range t.IdPassageiros {
							if idp == idPassageiro {
								t.IdPassageiros[i] = ""
								break
							}
						}
						mapaTrechos[idTrecho] = t
					}
				}
				dadosTrechos, _ := json.Marshal(mapaTrechos)
				os.WriteFile(getDataPath("trechos.json"), dadosTrechos, 0644)
				mutexMapaTrecho.Unlock()

				p.IsTrechosPassageiros = []string{}
				passageiros[idPassageiro] = p
				dadosP, _ := json.Marshal(passageiros)
				os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
				passageiroMu.Unlock()

				fmt.Printf("Passagem cancelada com sucesso para o passageiro %s!\n", idPassageiro)
				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			}, "/motorista/tentaCadastrarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
				//verificar que este puto tem um corrida já cadastrada, caso seja verdade já impede aí
				//se não tiver corrida cadastrada tem que checar se existe uma possibilidade de criar rota para o local
				//profano
				var infoCarona Model.InfoCarona
				err := json.Unmarshal(dados2, &infoCarona)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}

				motoristasOnlineMu.Lock()
				idMotorista := motoristasOnline[tk]
				motoristasOnlineMu.Unlock()

				motoristaMu.Lock()
				motorista := motoristas[idMotorista]
				if len(motorista.Trechos) != 0 {
					motoristaMu.Unlock()
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				motoristaMu.Unlock()
				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)

				//verificar se o conjunto de dados mandado está dentro do escopo de vértices permitidos pelo servidor
				//Para isso temos que fazer uma identificação de cada servidor e iniciar os dados
				//de cada respectivo servidor baseado em de id

			},
			"/motorista/cadastrarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
				fmt.Println("Cheguei no cadastrar corrida")
				var infoCarona Model.InfoCarona
				err := json.Unmarshal(dados2, &infoCarona)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				trechos, vertices, err := Model.ProcessarCadastroCorrida(infoCarona)
				if err != nil {
					fmt.Println("ERRO NO PROCESSAR CADASTRO CORRIDA")
				}

				listaPairTrechos, err := Model.CriarPairTrechos(trechos, vertices)
				if err != nil {
					fmt.Println("ERRO NÃO TRATAADOOOO")
				}

				motoristasOnlineMu.Lock()
				idMotorista := motoristasOnline[tk]
				motoristasOnlineMu.Unlock()

				motoristaMu.Lock()
				motorista := motoristas[idMotorista]
				motorista.Trechos = listaPairTrechos
				motoristas[idMotorista] = motorista

				dados, _ := json.Marshal(motoristas)
				os.WriteFile(getDataPath("Motoristas.json"), dados, 0644)
				m := motoristas[idMotorista]
				motoristaMu.Unlock()

				//Temos que separar a lógica de preencher o grafo
				//Da lógica de preencher as arestas

				grafoMu.Lock()
				err = Model.PreencheTrechosGrafo(m, grafo)
				if err != nil {
					fmt.Println("Erro no preenchemento do grafo")
				}
				//Atualizar arquivo grafo
				grafito, err := json.Marshal(grafo)
				if err != nil {
					fmt.Println("Erro ao converter grafo para json")
				}
				//Por algum motivo os Trechos não estão sendo salvos
				os.WriteFile(getDataPath("grafo.json"), grafito, 0644)
				grafoMu.Unlock()
				fmt.Println("Grafo atualizado com sucesso!")

				mutexMapaTrecho.Lock()
				for i := range trechos {
					mapaTrechos[trechos[i].Id] = trechos[i]
				}

				a, err := json.Marshal(mapaTrechos)
				os.WriteFile(getDataPath("trechos.json"), a, 0644)
				mutexMapaTrecho.Unlock()

				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
			"/motorista/cancelarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
				motoristasOnlineMu.Lock()
				idMotorista, online := motoristasOnline[tk]
				motoristasOnlineMu.Unlock()
				if !online || idMotorista == "" {
					Lgcc.GeraRespostaErro(401, conn, "")
					return
				}

				motoristaMu.Lock()
				motorista, existe := motoristas[idMotorista]
				if !existe || len(motorista.Trechos) == 0 {
					motoristaMu.Unlock()
					fmt.Printf("Motorista %s não tem corrida ativa para cancelar\n", idMotorista)
					Lgcc.GeraRespostaErro(404, conn, "")
					return
				}
				pairTrechos := motorista.Trechos

				// 1. Identifica os passageiros atendidos pelos trechos deste motorista
				passageiroMu.Lock()
				mutexMapaTrecho.Lock()

				passageirosAfetados := make(map[string]bool)

				for _, pair := range pairTrechos {
					trecho, existeTrecho := mapaTrechos[pair.IDtrecho]
					if existeTrecho {
						for _, idPass := range trecho.IdPassageiros {
							if idPass != "" {
								passageirosAfetados[idPass] = true
							}
						}
					}
				}

				// 2. Para cada passageiro atendido, acessa o slice de trechos dele e remove o passageiro de todos eles
				for idPass := range passageirosAfetados {
					if p, ok := passageiros[idPass]; ok {
						for _, idTrechoDoPassageiro := range p.IsTrechosPassageiros {
							if tPass, existeTPass := mapaTrechos[idTrechoDoPassageiro]; existeTPass {
								tPass.Nassentos++
								for i, idp := range tPass.IdPassageiros {
									if idp == idPass {
										tPass.IdPassageiros[i] = ""
										break
									}
								}
								mapaTrechos[idTrechoDoPassageiro] = tPass
							}
						}
						// Limpa o slice de trechos do passageiro
						p.IsTrechosPassageiros = []string{}
						passageiros[idPass] = p
					}
				}

				// 3. Remove do mapaTrechos os trechos do motorista que cancelou
				for _, pair := range pairTrechos {
					delete(mapaTrechos, pair.IDtrecho)
				}

				// Salva trechos.json e Passageiro.json
				dadosT, _ := json.Marshal(mapaTrechos)
				os.WriteFile(getDataPath("trechos.json"), dadosT, 0644)
				mutexMapaTrecho.Unlock()

				dadosP, _ := json.Marshal(passageiros)
				os.WriteFile(getDataPath("Passageiro.json"), dadosP, 0644)
				passageiroMu.Unlock()

				// 2. Atualiza o grafo retirando o id motorista associado àquele trecho
				grafoMu.Lock()
				for _, pair := range pairTrechos {
					grafo.RmvIdTrecho(pair.Origem, pair.Destino, idMotorista)
				}
				dadosGrafo, _ := json.Marshal(grafo)
				os.WriteFile(getDataPath("grafo.json"), dadosGrafo, 0644)
				grafoMu.Unlock()

				// 3. Limpa os trechos do motorista e salva em Motoristas.json
				motorista.Trechos = []Model.PairTrecho{}
				motoristas[idMotorista] = motorista
				dadosM, _ := json.Marshal(motoristas)
				os.WriteFile(getDataPath("Motoristas.json"), dadosM, 0644)
				motoristaMu.Unlock()

				fmt.Printf("Corrida do motorista %s cancelada com sucesso! Trechos removidos: %d\n", idMotorista, len(pairTrechos))
				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
		}
	})
	return mux
}
