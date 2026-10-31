package Apiclient

import (
	"Golang/Lgcc"
	"Golang/Model"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type Client struct {
	Conn  net.Conn
	Token string
}

func Dial(addr string) (*Client, error) {
	con, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{
		Conn: con,
	}, nil
}

//isso na verdade é um método
func (c *Client) AuthPassageiro (nome, senha string) (string, string, error) {
	var p Model.InfoLoginPassageiro
	var dadinhos *Lgcc.Token
	p.Nome = nome
	p.Senha = senha
	usuario, err := json.Marshal(p)
	if err != nil {
		fmt.Println("erro no auth passageiro")
	}
	dadinhos, err = Lgcc.Post("/login/passageiro", "true", usuario, c.Conn.(*net.TCPConn), "") 
	if err != nil {
		return "erro post APIClient", "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "Passageiro logado com sucesso!", dadinhos.Headers["Token"], nil
	}
	return "Passageiro não foi logado com sucesso", "", errors.New("erro")
}

func (c *Client) AuthMotorista (nome, senha string) (string, string, error) {
	var m Model.InfoLoginMotorista
	var dadinhos *Lgcc.Token
	m.Nome = nome
	m.Senha = senha
	usuario, err := json.Marshal(m)
	if err != nil {
		fmt.Println("Erro no authMotorista")
	}
	dadinhos, err = Lgcc.Post("/login/motorista", "true", usuario, c.Conn.(*net.TCPConn), "") 
	if err != nil {
		return "erro post APIClient", "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "Motorista logado com sucesso", dadinhos.Headers["Token"],nil
	} else {
		return "Senha ou usuário incorreto", "", errors.New("erro")
	}
	
	
}

func (c *Client) RegistraMotorista (nome, senha, carro string) (string, error) {
	var m Model.Motorista
	var dadinhos *Lgcc.Token
	m.Nome = nome
	m.Senha = senha
	m.Carro = carro
	m.Id = ""
	usuario, err := json.Marshal(m)
	if err != nil {
		fmt.Println("Erro no registraMotorista ApiClient")
	}
	dadinhos, err = Lgcc.Post("/cadastro/motorista", "true", usuario, c.Conn.(*net.TCPConn), "")
	if dadinhos.Headers["Status-Code"] == "200" {
		return "\nMotorista cadastrado com sucesso bb\n", nil
	}
	return "\nerro no registra motorista\n", err
}

func (c *Client) RegistraPassageiro (nome, senha string) (string, error) {
	var p Model.Passageiro
	var dadinhos *Lgcc.Token
	p.Nome = nome
	p.Senha = senha
	usuario, err := json.Marshal(p)
	if err != nil {
		fmt.Println("Erro no registra Passageiro APIClient")
	}
	dadinhos, err = Lgcc.Post("/cadastro/passageiro", "true", usuario, c.Conn.(*net.TCPConn), "")
	if dadinhos.Headers["Status-Code"] == "200" {
		return "\nPassageiro cadastrado com sucesso\n", nil
	}
	return "\nerro ou usuário passageiro não existe\n", fmt.Errorf("falha")
}

func (c *Client) ExibeCoisa(tela string) (string, error){
	tokas, err := Lgcc.Get(tela, "true", c.Conn.(*net.TCPConn), "")
	if err != nil{
		return "\nErro na requisição da tela de login\n", err
	}
	return string(tokas.Body), nil
}

func (c *Client) CadastrarCorrida(rota, data string, assentos int, valorTrecho float32, tk string, flag bool) (string, error){
	var ca Model.InfoCarona
	var dadinhos *Lgcc.Token
	ca.Data = data
	ca.ValorTrecho = valorTrecho
	ca.Assentos = assentos
	ca.Rota = rota
	carona, err := json.Marshal(ca)
	if err != nil {
		fmt.Println("Erro no registraMotorista ApiClient")
	}
	if !flag{
		dadinhos, err = Lgcc.Post("/motorista/tentaCadastrarCorrida", "true", carona, c.Conn.(*net.TCPConn), tk)
		if dadinhos.Headers["Status-Code"] == "200" {
			return "\n[1] Confirmar cadastro\n[0] Cancelar cadastro", nil
		}
		return "\nNão existe a possibilidade de cadastrar esta corrida\n", fmt.Errorf("falha")
	}else{
		fmt.Println("Entrei na condição")
		dadinhos, err = Lgcc.Post("/motorista/cadastrarCorrida", "true", carona, c.Conn.(*net.TCPConn), tk)
		if dadinhos.Headers["Status-Code"] == "200" {
			return "\n Corrida cadastrada com sucesso\n", nil
		}
		return "\nerro no registra motorista\n", fmt.Errorf("falha")
	}
}


func (c *Client) CancelarCorridaMotorista(tk string) (string, error) {
	dadinhos, err := Lgcc.Post("/motorista/cancelarCorrida", "true", nil, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "\nCorrida cancelada com sucesso!\n", nil
	}
	return "\nErro ao cancelar corrida\n", fmt.Errorf("falha")
}

func (c *Client) BuscarRotas(origem, destino, data string, tk string) ([][]Model.Aresta, string, error) {
	var pa Model.InfoPassagem
	pa.Origem = origem
	pa.Destino = destino
	pa.Data = data
	body, err := json.Marshal(pa)
	if err != nil {
		return nil, "", err
	}

	dadinhos, err := Lgcc.Post("/passageiro/tentaReservarPassagem", "true", body, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return nil, "", err
	}

	if dadinhos.Headers["Status-Code"] == "200" {
		var rotas [][]Model.Aresta
		err := json.Unmarshal(dadinhos.Body, &rotas)
		if err != nil {
			return nil, "", fmt.Errorf("erro ao deserializar rotas: %w", err)
		}
		if len(rotas) == 0 {
			return nil, "\nNenhuma rota disponível encontrada para essa data.\n", fmt.Errorf("nenhuma rota")
		}
		texto := FormatMenuRotas(rotas)
		return rotas, texto, nil
	}
	return nil, "\nErro ao buscar rotas (Nenhuma rota encontrada)\n", fmt.Errorf("rota não encontrada")
}

func (c *Client) ConfirmarReserva(rota []Model.Aresta, data string, tk string) (string, error) {
	req := Model.InfoConfirmarReserva{
		Data: data,
		Rota: rota,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	dadinhos, err := Lgcc.Post("/passageiro/reservarPassagem", "true", body, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}

	if dadinhos.Headers["Status-Code"] == "200" {
		return "\nPassagem reservada com sucesso!\n", nil
	}
	return "\nErro ao reservar passagem no servidor.\n", fmt.Errorf("falha na reserva")
}

func (c *Client) ReservarPassagem(origem, destino, data string, tk string, flag bool) (string, error) {
	if !flag {
		_, texto, err := c.BuscarRotas(origem, destino, data, tk)
		return texto, err
	}
	rotas, _, err := c.BuscarRotas(origem, destino, data, tk)
	if err != nil || len(rotas) == 0 {
		return "\nNenhuma rota disponível.\n", fmt.Errorf("falha")
	}
	return c.ConfirmarReserva(rotas[0], data, tk)
}

func FormatMenuRotas(rotas [][]Model.Aresta) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n========== Rotas Disponíveis (%d encontrada(s)) ==========\n", len(rotas)))
	for i, rota := range rotas {
		sb.WriteString(fmt.Sprintf("[%d] ", i+1))
		for j, aresta := range rota {
			if j == 0 {
				sb.WriteString(aresta.Origem)
			}
			sb.WriteString(" -> ")
			sb.WriteString(aresta.Destino)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("[0] Cancelar operação\n")
	return sb.String()
}

func (c *Client) ListarCorridasPassageiro(tk string) (string, error) {
	dadinhos, err := Lgcc.Get("/passageiro/listarCorridasCadastradas", "true", c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return string(dadinhos.Body), nil
	}
	return "Erro ao listar corridas", fmt.Errorf("falha ao listar")
}

func (c *Client) CancelarPassagem(tk string) (string, error) {
	dadinhos, err := Lgcc.Post("/passageiro/cancelarPassagem", "true", nil, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "\nPassagem cancelada com sucesso!\n", nil
	}
	return "\nErro ao cancelar passagem (Nenhuma passagem ativa encontrada)\n", fmt.Errorf("falha ao cancelar")
}

func (c *Client) Close() error { return c.Conn.Close() }

func formatarRotas(rotas [][]Model.Aresta) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n========== Rotas Disponíveis (%d encontradas) ==========\n", len(rotas)))
	for i, rota := range rotas {
		sb.WriteString(fmt.Sprintf("\n--- Rota %d ---\n", i+1))
		for j, aresta := range rota {
			if j == 0 {
				sb.WriteString(aresta.Origem)
			}
			sb.WriteString(" -> ")
			sb.WriteString(aresta.Destino)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n[1] Confirmar reserva    [0] Cancelar reserva\n")
	return sb.String()
}
