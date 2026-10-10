package Model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Passageiro struct {
	Nome               string `json:"nome"`
	Senha              string `json:"senha"`
	Id                 string `json:"id"`
	IdCaronaPassageiro string
}

type Motorista struct {
	Nome     string `json:"nome"`
	Senha    string `json:"senha"`
	Carro    string `json:"carro"`
	Id       string `json:"id"`
	IdCarona string
}

// struct de corrida para o passageiro
// lista de motoristas, lista de trechos
// isso será construído ao fazermos a bfs no grafo para montar uma rota para o passageiro
type CaronaPassageiro struct {
	Rota []Trecho
}

type PairTrecho struct {
	Origem  string
	Destino string
}

//peso do grafo
type Trecho struct {
	IdMotorista   string
	Data          string //vai ficar aqui por enquanto 
	IdPassageiros []string
	Trecho        PairTrecho 
	Assentos      int
}

// Este caba aqui será construído por nós
// percorrendo o grafo
type Carona struct {
	IdMotorista string
	IdCarona    string
	Data        string
	Horario     string
	ValorTrecho float32
	Assentos    int
	Rota        []string // ou Trecho
}

type InfoCarona struct {
	Data        string
	Horario     string
	ValorTrecho float32
	Assentos    int
	Rota        string
}

func GerarID() string {
	// 1. Pega o tempo atual em milissegundos
	timestamp := time.Now().UnixMilli()

	// 2. Gera 6 bytes de entropia (resultando em 12 caracteres hexadecimais)
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback de segurança caso a leitura de bytes falhe
		return fmt.Sprintf("%d", timestamp)
	}

	// 3. Concatena tudo em uma string formatada
	return fmt.Sprintf("%d-%s", timestamp, hex.EncodeToString(b))
}




// ProcessarLista recebe uma string, valida o separador, e retorna um slice em maiúsculo
func ProcessarLista(entrada string) ([]string, error) {
	// 1. Remove os espaços das pontas da string principal
	entrada = strings.TrimSpace(entrada)

	// 2. Valida se a string está vazia
	if entrada == "" {
		return nil, errors.New("a string de entrada está vazia")
	}

	// 3. Valida se a string usa vírgula como separador
	// (Se não tiver vírgula, consideramos que usou outro separador ou é inválida)
	if !strings.Contains(entrada, ",") {
		return nil, errors.New("formato inválido: a string deve ser separada por vírgulas")
	}

	// 4. Faz a separação da string
	itens := strings.Split(entrada, ",")
	var resultado []string

	// 5. Percorre os itens para limpar espaços vazios e converter para Upper Case
	for _, item := range itens {
		itemLimpo := strings.TrimSpace(item)
		
		// Ignora pedaços vazios caso a string venha com vírgulas duplas (ex: "A,,B")
		if itemLimpo != "" {
			resultado = append(resultado, strings.ToUpper(itemLimpo))
		}
	}

	// Validação extra caso a string fosse apenas vírgulas (ex: ",,,")
	if len(resultado) == 0 {
		return nil, errors.New("nenhum valor válido encontrado na string")
	}

	return resultado, nil
}

