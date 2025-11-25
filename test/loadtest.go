package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

// fastClient simula cliente que lê dados rapidamente (bom comportamento).
func fastClient(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Printf("Erro Fast %d: %v\n", id, err)
		return
	}
	defer conn.Close()

	// Handshake
	fmt.Fprintf(conn, "FAST_%d\n", id)
	bufio.NewReader(conn).ReadString('\n')

	// Vota para gerar broadcasts
	fmt.Fprintf(conn, "VOTE A\n")

	// Loop de leitura rápida mantém TCP receive buffer vazio
	reader := bufio.NewReader(conn)
	for {
		_, err := reader.ReadString('\n')
		if err != nil {
			return
		}
	}
}

// slowClient simula cliente malicioso que nunca lê dados (ataque DoS).
// TCP receive buffer enche -> sliding window = 0 -> servidor bloqueia em write()
func blockedClient(wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Printf("Erro Slow: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Fprintf(conn, "BLOCKED_CLIENT\n")
	// Lê mensagem de boas-vindas
	bufio.NewReader(conn).ReadString('\n')

	// Vota para entrar na lista de broadcast
	fmt.Fprintf(conn, "VOTE A\n")
	fmt.Println(">>> Cliente parou de ler -> buffer TCP vai encher <<<")

	// Nunca lê do socket -> buffer enche -> trava write()
	time.Sleep(999 * time.Hour)
}

func main() {
	var wg sync.WaitGroup

	fmt.Println("=== TESTE DE BROADCAST COM BUFFER CHEIO ===")
	fmt.Println("Objetivo: Demonstrar bloqueio em write() com mutex travado")
	// Cliente sabotador (trava buffer TCP)
	wg.Add(1)
	go blockedClient(&wg)
	time.Sleep(1 * time.Second)

	// Clientes normais (geram votos e broadcasts)
	clientCount := 50
	fmt.Printf("Iniciando %d clientes rápidos...\n", clientCount)
	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go fastClient(i, &wg)
		time.Sleep(10 * time.Millisecond)
	}

	fmt.Println("\nTeste rodando. Observe os logs do servidor.")
	fmt.Println("Modo sync: servidor BLOQUEIA ao enviar para cliente com buffer cheio")
	fmt.Println("Modo async: servidor CONTINUA processando (worker bloqueia isoladamente)")

	wg.Wait()
}
