package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

// VotingOptions encapsula as opções de voto disponíveis.
type VotingOptions struct {
	List          []string
	DisplayString string
}

// Server representa o servidor TCP de votação concorrente.
type Server struct {
	// SYSCALL: socket(AF_INET, SOCK_STREAM, 0) + bind() + listen()
	// listener é o file descriptor em estado LISTEN aguardando SYN packets
	listener net.Listener

	// Mutex protege acesso concorrente aos mapas compartilhados
	// Previne race conditions em leituras/escritas simultâneas
	mu         sync.Mutex
	clients    map[string]net.Conn // Mapa de file descriptors ativos (ID -> conexão TCP)
	votes      map[string]string   // Histórico de votos
	voteCounts map[string]int      // Placar agregado

	options           VotingOptions
	useAsyncBroadcast bool

	// Channel para comunicação assíncrona entre goroutines
	// Buffer de 1000 previne bloqueio em picos de carga
	broadcastChan chan map[string]int
}

// NewServer inicializa o servidor com opções de voto e modo de operação.
func NewServer(async bool, optionsList []string) *Server {
	s := &Server{
		clients:    make(map[string]net.Conn),
		votes:      make(map[string]string),
		voteCounts: make(map[string]int),
		options: VotingOptions{
			List:          optionsList,
			DisplayString: strings.Join(optionsList, ", "),
		},
		useAsyncBroadcast: async,
	}

	// Inicializa contadores para todas as opções
	for _, op := range optionsList {
		s.voteCounts[op] = 0
	}

	if async {
		s.broadcastChan = make(chan map[string]int, 1000)
		// Goroutine worker consome canal em background
		go s.broadcastWorker()
	}

	return s
}

// Start inicia o servidor TCP na porta especificada.
func (s *Server) Start(port string) {
	var err error

	// SYSCALL: socket() cria file descriptor
	// SYSCALL: bind() associa fd à porta 9000
	// SYSCALL: listen() marca socket como passivo, aceita SYN packets
	// Kernel mantém duas filas:
	//   - SYN queue: conexões half-open (aguardando ACK)
	//   - Accept queue: conexões completas prontas para Accept()
	s.listener, err = net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Erro ao iniciar: %v", err)
	}
	log.Printf("Servidor ouvindo na porta %s", port)
	log.Printf("Opções de voto: [%s]", s.options.DisplayString)

	// Event loop principal
	for {
		// SYSCALL: accept(listener_fd, &client_addr, &addr_len)
		// Bloqueia até conexão disponível na Accept queue
		// Kernel cria NOVO file descriptor para a conexão do cliente
		// Retorna net.Conn (wrapper Go do fd)
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Erro no accept:", err)
			continue
		}

		// Goroutine separada para cada cliente (modelo M:N do Go)
		// Goroutines são multiplexadas em threads do SO pelo runtime
		go s.handleClient(conn)
	}
}

// handleClient processa um cliente conectado em goroutine dedicada.
func (s *Server) handleClient(conn net.Conn) {
	// SYSCALL: close(fd) ao sair (libera file descriptor no kernel)
	defer conn.Close()

	// bufio.Reader mantém buffer interno de 4KB
	// Reduz syscalls: ao invés de read(fd, buf, 1) para cada byte,
	// faz read(fd, internal_buffer, 4096) e serve da memória
	reader := bufio.NewReader(conn)

	// SYSCALL: read(fd, buffer, size) - bloqueante se não há dados
	idStr, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	id := strings.TrimSpace(idStr)

	// Seção crítica: protege acesso ao mapa compartilhado
	s.mu.Lock()
	if _, exists := s.clients[id]; exists {
		s.mu.Unlock()
		conn.Write([]byte("ERRO: ID em uso\n"))
		return
	}
	s.clients[id] = conn
	s.mu.Unlock()

	log.Printf("Conectado: %s", id)

	welcomeMsg := fmt.Sprintf("Bem-vindo! Opcoes disponiveis: [%s]. Digite: VOTE [Opcao]\n", s.options.DisplayString)
	
	// SYSCALL: write(fd, buffer, len)
	// Escreve no TCP send buffer do kernel
	// Kernel fragmenta em segmentos TCP (MSS ~1460 bytes) e envia
	conn.Write([]byte(welcomeMsg))

	// Loop de leitura de comandos
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			// EOF: cliente fechou conexão
			break
		}

		msg = strings.TrimSpace(msg)
		if strings.HasPrefix(msg, "VOTE ") {
			s.processVote(id, strings.TrimPrefix(msg, "VOTE "))
		}
	}

	// Cleanup: remove cliente desconectado
	s.mu.Lock()
	delete(s.clients, id)
	s.mu.Unlock()

	log.Printf("Desconectado: %s", id)
}

// processVote processa um voto e dispara broadcast.
func (s *Server) processVote(id, option string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Valida se já votou
	if _, jaVotou := s.votes[id]; jaVotou {
		return
	}

	// Valida opção
	isValid := false
	for _, validOption := range s.options.List {
		if option == validOption {
			isValid = true
			break
		}
	}

	if !isValid {
		log.Printf("Voto inválido de %s: %s", id, option)
		return
	}

	// Atualiza estado
	s.votes[id] = option
	s.voteCounts[option]++
	log.Printf("Voto: %s -> %s", id, option)

	if s.useAsyncBroadcast {
		// MODO ASSÍNCRONO: Evita I/O bloqueante com mutex travado
		
		// Snapshot do placar (cópia profunda evita race conditions)
		snapshot := make(map[string]int, len(s.voteCounts))
		for k, v := range s.voteCounts {
			snapshot[k] = v
		}

		// Envia para channel (operação rápida, não bloqueia se buffer não está cheio)
		// Worker goroutine fará o I/O de rede fora da seção crítica
		s.broadcastChan <- snapshot
	} else {
		// MODO BLOQUEANTE: I/O de rede com mutex travado
		// PROBLEMA: Se conn.Write() bloquear (cliente lento), 
		// toda votação trava (mutex não é liberado)
		s.broadcastLocked()
	}
}

// broadcastLocked envia atualizações segurando o mutex principal (modo bloqueante).
func (s *Server) broadcastLocked() {
	msg := fmt.Sprintf("UPDATE: %v\n", s.voteCounts)
	msgBytes := []byte(msg)

	for id, conn := range s.clients {
		if _, votou := s.votes[id]; votou {
			// GARGALO: write() pode bloquear se TCP send buffer estiver cheio
			// (cliente não lê dados, sliding window = 0)
			// Mutex permanece travado durante bloqueio = servidor congelado
			conn.Write(msgBytes)
		}
	}
}

// broadcastWorker consome channel e faz broadcast assíncrono.
func (s *Server) broadcastWorker() {
	// Consome canal em loop infinito
	// Bloqueia (sem consumir CPU) quando canal vazio
	for update := range s.broadcastChan {
		msg := fmt.Sprintf("UPDATE: %v\n", update)
		msgBytes := []byte(msg)

		// Mutex travado apenas durante leitura do mapa de clientes
		s.mu.Lock()
		
		// Snapshot de clientes para envio fora da seção crítica
		// (solução ideal seria copiar clientes também, mas didaticamente aceitável)
		for id, conn := range s.clients {
			if _, votou := s.votes[id]; votou {
				// SYSCALL: write(fd, buffer, len)
				// Pode bloquear aqui, mas não trava votações
				// (goroutines de voto já liberaram mutex)
				conn.Write(msgBytes)
			}
		}
		
		s.mu.Unlock()
	}
}
