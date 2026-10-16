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
	Trechos []Aresta
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
	Data          string 
	IdPassageiros []string 
	Nassentos      int
}



//Struct para ser mandado as infos no protocolo
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



//grafo utilizando lista de adjacencia

type Aresta struct {
	Origem string
	Destino string
	Ids [] string
}
type Grafo struct {
	g map[string][]Aresta
}

func NovoGrafo() *Grafo {
	return &Grafo{g: make(map[string][]Aresta)}
}


//Simplesmente adiciona uma aresta na lista de adjacencia sem checar se ela já existe ou não
//Pode causar duplicatas
func (grafo *Grafo) AddAresta(o, d, id string) {
	a := []string{id}
    grafo.g[o] = append(grafo.g[d], Aresta{o, d, a})
}

//Acredito que este tipo de remoção é mais lógico do que físico devido a natureza dos slices
//talvez seja interessante buscarmos uma outra forma de remover as coisas xd

//no caso de remoção de uma aresta teríamos que avisar geral sobre suas rotas sendo canceladas
func (grafo *Grafo) RmvAresta(o, d string) {
	arestas := grafo.g[o] 
	for i := range arestas {
		if arestas[i].Destino == d {
			arestas[i] = arestas[len(arestas)-1]
			arestas = arestas[:len(arestas)-1] //exclusivo
		}
	}
}


func (grafo *Grafo) AddIdTrecho(o, d, id string) {
	arestas := grafo.g[o]
	for _, aresta := range arestas{
		if aresta.Destino == d{
			aresta.Ids = append(aresta.Ids, id)
		}
	}
}

func (grafo *Grafo) RmvIdTrecho(o, d, id string){
	arestas := grafo.g[o]
	for _, aresta := range arestas {
		if aresta.Destino == d {
			for _, idp := range aresta.Ids{
				if id == idp{
					idp = aresta.Ids[len(aresta.Ids)-1]
					aresta.Ids = aresta.Ids[:len(aresta.Ids)-1]
				}
			}
		}
	}
}









