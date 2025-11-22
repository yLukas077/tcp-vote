package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		log.Fatal("Erro ao abrir arquivo de log:", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)

	fmt.Println("--- SERVIDOR RODANDO ---")
	fmt.Println("Todos os logs estão sendo escritos em 'server.log'.")
	fmt.Println("Pressione Ctrl+C para encerrar.")

	server := NewServer()
	server.Start(":9000")
}
