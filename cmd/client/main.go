package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	// SYSCALL: socket() + connect() - cria socket TCP e estabelece conexão com servidor
    // Kernel cria um file descriptor (FD) para rastrear este socket
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Println("Erro ao conectar:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Conectado ao servidor TCP!")

	// Goroutine dedicada para leitura assíncrona
	// Permite receber broadcasts enquanto o usuário digita
	go func() {
		scanner := bufio.NewScanner(conn)
		// SYSCALL: read(fd, buffer, size) - bloqueante até dados chegarem
		for scanner.Scan() {
			fmt.Println("\n[SERVIDOR]:", scanner.Text())
			fmt.Print(">> ")
		}
		// Servidor encerrou conexão (close do FD remoto)
		fmt.Println("\nConexão com o servidor encerrada.")
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)

	// Handshake: envia NOME do cliente
	fmt.Print("Digite seu NOME para entrar: ")
	scanner.Scan()
	id := scanner.Text()
	
	// SYSCALL: write(fd, buffer, len) - escreve no socket TCP usando seu FD
	fmt.Fprintf(conn, "%s\n", id)

	// Loop de envio de comandos
	for {
		fmt.Print(">> ")
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		fmt.Fprintf(conn, "%s\n", text)
	}
}
