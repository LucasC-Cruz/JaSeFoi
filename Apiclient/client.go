package Apiclient

import (
	"Golang/Lgcc"
	"Golang/Model"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	Conn     net.Conn
	Token    string
	HTTPBase string // URL base HTTP do servidor, ex: "http://localhost:8871"
	httpc    *http.Client
}

// Dial conecta via TCP (protocolo Lgcc) e guarda a URL base HTTP para chamadas diretas.
// addrTCP: "localhost:8741"  |  addrHTTP (opcional): "http://localhost:8871"
func Dial(addrTCP string, addrHTTP ...string) (*Client, error) {
	con, err := net.DialTimeout("tcp", addrTCP, 5*time.Second)
	if err != nil {
		return nil, err
	}
	base := ""
	if len(addrHTTP) > 0 {
		base = strings.TrimRight(addrHTTP[0], "/")
	}
	return &Client{
		Conn:     con,
		HTTPBase: base,
		httpc:    &http.Client{Timeout: 35 * time.Second},
	}, nil
}

//isso na verdade é um método
func (c *Client) AuthPassageiro(nome, senha string) (string, string, error) {
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

func (c *Client) LogoutPassageiro(tk string) (string, error) {
	dadinhos, err := Lgcc.Post("/logout/passageiro", "true", nil, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "Passageiro deslogado com sucesso!", nil
	}
	return "Erro ao deslogar", fmt.Errorf("falha ao deslogar")
}

func (c *Client) AuthMotorista(nome, senha string) (string, string, error) {
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
		return "Motorista logado com sucesso", dadinhos.Headers["Token"], nil
	} else {
		return "Senha ou usuário incorreto", "", errors.New("erro")
	}
}

func (c *Client) LogoutMotorista(tk string) (string, error) {
	dadinhos, err := Lgcc.Post("/logout/motorista", "true", nil, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		return "Motorista deslogado com sucesso!", nil
	}
	return "Erro ao deslogar", fmt.Errorf("falha ao deslogar")
}

func (c *Client) RegistraMotorista(nome, senha, carro string) (string, error) {
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

func (c *Client) RegistraPassageiro(nome, senha string) (string, error) {
	var p Model.Passageiro
	var dadinhos *Lgcc.Token
	p.Nome = nome
	p.Senha = senha
	usuario, err := json.Marshal(p)
	if err != nil {
		fmt.Println("Erro no registra Passageiro APIClient")
	}

	dadinhos, err = Lgcc.Post("/cadastro/passageiro", "true", usuario, c.Conn.(*net.TCPConn), "")
	fmt.Println(dadinhos.Headers)
	if dadinhos.Headers["Status-Code"] == "200" {
		return "\nPassageiro cadastrado com sucesso\n", nil
	}
	return "\nerro ou usuário passageiro não existe\n", fmt.Errorf("falha")
}

func (c *Client) ExibeCoisa(tela string) (string, error) {
	tokas, err := Lgcc.Get(tela, "true", c.Conn.(*net.TCPConn), "")
	if err != nil {
		return "\nErro na requisição da tela de login\n", err
	}
	return string(tokas.Body), nil
}

func (c *Client) CadastrarCorrida(rota, data string, assentos int, valorTrecho float32, tk string, flag bool) (string, error) {
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
	if !flag {
		dadinhos, err = Lgcc.Post("/motorista/tentaCadastrarCorrida", "true", carona, c.Conn.(*net.TCPConn), tk)
		if err != nil {
			return "\nErro ao comunicar com o servidor\n", err
		}
		if dadinhos.Headers["Status-Code"] == "200" {
			return "\n[1] Confirmar cadastro\n[0] Cancelar cadastro", nil
		}
		if len(dadinhos.Body) > 0 && string(dadinhos.Body) != "erro" {
			return fmt.Sprintf("\n%s\n", string(dadinhos.Body)), fmt.Errorf("falha: %s", string(dadinhos.Body))
		}
		return "\nNão existe a possibilidade de cadastrar esta corrida\n", fmt.Errorf("falha")
	} else {
		fmt.Println("Entrei na condição")
		dadinhos, err = Lgcc.Post("/motorista/cadastrarCorrida", "true", carona, c.Conn.(*net.TCPConn), tk)
		if err != nil {
			return "\nErro ao comunicar com o servidor\n", err
		}
		if dadinhos.Headers["Status-Code"] == "200" {
			return "\n Corrida cadastrada com sucesso\n", nil
		}
		if len(dadinhos.Body) > 0 && string(dadinhos.Body) != "erro" {
			return fmt.Sprintf("\n%s\n", string(dadinhos.Body)), fmt.Errorf("falha: %s", string(dadinhos.Body))
		}
		return "\nerro no registra motorista\n", fmt.Errorf("falha")
	}
}

func (c *Client) CancelarCorridaMotorista(tk string) (string, error) {
	if c.HTTPBase != "" {
		httpReq, err := http.NewRequest(http.MethodPost, c.HTTPBase+"/motorista/cancelar-corrida", nil)
		if err != nil {
			return "", err
		}
		httpReq.Header.Set("Authorization", "Bearer "+tk)
		resp, err := c.httpc.Do(httpReq)
		if err != nil {
			return "\nErro ao cancelar corrida no servidor.\n", err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return "\nCorrida cancelada com sucesso!\n", nil
		}
		return "\nErro ao cancelar corrida\n", fmt.Errorf("falha")
	}
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
	// Se HTTPBase está configurado, usa HTTP diretamente
	if c.HTTPBase != "" {
		params := url.Values{}
		params.Set("origem", origem)
		params.Set("destino", destino)
		params.Set("data", data)
		req, err := http.NewRequest(http.MethodGet, c.HTTPBase+"/passageiro/buscar-rotas?"+params.Encode(), nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+tk)
		resp, err := c.httpc.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, "\nErro ao buscar rotas (Nenhuma rota encontrada)\n", fmt.Errorf("rota não encontrada")
		}
		var rotas [][]Model.Aresta
		if err := json.NewDecoder(resp.Body).Decode(&rotas); err != nil {
			return nil, "", fmt.Errorf("erro ao deserializar rotas: %w", err)
		}
		if len(rotas) == 0 {
			return nil, "\nNenhuma rota disponível encontrada para essa data.\n", fmt.Errorf("nenhuma rota")
		}
		return rotas, FormatMenuRotas(rotas), nil
	}
	// Fallback TCP (protocolo Lgcc - servidor TCP atua como intermediário para o servidor HTTP)
	var pa Model.InfoPassagem
	pa.Origem = origem
	pa.Destino = destino
	pa.Data = data
	body, err := json.Marshal(pa)
	if err != nil {
		return nil, "", err
	}
	dadinhos, err := Lgcc.Post("/passageiro/buscaRotasPassagem", "true", body, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return nil, "", err
	}
	if dadinhos.Headers["Status-Code"] == "200" {
		var rotas [][]Model.Aresta
		if err := json.Unmarshal(dadinhos.Body, &rotas); err != nil {
			return nil, "", fmt.Errorf("erro ao deserializar rotas: %w", err)
		}
		if len(rotas) == 0 {
			return nil, "\nNenhuma rota disponível encontrada para essa data.\n", fmt.Errorf("nenhuma rota")
		}
		return rotas, FormatMenuRotas(rotas), nil
	}
	return nil, "\nErro ao buscar rotas (Nenhuma rota encontrada)\n", fmt.Errorf("rota não encontrada")
}

