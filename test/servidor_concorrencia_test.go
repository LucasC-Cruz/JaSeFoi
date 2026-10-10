package main

import (
	"Golang/Apiclient"
	"fmt"
	"sync"
	"testing"
)

func TestConcorrenciaReservas(t *testing.T) { 
	addr := "localhost:6742" // Servidor precisa estar rodando aqui

	const totalAssentos = 3
	const totalAtacantes = 100	

	// 1. Conecta um motorista para criar a carona de teste
	driverClient, err := Apiclient.Dial(addr)
	if err != nil {
		t.Fatalf("Erro ao conectar motorista: %v", err)
	}
	defer driverClient.Close()

	// Registra e loga o motorista
	_, err = driverClient.RegistraMotorista("motorista_chefe", "123", "CarroA")
	_, tkMotorista, err := driverClient.AuthMotorista("motorista_chefe", "123")
	if err != nil {
		t.Fatalf("Erro no auth do motorista: %v", err)
	}

	// Publica a corrida com apenas 3 assentos
	// Rota: SALVADOR -> FEIRA DE SANTANA, Data: 20/10
	_, err = driverClient.CadastrarCorrida("Salvador, Feira de Santana", "20/10", "08:00", totalAssentos, 25.0, tkMotorista)
	if err != nil {
		t.Fatalf("Erro ao cadastrar corrida: %v", err)
	}
	fmt.Println("Corrida cadastrada com sucesso para o teste!")

	var wg sync.WaitGroup
	var sucessos int32
	var falhas int32
	var mu sync.Mutex

	start := make(chan struct{})

	fmt.Printf("Iniciando disputa de %d passageiros por apenas %d assentos...\n", totalAtacantes, totalAssentos)

	for i := 0; i < totalAtacantes; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			c, err := Apiclient.Dial(addr)
			if err != nil {
				mu.Lock(); falhas++; mu.Unlock()
				return
			}
			defer c.Close()

			nomePassageiro := fmt.Sprintf("pass_%d", id)
			senha := "123"

			// Cadastra o passageiro e depois loga
			_, _ = c.RegistraPassageiro(nomePassageiro, senha)
			_, tkPass, err := c.AuthPassageiro(nomePassageiro, senha)
			if err != nil || tkPass == "" {
				mu.Lock(); falhas++; mu.Unlock()
				return
			}

			// Sincroniza para todo mundo tentar reservar no mesmo instante
			<-start

			_, err = c.ReservarPassagem("Salvador", "Feira de Santana", "20/10", tkPass)

			mu.Lock()
			if err == nil {
				sucessos++
			} else {
				falhas++
			}
			mu.Unlock()
		}(i)
	}

	// Dispara todos juntos
	close(start)
	wg.Wait()

	fmt.Printf("--- RESULTADO DO TESTE ---\n")
	fmt.Printf("Esperado sucesso: %d | Obtido sucesso: %d\n", totalAssentos, sucessos)
	fmt.Printf("Esperado falhas: %d | Obtido falhas: %d\n", totalAtacantes-totalAssentos, falhas)

	if int(sucessos) != totalAssentos {
		t.Errorf("Falha na concorrência: esperava exatamente %d sucessos, mas passaram %d", totalAssentos, sucessos)
	}
}