package Lgcc

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"crypto/rand"
	"encoding/hex"	
)

type Token struct {
	Headers map[string]string
	Body    []byte //io.ReaderClose() tem que ser essa aq
}

// recebe também a conexão tcp já estabelecida
// recebemos do servidor um ponteiro para uma struct do tipo resposta
// as saídas são o ponteiro do tipo resposta para manipulação principalmente do bod3y
// e um possível erro
// get recebe body como nil por padrão
func Get(caminho, keep string, conn *net.TCPConn, token string) (*Token, error) {

	header := GeraHeaderReq(caminho, "get", keep, token)
	err := EscritaResp(conn, header, nil)
	if err != nil {
		fmt.Println("Não conseguimos escrever ao servidor :(, err " + err.Error())
		return nil, err
	}

	//vamos simplesmente mandar uma string que vai conter o caminho do dado
	reader := bufio.NewReader(conn)
	headers := make(map[string]string)

	headers, err = OneForAllParser(reader, headers)
	if err != nil {
		fmt.Println("Deu ruim o parser")
	}
	n, _ := strconv.Atoi(headers["Tamanho-Conteudo"])
	body := make([]byte, n)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}

	return &Token{Headers: headers, Body: body}, nil
}

func Post(caminho string, keep string, corpo []byte, conn *net.TCPConn, token string) (*Token, error) {

	body, tamain, tipo, err := GeraBody(corpo)
	if err != nil {
		fmt.Println("Problema no post na hr de converter o body")
	}

	header := GeraHeaderReqPost(caminho, "post", keep, tipo, tamain, token)
	err = EscritaResp(conn, header, body)
	if err != nil {
		fmt.Println("Não conseguimos escrever ao servidor :(, err " + err.Error())
		return nil, err
	}

	reader := bufio.NewReader(conn)
	headers := make(map[string]string)

	headers, err = OneForAllParser(reader, headers)
	if err != nil {
		fmt.Println("Deu ruim o parser")
		return nil, err
	}

	var respBody []byte
	if tamStr, ok := headers["Tamanho-Conteudo"]; ok && tamStr != "" {
		n, _ := strconv.Atoi(tamStr)
		if n > 0 {
			respBody = make([]byte, n)
			if _, err := io.ReadFull(reader, respBody); err != nil {
				return nil, err
			}
		}
	}

	return &Token{Headers: headers, Body: respBody}, nil
}
func OneForAllParser(reader *bufio.Reader, headers map[string]string) (map[string]string, error) {
	//jeito IAônico faraônico, possivelmente há um jeito melhor de fazer isso daqui
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[parts[0]] = parts[1]
		}
	}

	return headers, nil
}

// Quando o cliente vai solicitar algo através de um get ele pede por esse header aqui
// Header de get do cliente
func GeraHeaderReq(caminho, protocolo, keepAlive, token string) []byte {
	var sb strings.Builder
	sb.WriteString("Caminho:")
	sb.WriteString(caminho)
	sb.WriteString("\n")
	sb.WriteString("Protocolo:")
	sb.WriteString(protocolo)
	sb.WriteString("\n")
	sb.WriteString("KeepAlive:")
	sb.WriteString(keepAlive)
	sb.WriteString("\n")
	sb.WriteString("Token:")
	sb.WriteString(token)
	sb.WriteString("\n")
	sb.WriteString("\n")
	return []byte(sb.String())
}

// Resposta do server para um get feito pelo cliente
func GeraHeaderFull(status string, statusCode int, TipoConteudo string, tamanhoConteudo int, token string) []byte {
	var sb strings.Builder
	strStatusCode := strconv.Itoa(statusCode)
	strTamanhoConteudo := strconv.Itoa(tamanhoConteudo) // <- tamanho REAL, direto do Marshal
	sb.WriteString("Status:")
	sb.WriteString(status)
	sb.WriteString("\n")
	sb.WriteString("Status-Code:")
	sb.WriteString(strStatusCode)
	sb.WriteString("\n")
	sb.WriteString("Tamanho-Conteudo:")
	sb.WriteString(strTamanhoConteudo)
	sb.WriteString("\n")
	sb.WriteString("Tipo-Conteudo:")
	sb.WriteString(TipoConteudo)
	sb.WriteString("\n")
	sb.WriteString("Token:")
	sb.WriteString(token)
	sb.WriteString("\n")
	sb.WriteString("\n")
	return []byte(sb.String())
}

