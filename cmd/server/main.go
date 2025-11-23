package main

import (
	"fmt"
	"log"
	"os"

	"github.com/yLukas077/tcp-vote/internal/server"
)

func main() {
	// Redireciona logs para arquivo persistente
	logFile, err := os.OpenFile("logs/server.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		log.Fatal("Erro ao abrir log:", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	fmt.Println("=== SERVIDOR TCP DE VOTAÇÃO ===")
	fmt.Println("Logs: logs/server.log")
	fmt.Println("Modo: Assíncrono (Channel + Worker)")
	fmt.Println("Pressione Ctrl+C para encerrar.")

	// Opções de voto configuráveis
	opcoes := []string{"A", "B", "C"}
	
	// Inicia servidor em modo assíncrono (true = non-blocking broadcast)
	srv := server.NewServer(true, opcoes)
	srv.Start(":9000")
}
