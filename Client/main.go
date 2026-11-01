package main

import (
	"Golang/Apiclient"
	"flag"
	"fmt"
	"os"
)

const (
	HOST = "localhost"
	PORT = "6741"
	TYPE = "tcp"
)

func main() {
	addr := flag.String("addr", "localhost:6741", "endereço do servidor (host:porta)")
	flag.Parse()

	c, err := Apiclient.Dial(*addr)
	if err != nil {
		fmt.Println("erro ao conectar:", err)
		os.Exit(1)
	}
	defer c.Close()

	var opcao string
	var nome string
	var senha string
	var carro string
	var tk string

MenuPrincipal:
	for {
		front, err := c.ExibeCoisa("/")
		if err != nil {
			fmt.Println(err)
			break MenuPrincipal
		}
		fmt.Print(front)
		fmt.Println()
		fmt.Scan(&opcao)

		switch opcao {
		// ==========================================
		// 1. LOGIN PASSAGEIRO
		// ==========================================
		case "1":
			front, err := c.ExibeCoisa("/tela/login")
			if err != nil {
				fmt.Println(err)
				continue MenuPrincipal
			}
			fmt.Print(front)
			fmt.Scan(&nome)
			fmt.Scan(&senha)

			front, tk, err = c.AuthPassageiro(nome, senha)
			if err != nil {
				fmt.Println(front)
				fmt.Println(err)
				continue MenuPrincipal
			}

			fmt.Println("\nLogin efetuado com sucesso!")

		MenuPassageiro:
			for {
				fmt.Println("\n--- ÁREA DO PASSAGEIRO ---")
				fmt.Println("[1] Listar corridas cadastradas")
				fmt.Println("[2] Reservar uma passagem")
				fmt.Println("[3] Cancelar passagem")
				fmt.Println("[0] Deslogar / Voltar")
				
				var subOpcao string
				fmt.Scan(&subOpcao)

				switch subOpcao {
				case "1":
					fmt.Println("Listando suas corridas cadastradas...")
					res, err := c.ListarCorridasPassageiro(tk)
					if err != nil {
						fmt.Println("Erro:", err)
					} else {
						fmt.Println("Seus trechos reservados:")
						fmt.Println(res)
					}
					
				case "2":
					var origem, destino, data string
					var escolha int

					fmt.Println("Digite a Origem:")
					fmt.Scan(&origem)

					fmt.Println("Digite o Destino:")
					fmt.Scan(&destino)

					fmt.Println("Digite a Data (ex: 20/10):")
					fmt.Scan(&data)

					rotas, menu, err := c.BuscarRotas(origem, destino, data, tk)
					if err != nil {
						fmt.Println(menu)
						fmt.Println("Erro:", err)
						continue MenuPassageiro
					}
					fmt.Println(rotas)

					fmt.Print(menu)
					fmt.Print("\nDigite o número da rota desejada: >> ")
					fmt.Scan(&escolha)

					if escolha <= 0 || escolha > len(rotas) {
						fmt.Println("Operação cancelada ou opção inválida.")
						continue MenuPassageiro
					}

					rotaEscolhida := rotas[escolha-1]
					msg, err := c.ConfirmarReserva(rotaEscolhida, data, tk)
					if err != nil {
						fmt.Println("Erro:", err)
					} else {
						fmt.Println(msg)
					}
					continue MenuPassageiro

				case "3":
					fmt.Println("Cancelando passagem...")
					msg, err := c.CancelarPassagem(tk)
					if err != nil {
						fmt.Println("Erro:", err)
					} else {
						fmt.Println(msg)
					}
				case "0":
					fmt.Println("Deslogando...")
					c.LogoutPassageiro(tk)
					break MenuPassageiro
				default:
					fmt.Println("Opção inválida.")
				}
			}

		// ==========================================
		// 2. LOGIN MOTORISTA
		// ==========================================
		case "2":
			front, err := c.ExibeCoisa("/tela/login")
			if err != nil {
				fmt.Println(err)
				continue MenuPrincipal
			}
			fmt.Print(front)
			fmt.Scan(&nome)
			fmt.Scan(&senha)

			front, tk, err = c.AuthMotorista(nome, senha)
			if err != nil {
				fmt.Println(front)
				continue MenuPrincipal
			}

			fmt.Println("\nLogin efetuado com sucesso!")

		MenuMotorista:
			for {
				fmt.Println("\n--- ÁREA DO MOTORISTA ---")
				fmt.Println("[1] Cadastrar Corrida")
				fmt.Println("[2] Iniciar Corrida")
				fmt.Println("[3] Cancelar corrida")
				fmt.Println("[4] Finalizar corrida")
				fmt.Println("[0] Deslogar / Voltar")
				
				var subOpcao string
				fmt.Scan(&subOpcao)

				switch subOpcao {
				case "1":
					var rota, data string
					var assentos int
					var valorTrecho float32
					var confirmacao string

					front, err := c.ExibeCoisa("/tela/menuMotorista")
					if err != nil {
						fmt.Println("Erro no print:", err)
					}
					fmt.Print(front)

					front, err = c.ExibeCoisa("/tela/menuMotorista/cidades")
					if err != nil {
						fmt.Println("Erro no print:", err)
					}
					fmt.Print(front)

					fmt.Print("\nRota: >> ")
					fmt.Scan(&rota)
					fmt.Print("Data: >> ")
					fmt.Scan(&data)
					fmt.Print("Número de assentos: >> ")
					fmt.Scan(&assentos)
					fmt.Print("Valor do trecho: >> ")
					fmt.Scan(&valorTrecho)

					front , err = c.CadastrarCorrida(rota, data, assentos, valorTrecho, tk, false)
					if err != nil {
						fmt.Print(front)
						continue MenuMotorista
					}

					fmt.Print(front)
					fmt.Println("")
					fmt.Scan(&confirmacao)
					if confirmacao != "1"{
						fmt.Println("Operação cancelada")
						
					} else{
						fmt.Println(rota, data, assentos, valorTrecho)
						front , err = c.CadastrarCorrida(rota, data, assentos, valorTrecho, tk, true)
						fmt.Println(front)
						continue MenuMotorista
					}

				case "2":
					fmt.Println("Iniciando corrida...")
				case "3":
					fmt.Println("Cancelando corrida...")
					msg, err := c.CancelarCorridaMotorista(tk)
					if err != nil {
						fmt.Println("Erro:", err)
					} else {
						fmt.Println(msg)
					}
				case "4":
					fmt.Println("Finalizando corrida...")
				case "0":
					fmt.Println("Deslogando...")
					c.LogoutMotorista(tk)
					break MenuMotorista
				default:
					fmt.Println("Opção inválida.")
				}
			}

		// ==========================================
		// 3. CADASTRO PASSAGEIRO
		// ==========================================
		case "3":
			front, err := c.ExibeCoisa("/tela/cadastroPassageiro")
			if err != nil {
				fmt.Println(err)
				continue MenuPrincipal
			}
			fmt.Print(front)
			fmt.Scan(&nome)
			fmt.Scan(&senha)
			
			front, err = c.RegistraPassageiro(nome, senha)
			if err != nil {
				fmt.Println(front)
				fmt.Println(err)
			} else {
				fmt.Print(front)
			}

		// ==========================================
		// 4. CADASTRO MOTORISTA
		// ==========================================
		case "4":
			front, err := c.ExibeCoisa("/tela/cadastroMotorista")
			if err != nil {
				fmt.Println(err)
				continue MenuPrincipal
			}
			fmt.Print(front)
			fmt.Scan(&nome)
			fmt.Scan(&senha)
			fmt.Scan(&carro)
			
			front, err = c.RegistraMotorista(nome, senha, carro)
			if err != nil {
				fmt.Println(front)
				fmt.Println(err)
			} else {
				fmt.Print(front)
			}

		// ==========================================
		// 0. SAIR DO APP
		// ==========================================
		case "0":
			fmt.Println("Saindo da aplicação...")
			break MenuPrincipal

		default:
			fmt.Println("Entrada Inválida. Tente novamente.")
		}
	}
}