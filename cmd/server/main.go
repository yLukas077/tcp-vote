package main

import (
	"fmt"
	"log"
	"os"
	"time"

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
	fmt.Println("Modo: Assíncrono (Chanells + Worker)")

	// Opções de voto configuráveis
	opcoes := []string{"A", "B", "C"}
	// Inicia servidor em modo assíncrono (true = non-blocking broadcast)
	srv := server.NewServer(true, opcoes)

	// Inicia votação após 5 segundos com duração de 60 segundos
	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("Iniciando votação (300 segundos)...")
		srv.StartVoting(300)
	}()

	srv.Start(":9000")
}
