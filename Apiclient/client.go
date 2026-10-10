package Apiclient

import (
	"encoding/json"
	"net"
	"time"
	"fmt"
	"Golang/Lgcc"
	"Golang/Model"
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

func (c *Client) AuthPassageiro (nome, senha string) (string, string, error) {
	var p Model.Passageiro
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
	return "Passageiro não foi logado com sucesso", "", nil
}

func (c *Client) AuthMotorista (nome, senha string) (string, string, error) {
	var m Model.Motorista
	var dadinhos *Lgcc.Token
	m.Nome = nome
	m.Senha = senha
	usuario, err := json.Marshal(m)
	if err != nil {
		fmt.Println("Cuzinho")
	}
	dadinhos, err = Lgcc.Post("/login/motorista", "true", usuario, c.Conn.(*net.TCPConn), "") 
	if err != nil {
		return "erro post APIClient", "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "Motorista logado com sucesso bb", dadinhos.Headers["Token"],nil
	}
	return "erro ou usuário motorista não existe", "", nil
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
		return "\nPassageiro cadastrado com sucesso bb\n", nil
	}
	return "\nerro ou usuário passageiro não existe\n", err
}

func (c *Client) ExibeCoisa(tela string) (string, error){
	tokas, err := Lgcc.Get(tela, "true", c.Conn.(*net.TCPConn), "")
	if err != nil{
		return "\nErro na requisição da tela de login\n", err
	}
	return string(tokas.Body), nil
}

func (c *Client) CadastrarCorrida(rota, data, horario string, assentos int, valorTrecho float32, tk string) (string, error){
	var ca Model.InfoCarona
	var dadinhos *Lgcc.Token
	ca.Data = data
	ca.Horario = horario
	ca.ValorTrecho = valorTrecho
	ca.Assentos = assentos
	ca.Rota = rota
	carona, err := json.Marshal(ca)
	if err != nil {
		fmt.Println("Erro no registraMotorista ApiClient")
	}
	dadinhos, err = Lgcc.Post("/motorista/cadastrarCorrida", "true", carona, c.Conn.(*net.TCPConn), tk)
	if dadinhos.Headers["Status-Code"] == "200" {
		return "\n Grafo cadastrado com sucesso bb\n", nil
	}
	return "\nerro no registra motorista\n", err
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

func (c *Client) ReservarPassagem(origem, destino, data string, tk string) (string, error) {
	info := map[string]string{
		"origem": origem,
		"destin": destino,
		"data":   data,
	}
	body, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	dadinhos, err := Lgcc.Post("/passageiro/reservarPassagem", "true", body, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}

	if dadinhos.Headers["Status-Code"] == "200" {
		return fmt.Sprintf("\nPassagem reservada com sucesso! ID da Reserva: %s\n", dadinhos.Headers["Token"]), nil
	}
	return "\nErro ao reservar passagem (Rota não encontrada ou sem assentos)\n", fmt.Errorf("falha na reserva")
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