// Quando o client quer fazer um post ele utiliza este header
// header de post do cliente
func GeraHeaderReqPost(caminho, protocolo, keepAlive, TipoConteudo string, tamanhoConteudo int, token string) []byte {
	var sb strings.Builder
	strTamanhoConteudo := strconv.Itoa(tamanhoConteudo)
	sb.WriteString("Caminho:")
	sb.WriteString(caminho)
	sb.WriteString("\n")
	sb.WriteString("Protocolo:")
	sb.WriteString(protocolo)
	sb.WriteString("\n")
	sb.WriteString("KeepAlive:")
	sb.WriteString(keepAlive)
	sb.WriteString("\n")
	sb.WriteString("Tamanho-Conteudo:")
	sb.WriteString(strTamanhoConteudo)
	sb.WriteString("\n")
	sb.WriteString("Tipo-Conteudo:")
	sb.WriteString(TipoConteudo)
	sb.WriteString("\n")
	sb.WriteString("Token:")
	sb.WriteString(token)
	sb.WriteString("\n")
	sb.WriteString("\n")
	return []byte(sb.String())
}

// Servidor utiliza este header para uma devolutiva de post
// Header de resposta do servidor ao cliente para um post feito pelo cliente
func GeraHeaderRespPost(status string, statusCode int, token string) []byte {
	var sb strings.Builder
	strStatusCode := strconv.Itoa(statusCode)

	sb.WriteString("Status:")
	sb.WriteString(status)
	sb.WriteString("\n")
	sb.WriteString("Status-Code:")
	sb.WriteString(strStatusCode)
	sb.WriteString("\n")
	sb.WriteString("Token:")
	sb.WriteString(token)
	sb.WriteString("\n")
	sb.WriteString("\n")
	return []byte(sb.String())
}



// Consertar a geração do erro disso daqui :(
// quero poder aceitar qualquer tipo de conteúdo, string, map, slice, e retornar string
func GeraBody(Body any) ([]byte, int, string, error) {
	switch valor := Body.(type) {
	case string:
		dados := []byte(valor)
		return dados, len(valor), "text/plain", nil
	case []byte:
		return valor, len(valor), "application/json", nil
	default:
		fmt.Println("Erro na asserção de tipo")
	}
	return []byte("Erro"), 1, "erro", nil 
}

// aceitaria o map de string string como header e botaria num buffer e depois mandaria
func EscritaResp(conn net.Conn, header, body []byte) error {
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(body) // ou io.Copy(conn, bodyReader) se body vier de um stream/arquivo
	return err
}

// REFACTOR
func GeraRespostaErro(codigoErro int, conn net.Conn, token string) {
	msgErro := "erro"
	if token != "" {
		msgErro = token
	}
	body, tamanho, tipo, err := GeraBody(msgErro)
	if err != nil {
		fmt.Println(err)
	}
	header := GeraHeaderFull("ERROR", codigoErro, tipo, tamanho, token)
	err = EscritaResp(conn, header, body)
	if err != nil {
		fmt.Println("Erro na escrita de volta do lgcc")
		fmt.Println(err)
	}
}

// GerarToken cria uma string aleatória e segura.
// O parâmetro 'tamanhoBytes' define a força do token. 
// Nota: O tamanho da string final será o dobro desse valor (em hexadecimal).
func GerarToken(tamanhoBytes int) (string, error) {
	// Cria um slice de bytes com o tamanho desejado
	b := make([]byte, tamanhoBytes)
	
	// rand.Read preenche o slice com dados criptograficamente aleatórios
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	
	// Converte os bytes para uma string hexadecimal amigável (a-f, 0-9)
	return hex.EncodeToString(b), nil
}


func DevolutivaGet(dados any, status string, statusCode int, token string, conn net.Conn) error{
	body, tamanho, tipo, err := GeraBody(dados)
	if err != nil{
		fmt.Println("Erro na geração do body")
		return err
	}
	header := GeraHeaderFull(status, statusCode, tipo, tamanho, token)
	err = EscritaResp(conn, header, body)
	if err != nil{
		fmt.Println(" Erro na escrita da resposta na devolutiva do Get ao cliente!")
		return err
	}
	return nil
}


//func DevolutivaPost() error { 

//}