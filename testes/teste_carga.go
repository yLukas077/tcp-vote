package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

func main() {
	totalBots := 50
	var wg sync.WaitGroup

	fmt.Printf("Iniciando Ramp-up de %d bots...\n", totalBots)

	for i := 0; i < totalBots; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", "localhost:9000")
			if err != nil {
				// Se der erro, só imprime e sai
				fmt.Printf("x")
				return
			}
			defer conn.Close()

			// Manda ID
			fmt.Fprintf(conn, "BOT_%d\n", id)

			// Vota
			time.Sleep(1 * time.Second)
			fmt.Fprintf(conn, "VOTE A\n")

			// Segura a conexão aberta por 10s para ocupar o servidor
			time.Sleep(10 * time.Second)
		}(i)

		// O SEGREDO: Pausa de 2ms para não estourar o buffer do Windows
		time.Sleep(2 * time.Millisecond)
		if i%100 == 0 {
			fmt.Print(".")
		} // Barra de progresso visual
	}
	wg.Wait()
	fmt.Println("\nTeste finalizado.")
}
