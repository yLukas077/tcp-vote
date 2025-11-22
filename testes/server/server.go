package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

type Server struct {
	listener net.Listener

	// Dados combinados (Simples de entender visualmente)
	mu         sync.Mutex
	clients    map[string]net.Conn
	votes      map[string]string
	voteCounts map[string]int
}

func NewServer() *Server {
	return &Server{
		clients:    make(map[string]net.Conn),
		votes:      make(map[string]string),
		voteCounts: make(map[string]int),
	}
}

func (s *Server) Start(port string) {
	var err error
	s.listener, err = net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Erro ao iniciar: %v", err)
	}

	log.Printf("Servidor ouvindo na porta %s", port)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Erro no accept:", err)
			continue
		}
		go s.handleClient(conn)
	}
}

func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// 1. Ler o ID do Cliente
	idStr, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	id := strings.TrimSpace(idStr)

	// 2. Registrar (Critical Section)
	s.mu.Lock()
	if _, exists := s.clients[id]; exists {
		s.mu.Unlock()
		conn.Write([]byte("ERRO: ID em uso\n"))
		return
	}
	s.clients[id] = conn
	s.mu.Unlock()

	log.Printf("Conectado: %s", id)
	conn.Write([]byte("Bem-vindo! Digite: VOTE [Opcao]\n"))

	// 3. Loop de Comandos
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			break
		} // Cliente desconectou

		msg = strings.TrimSpace(msg)
		if strings.HasPrefix(msg, "VOTE ") {
			s.processVote(id, strings.TrimPrefix(msg, "VOTE "))
		}
	}

	// 4. Cleanup ao sair
	s.mu.Lock()
	delete(s.clients, id)
	s.mu.Unlock()
	log.Printf("Desconectado: %s", id)
}

func (s *Server) processVote(id, option string) {
	s.mu.Lock()
	defer s.mu.Unlock() // Destrava automaticamente ao fim da função

	// Validação básica
	if _, jaVotou := s.votes[id]; jaVotou {
		return // Ignora silenciosamente ou manda erro
	}

	// Registra
	s.votes[id] = option
	s.voteCounts[option]++
	log.Printf("Voto: %s -> %s", id, option)

	// FALHA ESTRATÉGICA AQUI: Broadcast dentro do Lock
	s.broadcast()
}

func (s *Server) broadcast() {
	// Gera string de placar
	msg := fmt.Sprintf("UPDATE: %v\n", s.voteCounts)

	// Envia para todos
	for _, conn := range s.clients {
		conn.Write([]byte(msg))
	}
}
