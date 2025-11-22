package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	conn, _ := net.Dial("tcp", "localhost:9000")
	defer conn.Close()

	// Goroutine para ouvir o servidor
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("[SERVIDOR]:", scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Digite seu ID: ")
	scanner.Scan()
	fmt.Fprintf(conn, "%s\n", scanner.Text())

	fmt.Println("Digite 'VOTE A', 'VOTE B' ou 'VOTE C'")
	for {
		fmt.Print(">> ")
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		fmt.Fprintf(conn, "%s\n", text)
	}
}
