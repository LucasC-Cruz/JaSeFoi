package main

import (
	"Golang/Lgcc"
	"Golang/Model"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
)

type InfoReserva struct {
	Origem  string `json:"origem"`
	Destino string `json:"destin"`
	Data    string `json:"data"`
}

var (
	telas                map[string]string
	passageiros          map[string]Model.Passageiro
	motoristas           map[string]Model.Motorista
	motoristasOnline     map[string]string
	passageirosOnline    map[string]string
	caronas              map[string]Model.Carona
	grafo                map[string]map[string][]Model.Trecho
	caronasPassageiros   map[string]Model.CaronaPassageiro
	caronasPassageirosMu sync.Mutex

	caronasMu    sync.Mutex
	passageiroMu sync.Mutex
	motoristaMu  sync.Mutex
	grafoMu      sync.Mutex
	once         sync.Once
	once2        sync.Once
	once3        sync.Once
	once4        sync.Once
	once5        sync.Once
	once6        sync.Once
	once7        sync.Once
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Comando incorreto!")
		fmt.Println("Talvez queira dizer: go run <arquivo> <porta>?")
		os.Exit(1)
	}

	porta := fmt.Sprintf(":%s", os.Args[1])

	listener, err := net.Listen("tcp", porta)
	if err != nil {
		fmt.Println("ERRO ao criar socket ouvidor, err: ", err)
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Printf("Estou escutando no endereço %s\n", listener.Addr())

	iniciarArqTela()
	iniciarArqPassageiros()
	iniciarArqMotoristas()
	iniciarArqGrafo()
	iniciarArqCaronas()
	iniciarArqCaronasPassageiros()
	motoristasOnline = make(map[string]string)
	passageirosOnline = make(map[string]string)

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
		dados, err := os.ReadFile("../data/telas.json")
		if err != nil {
			fmt.Println("Erro ao ler arquivo", err)
			os.Exit(1)
		}
		err = json.Unmarshal(dados, &telas)
		if err != nil {
			fmt.Println("Erro na conversão de json para map")
			os.Exit(1)
		}
	})
}

func iniciarArqPassageiros() {
	once3.Do(func() {
		dados, err := os.ReadFile("../data/Passageiro.json")
		if err != nil {
			fmt.Println("Erro ao ler arquivo Passageiro.json", err)
			os.Exit(1)
		}
		err = json.Unmarshal(dados, &passageiros)
		if err != nil {
			fmt.Println("Erro na conversão de json para slice de passageiros")
			os.Exit(1)
		}
	})
}

func iniciarArqMotoristas() {
	once4.Do(func() {
		dados, err := os.ReadFile("../data/Motoristas.json")
		if err != nil {
			fmt.Println("Erro ao ler arquivo Motoristas.json", err)
			os.Exit(1)
		}
		err = json.Unmarshal(dados, &motoristas)
		if err != nil {
			fmt.Println("Erro na conversão de json para slice de usuarios")
			os.Exit(1)
		}
	})
}

func iniciarArqGrafo() {
	once5.Do(func() {
		if grafo == nil {
			grafo = make(map[string]map[string][]Model.Trecho)
		}
		dados, err := os.ReadFile("../data/grafo.json")
		if err != nil {
			if os.IsNotExist(err) {
				os.WriteFile("../data/grafo.json", []byte("{}"), 0644)
				return
			}
			os.Exit(1)
		}
		if len(dados) == 0 {
			return
		}
		json.Unmarshal(dados, &grafo)
		if grafo == nil {
			grafo = make(map[string]map[string][]Model.Trecho)
		}
	})
}

func iniciarArqCaronas() {
	once6.Do(func() {
		if caronas == nil {
			caronas = make(map[string]Model.Carona)
		}
		dados, err := os.ReadFile("../data/carona.json")
		if err != nil {
			if os.IsNotExist(err) {
				os.WriteFile("../data/carona.json", []byte("{}"), 0644)
				return
			}
			os.Exit(1)
		}
		if len(dados) == 0 {
			return
		}
		json.Unmarshal(dados, &caronas)
		if caronas == nil {
			caronas = make(map[string]Model.Carona)
		}
	})
}

