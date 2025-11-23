package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

// fastClient conecta, VOTA (para gerar broadcast) e fica lendo rapidamente.
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
	// Lê as boas-vindas
	bufio.NewReader(conn).ReadString('\n')

	// VOTA para gerar tráfego de broadcast no servidor
	fmt.Fprintf(conn, "VOTE A\n")

	// Loop de leitura rápida para manter o buffer vazio
	reader := bufio.NewReader(conn)
	for {
		_, err := reader.ReadString('\n')
		if err != nil {
			// Servidor desconectou ou caiu
			return
		}
		// Opcional: imprimir um pontinho para mostrar que está vivo
		// fmt.Print(".") 
	}
}

// slowClient conecta e NUNCA lê nada, travando o socket.
func slowClient(wg *sync.WaitGroup) {
	defer wg.Done()
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Printf("Erro Slow: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Fprintf(conn, "SLOW_CLIENT\n")
	fmt.Println(">>> Cliente Lento conectado e travando o buffer de leitura <<<")

	// Não lê nada. Trava o servidor na próxima tentativa de escrita.
	time.Sleep(999 * time.Hour)
}

func main() {
	var wg sync.WaitGroup

	fmt.Println("Iniciando teste de carga cirúrgico...")

	// 1. Inicia o cliente lento (o sabotador)
	wg.Add(1)
	go slowClient(&wg)
	
	// Dá um tempo para ele conectar
	time.Sleep(1 * time.Second)

	// 2. Inicia muitos clientes normais que vão gerar votos e broadcasts
	clientCount := 50
	fmt.Printf("Iniciando %d clientes rápidos...\n", clientCount)
	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go fastClient(i, &wg)
		// Pequena pausa para não sobrecarregar o "Accept" do SO
		time.Sleep(10 * time.Millisecond) 
	}

	fmt.Println("Teste rodando. Observe os logs do servidor.")
	// Espera para sempre (ou até Ctrl+C)
	wg.Wait()
}
