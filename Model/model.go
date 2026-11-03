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
	IsTrechosPassageiros []string `json:"isTrechosPassageiros"`
}




type Motorista struct {
	Nome     string `json:"nome"` 
	Senha    string `json:"senha"`
	Carro    string `json:"carro"`
	Id       string `json:"id"`
	Trechos []PairTrecho
}



type PairTrecho struct {
	Origem  string
	Destino string
	IDtrecho string
}

//peso do grafo
type Trecho struct {
	Id            string
	Preco         float32
	Data          string 
	IdPassageiros []string 
	Nassentos     int
}



//Struct para ser mandado as infos no protocolo
type InfoCarona struct {
	Data        string
	ValorTrecho float32
	Assentos    int
	Rota        string
}

type InfoLoginMotorista struct {
	Nome     string `json:"nome"`
	Senha    string `json:"senha"`
	Id       string `json:"id"`
}

type InfoLoginPassageiro struct {
	Nome     string `json:"nome"`
	Senha    string `json:"senha"`
	Id       string `json:"id"`
}

type InfoPassagem struct {
	Origem     string `json:"origem"`
	Destino    string `json:"destino"`
	Data       string `json:"data"`
}

type InfoConfirmarReserva struct {
	Rota         []Aresta `json:"rota"`
	Confirma     bool     `json:"confirma"`
	Cancelar     bool     `json:"cancelar,omitempty"`
	TkAssociado  string   `json:"token"`
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

//Casos do motorista
//Cadastrar corrida
  //caso 1: A, B, C, outras info
  //criamos n-1 trechos com diferentes id e a mesma info, sendo n o número de vertices
  //atribuimos ao map de trechos esses novos trechos 
  //pegamos a info da origem e destino e ids dos trechos e criamos
  // o pairTrechos e damos append no slice de PairTrechos do motorista
  //Por último percorremos o grafo e atribuímos o id do motorista em suas respectivas arestas
   
//Remover corrida
	//Acessa o 
/*
	A -> [{'A','B',[1, 2, 'caso1']},{'A','C',[4, 5]},{}]
	B -> [{'B', 'C' [7, 12, 4,'caso1']},{},{}]
	C -> [{},{},{}]
	D -> []
*/

func ProcessarCadastroCorrida (infoCarona InfoCarona)([]Trecho, []string, error){
	vertices, err := ProcessarLista(infoCarona.Rota)
	if err != nil {
		fmt.Println("Erro no processamento da lista de dados")
		fmt.Println("Propragação de erro")
		return nil, nil, err
	}
	n:= len(vertices)-1
	arrayTrechos := make([]Trecho, n)
	for i := range arrayTrechos{
		id := GerarID()
		listaDePassageiros := make([]string, infoCarona.Assentos)
		trecho := Trecho{id, infoCarona.ValorTrecho, infoCarona.Data, listaDePassageiros, infoCarona.Assentos}
		arrayTrechos[i] = trecho
	}
	return arrayTrechos, vertices, nil
}

//TRATAR POSSÍVEIS ERROS :0
func CriarPairTrechos(trechos []Trecho, vertices []string) ([]PairTrecho, error){
	sliceIdTrechos := make([]PairTrecho, len(vertices)-1)
	for i:=0; i<=len(vertices)-2;i++{
		pairTrecho := PairTrecho{vertices[i], vertices[i+1], trechos[i].Id}
		sliceIdTrechos[i] = pairTrecho
	}
	return sliceIdTrechos, nil
}
// ProcessarLista recebe uma string que é um trecho completo que tem que ser
// quebrado em vertices. valida o separador, e retorna um slice em maiúsculo dos vértices que compõem este
// trecho
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
		itemSemUnderline := strings.ReplaceAll(item, "_", " ")
		itemLimpo := strings.Join(strings.Fields(itemSemUnderline), " ")
		
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


//Meu grafo é um map de string slice de aresta
//é uma lista de adjacencia implementada utilizando um map
//Minha aresta tem origem, destino e Ids, acredito que de trechos
//vamos ter um map de trechos


//Gera um map de string Trecho. Serve para que possamos manipular os trechos
//O servidor tipicamente vai atribuir valores a esta estrutura de dados a partir da
//leitura de um arquivo json
func novoMapTrechos() *map[string]Trecho {
	mapTrechos:=make(map[string]Trecho)
	return &mapTrechos
}
func novoMapMotoristas() *map[string]Motorista {
	mapMotoristas:=make(map[string]Motorista)
	return &mapMotoristas
}

func novoMapPassageiros() *map[string]Passageiro {
	mapPassageiros:=make(map[string]Passageiro)
	return &mapPassageiros
}


func PreencheTrechosGrafo(m Motorista, g *Grafo) (error) {
	pairTrechos := m.Trechos
	for i := range pairTrechos {
		infos := pairTrechos[i]
		g.AddIdTrecho(infos.Origem, infos.Destino, m.Id)
	}
	return nil
}


//grafo utilizando lista de adjacencia

type Aresta struct {
	Origem  string
	Destino string
	Ids []  string
}
type Grafo struct {
	G map[string][]Aresta
}

func NovoGrafo() *Grafo {
	return &Grafo{G: make(map[string][]Aresta)}
}


//Simplesmente adiciona uma aresta na lista de adjacencia sem checar se ela já existe ou não
//Pode causar duplicatas
func (grafo *Grafo) AddAresta(o, d, id string) {
	a := []string{id}
    grafo.G[o] = append(grafo.G[o], Aresta{o, d, a})
}

//Acredito que este tipo de remoção é mais lógico do que físico devido a natureza dos slices
//talvez seja interessante buscarmos uma outra forma de remover as coisas xd

//no caso de remoção de uma aresta teríamos que avisar geral sobre suas rotas sendo canceladas
func (grafo *Grafo) RmvAresta(o, d string) {
	arestas := grafo.G[o] 
	for i := range arestas {
		if arestas[i].Destino == d {
			arestas[i] = arestas[len(arestas)-1]
			arestas = arestas[:len(arestas)-1] //exclusivo
		}
	}
}


func (grafo *Grafo) AddIdTrecho(o, d, id string) {
	arestas, existe := grafo.G[o]
	if existe {
		for i := range arestas{
			if arestas[i].Destino == d{
				arestas[i].Ids = append(arestas[i].Ids, id)
				return
			} else {
				grafo.AddAresta(o, d, id)
			}
		}
	} else{
		grafo.AddAresta(o, d, id)
	}
}

func (grafo *Grafo) RmvIdTrecho(o, d, id string) {
	arestas, ok := grafo.G[o]
	if !ok {
		return
	}
	for i := range arestas {
		if arestas[i].Destino == d {
			novosIds := make([]string, 0, len(arestas[i].Ids))
			for _, idp := range arestas[i].Ids {
				if idp != id {
					novosIds = append(novosIds, idp)
				}
			}
			arestas[i].Ids = novosIds
			break
		}
	}
	grafo.G[o] = arestas
}
func (g *Grafo) BuscarRotas(
    origem, destino, data string,
    motoristas map[string]Motorista,
    trechos map[string]Trecho,
) ([][]Aresta, error) {
    if origem == destino {
        return nil, fmt.Errorf("origem e destino são iguais")
    }
    if _, ok := g.G[origem]; !ok {
        return nil, fmt.Errorf("origem %q não existe no grafo", origem)
    }

    var rotas [][]Aresta
    caminho := []Aresta{}
    visitados := map[string]bool{origem: true}

    var dfs func(atual string)
    dfs = func(atual string) {
        if atual == destino {
            rota := make([]Aresta, len(caminho))
            copy(rota, caminho)
            rotas = append(rotas, rota)
            return
        }

        for _, a := range g.G[atual] {
            if visitados[a.Destino] {
                continue
            }
            // critério novo: só segue se houver trecho válido nesta aresta
            if !arestaDisponivel(a, data, motoristas, trechos) {
                continue
            }

            visitados[a.Destino] = true
            caminho = append(caminho, a)

            dfs(a.Destino)

            caminho = caminho[:len(caminho)-1]
            visitados[a.Destino] = false
        }
    }

    dfs(origem)

    if len(rotas) == 0 {
        return nil, fmt.Errorf("não existe rota disponível de %q para %q em %s", origem, destino, data)
    }
    return rotas, nil
}

// arestaDisponivel devolve true se algum motorista da aresta tem um trecho
// naquela origem->destino, na data pedida, e com assentos livres.
func arestaDisponivel(
    a Aresta,
    data string,
    motoristas map[string]Motorista,
    trechos map[string]Trecho,
) bool {
    for _, idMotorista := range a.Ids {
        m, ok := motoristas[idMotorista]
        if !ok {
            continue
        }
        for _, p := range m.Trechos {
            // o motorista tem PairTrechos de várias arestas: filtra só os desta
            if p.Origem != a.Origem || p.Destino != a.Destino {
                continue
            }
            t, ok := trechos[p.IDtrecho]
            if !ok {
                continue
            }
            if t.Data == data && t.Nassentos > 0 {
                return true
            }
        }
    }
    return false
}





