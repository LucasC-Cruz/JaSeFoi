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
	checkMotoristas      map[string]Model.InfoLoginMotorista
	checkPassageiros     map[string]Model.InfoLoginPassageiro
	motoristas           map[string]Model.Motorista
	mapaTrechos          map[string]Model.Trecho
	motoristasOnline     map[string]string
	passageirosOnline    map[string]string
	grafo                *Model.Grafo
	caronasPassageiros   map[string]Model.CaronaPassageiro


	caronasPassageirosMu  sync.Mutex
	motoristasOnlineMu    sync.Mutex
	mutexCheckMotoristas  sync.Mutex
	mutexCheckPassageiros sync.Mutex
	mutexMapaTrecho       sync.Mutex
	caronasMu    		  sync.Mutex
	passageiroMu 		  sync.Mutex
	motoristaMu  		  sync.Mutex
	grafoMu      		  sync.Mutex


	once         		 sync.Once
	once2        		 sync.Once
	once3        		 sync.Once
	once4        		 sync.Once
	once5        		 sync.Once
	once6        		 sync.Once
	once7        		 sync.Once
	once8        		 sync.Once
	once9        		 sync.Once
	once10        		 sync.Once
	
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
	iniciarArqCaronasPassageiros()
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

func iniciarArqTrechos() {
	once9.Do(func() {
		dados, err := os.ReadFile("../data/trechos.json")
		if err != nil {
			fmt.Println("Erro ao ler arquivo trechos.json", err)
			os.Exit(1)
		}
		err = json.Unmarshal(dados, &mapaTrechos)
		if err != nil {
			fmt.Println("Erro na conversão de json para trechos")
			os.Exit(1)
		}
	})
}

func iniciarArqCheckMotoristas() {
	once8.Do(func() {
		dados, err := os.ReadFile("../data/checkMotoristas.json")
		if err != nil {
			fmt.Println("Erro ao ler arquivo checkMotoristas.json", err)
			os.Exit(1)
		}
		err = json.Unmarshal(dados, &checkMotoristas)
		if err != nil {
			fmt.Println("Erro na conversão de json para map check motoristas")
			os.Exit(1)
		}
	})
}


func iniciarArqCheckPassageiro() {
	once10.Do(func() {
		dados, err := os.ReadFile("../data/checkPassageiros.json")
		if err != nil {
			fmt.Println("Erro ao ler arquivo checkPassageiros.json", err)
			os.Exit(1)
		}
		err = json.Unmarshal(dados, &checkPassageiros)
		if err != nil {
			fmt.Println("Erro na conversão de json para map check passageiros")
			os.Exit(1)
		}
	})
}


func iniciarArqGrafo() {
	once5.Do(func() {
		if grafo == nil {
			grafo = Model.NovoGrafo()
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
					fmt.Println("Deu erro aqui")
					checkPassageiros[p.Nome] = Model.InfoLoginPassageiro{Nome: p.Nome, Senha: p.Senha, Id: p.Id}
					fmt.Println("Deu erro aqui 12")
					dados, _ := json.Marshal(passageiros)
					dados2, _ := json.Marshal(checkPassageiros)
					os.WriteFile("../data/Passageiro.json", dados, 0644)
					os.WriteFile("../data/checkPassageiros.json", dados2, 0644)
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
					os.WriteFile("../data/Motoristas.json", dados, 0644)
					os.WriteFile("../data/checkMotoristas.json", dados2, 0644)
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
				a, existe := checkPassageiros[p.Nome]
				if existe {
					if a.Senha == p.Senha {
						tk, _ := Lgcc.GerarToken(16)
						passageirosOnline[tk] = a.Id
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
					if a.Senha == m.Senha{
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
			"/passageiro/listarCorridasCadastradas": func(conn net.Conn, dados2 []byte, tk string) {
			},
			"/passageiro/reservarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				
			},
			"/passageiro/cancelarPassagem": func(conn net.Conn, dados2 []byte, tk string) {
				
			},"/motorista/tentaCadastrarCorrida":func(conn net.Conn, dados2 []byte, tk string) {
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
				if err !=nil{
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
				os.WriteFile("../data/Motoristas.json", dados, 0644)
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
				if err != nil{
					fmt.Println("Erro ao converter grafo para json")
				}
				//Por algum motivo os Trechos não estão sendo salvos
				os.WriteFile("../data/grafo.json", grafito, 0644)
				grafoMu.Unlock()
				fmt.Println("Grafo atualizado com sucesso!")


				mutexMapaTrecho.Lock()
				for i:= range trechos{
					mapaTrechos[trechos[i].Id] = trechos[i]
				}
		 
				a, err := json.Marshal(mapaTrechos)
				os.WriteFile("../data/trechos.json", a, 0644)
				mutexMapaTrecho.Unlock()

				header := Lgcc.GeraHeaderRespPost("OK", 200, tk)
				Lgcc.EscritaResp(conn, header, nil)
			},
			"/motorista/cancelarCorrida": func(conn net.Conn, dados2 []byte, tk string) {
			},
		}
	})
	return mux
}