func iniciarArqCaronasPassageiros() {	
	once7.Do(func() {
		if caronasPassageiros == nil {
			caronasPassageiros = make(map[string]Model.CaronaPassageiro)
		}
		dados, err := os.ReadFile("../data/caronas_passageiros.json")
		if err != nil {
			if os.IsNotExist(err) {
				os.WriteFile("../data/caronas_passageiros.json", []byte("{}"), 0644)
				return
			}
			return
		}
		if len(dados) == 0 {
			return
		}
		json.Unmarshal(dados, &caronasPassageiros)
		if caronasPassageiros == nil {
			caronasPassageiros = make(map[string]Model.CaronaPassageiro)
		}
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
			delete(motoristasOnline, headers["Token"])
			delete(passageirosOnline, headers["Token"])
			fmt.Println("Encerrando conexão...")
			break
		}
	}
}

func InicializaMux(tela *map[string]string) map[string]func(net.Conn, []byte, string) {
	var mux map[string]func(net.Conn, []byte, string)// isso daqui não seria alocado em memória?

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
					Lgcc.devolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/cadastroPassageiro": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/cadastroPassageiro"]
				if existe {
					Lgcc.devolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/cadastroMotorista": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/cadastroMotorista"]
				if existe {
					Lgcc.devolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/menuMotorista": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/menuMotorista"]
				if existe {
					Lgcc.devolutivaGet(valor, "200", 200, tk, conn)
				} else {
					conn.Close()
				}
			},
			"/tela/menuMotorista/cidades": func(conn net.Conn, dados []byte, tk string) {
				valor, existe := telas["/tela/menuMotorista/cidades"]
				if existe {
					Lgcc.devolutivaGet(valor, "200", 200, tk, conn)
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
				passageiroMu.Lock()
				_, existe := passageiros[p.Nome]
				if existe {
					passageiroMu.Unlock()
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				} else {
					p.Id = Model.GerarID()
					passageiros[p.Nome] = p
					a, _ := json.Marshal(passageiros)
					os.WriteFile("../data/Passageiro.json", a, 0644)
					passageiroMu.Unlock()

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
				motoristaMu.Lock()
				_, existe := motoristas[m.Nome]
				if existe {
					motoristaMu.Unlock()
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				} else {
					m.Id = Model.GerarID()
					motoristas[m.Nome] = m
					cu, _ := json.Marshal(motoristas)
					os.WriteFile("../data/Motoristas.json", cu, 0644)
					motoristaMu.Unlock()

					header := Lgcc.GeraHeaderRespPost("OK", 200, "")
					Lgcc.EscritaResp(conn, header, nil)
				}
			},
			"/login/passageiro": func(conn net.Conn, dados2 []byte, tka string) {
				var p Model.Passageiro
				err := json.Unmarshal(dados2, &p)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				_, existe := passageiros[p.Nome]
				if existe {
					tk, _ := Lgcc.GerarToken(16)
					passageirosOnline[tk] = p.Nome
					header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
					Lgcc.EscritaResp(conn, header, nil)
				} else {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
			},
			"/login/motorista": func(conn net.Conn, dados2 []byte, tka string) {
				var m Model.Motorista
				err := json.Unmarshal(dados2, &m)
				if err != nil {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
				_, existe := motoristas[m.Nome]
				if existe {
					tk, _ := Lgcc.GerarToken(16)
					motoristasOnline[tk] = m.Nome
					header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
					Lgcc.EscritaResp(conn, header, nil)
				} else {
					Lgcc.GeraRespostaErro(999, conn, "")
					return
				}
			},
			"/passageiro/listarCorridasCadastradas": func(conn net.Conn, dados2 []byte, tk string) {
			},
			"/passageiro/reservarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
			},
			"/passageiro/cancelarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				
			},
			"/motorista/cadastrarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
			},
			"/motorista/cancelarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
			},
		}
	})
	return mux
}