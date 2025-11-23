package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	// Tenta conectar ao servidor na porta 9000
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Println("Erro ao conectar:", err)
		return
	}
	// Garante que a conexão será fechada ao sair do programa
	defer conn.Close()

	fmt.Println("Conectado ao servidor TCP!")

	// --- GOROUTINE DE LEITURA (Ouvido) ---
	// Iniciamos uma thread separada para ficar ouvindo tudo que o servidor manda.
	// Isso inclui a mensagem de boas-vindas (com as opções) e os broadcasts de placar.
	go func() {
		scanner := bufio.NewScanner(conn)
		// O scanner.Scan() bloqueia aqui até chegar uma nova linha do servidor
		for scanner.Scan() {
			// Imprime qualquer mensagem recebida com um prefixo para identificar
			fmt.Println("\n[SERVIDOR]:", scanner.Text())
			// Reimpressão do prompt visual para ficar bonito no terminal após uma mensagem recebida
			fmt.Print(">> ")
		}
		// Se o loop acabar, o servidor desconectou ou houve erro
		fmt.Println("\nConexão com o servidor encerrada.")
		os.Exit(0)
	}()

	// --- THREAD PRINCIPAL (Boca) ---
	// Esta parte cuida de ler o teclado do usuário e enviar para o servidor.

	scanner := bufio.NewScanner(os.Stdin)

	// 1. Handshake inicial (Enviar ID)
	fmt.Print("Digite seu ID para entrar: ")
	scanner.Scan()
	id := scanner.Text()
	fmt.Fprintf(conn, "%s\n", id)

	// NOTA: Removemos a linha que dizia quais eram as opções de voto.
	// Agora, esperamos a goroutine acima receber e imprimir a mensagem de
	// boas-vindas do servidor, que contém as opções atualizadas.

	// 2. Loop de envio de comandos
	for {
		fmt.Print(">> ") // Prompt visual
		// Bloqueia esperando o usuário digitar algo no terminal
		if !scanner.Scan() {
			break // Usuário deu Ctrl+C ou fechou o terminal
		}
		text := scanner.Text()
		// Envia o texto cru para o servidor com uma quebra de linha no final
		fmt.Fprintf(conn, "%s\n", text)
	}
}