func (c *Client) ConfirmarReserva(rota []Model.Aresta, confirma bool, tk string) (string, error) {
	return c.TentaReserva(rota, confirma, tk)
}

func (c *Client) TentaReserva(rota []Model.Aresta, confirma bool, tk string) (string, error) {
	req := Model.InfoConfirmarReserva{
		Rota:        rota,
		Confirma:    confirma,
		Cancelar:    false,
		TkAssociado: tk,
	}
	return c.enviarReqReserva(req, tk)
}

func (c *Client) CancelarReservaPendente(rota []Model.Aresta, tk string) (string, error) {
	req := Model.InfoConfirmarReserva{
		Rota:        rota,
		Confirma:    false,
		Cancelar:    true,
		TkAssociado: tk,
	}
	return c.enviarReqReserva(req, tk)
}

func (c *Client) enviarReqReserva(req Model.InfoConfirmarReserva, tk string) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	// Se HTTPBase está configurado, usa HTTP diretamente
	if c.HTTPBase != "" {
		httpReq, err := http.NewRequest(http.MethodPost, c.HTTPBase+"/passageiro/tentar-reserva", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+tk)
		resp, err := c.httpc.Do(httpReq)
		if err != nil {
			return "\nErro ao reservar passagem no servidor.\n", err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			if req.Cancelar {
				return "\nReserva cancelada. A rota foi liberada para outros passageiros.\n", nil
			}
			if req.Confirma {
				return "\nPassagem confirmada com sucesso!\n", nil
			}
			return "\nAssentos travados! Confirme ou cancele em até 10s.\n", nil
		}
		if resp.StatusCode == 409 {
			return "\nAssentos indisponíveis.\n", fmt.Errorf("conflito")
		}
		if resp.StatusCode == 408 || resp.StatusCode == 410 {
			return "\nTimeout: você não confirmou em 10s e a rota foi liberada.\n", fmt.Errorf("timeout")
		}
		return "\nErro ao reservar passagem no servidor.\n", fmt.Errorf("falha na reserva")
	}
	// Via TCP (servidor TCP atua como intermediário para o servidor HTTP)
	dadinhos, err := Lgcc.Post("/passageiro/tentaReserva", "true", body, c.Conn.(*net.TCPConn), tk)
	if err != nil {
		return "", err
	}
	statusCode := dadinhos.Headers["Status-Code"]
	if statusCode == "200" {
		if req.Cancelar {
			return "\nReserva cancelada. A rota foi liberada para outros passageiros.\n", nil
		}
		if req.Confirma {
			return "\nPassagem confirmada com sucesso!\n", nil
		}
		return "\nAssentos travados! Confirme ou cancele em até 10s.\n", nil
	}
	if statusCode == "409" {
		return "\nAssentos indisponíveis.\n", fmt.Errorf("conflito")
	}
	if statusCode == "408" || statusCode == "410" {
		return "\nTimeout: você não confirmou em 10s e a rota foi liberada.\n", fmt.Errorf("timeout")
	}
	return "\nErro ao reservar passagem no servidor.\n", fmt.Errorf("falha na reserva")
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
	if c.HTTPBase != "" {
		httpReq, err := http.NewRequest(http.MethodPost, c.HTTPBase+"/passageiro/cancelar-passagem", nil)
		if err != nil {
			return "", err
		}
		httpReq.Header.Set("Authorization", "Bearer "+tk)
		resp, err := c.httpc.Do(httpReq)
		if err != nil {
			return "\nErro ao cancelar passagem no servidor.\n", err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return "\nPassagem cancelada com sucesso!\n", nil
		}
		return "\nErro ao cancelar passagem (Nenhuma passagem ativa encontrada)\n", fmt.Errorf("falha ao cancelar")
	}
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
