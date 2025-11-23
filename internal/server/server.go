package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)


type VotingState string

const (
    VotingNotStarted VotingState = "NOT_STARTED"
    VotingActive     VotingState = "ACTIVE"
    VotingEnded      VotingState = "ENDED"
)

// VotingOptions encapsula as opções de voto disponíveis.
type VotingOptions struct {
	List          []string
	DisplayString string
}

// Server representa o servidor TCP de votação concorrente.
type Server struct {
    // SYSCALL: socket(AF_INET, SOCK_STREAM, 0) + bind() + listen()
    // listener é o socket em estado LISTEN aguardando SYN packets
    // Internamente, o kernel usa um file descriptor (FD) para rastrear este socket
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

	// Controle de votação
    votingState    VotingState
    votingDeadline time.Time
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
		votingState:       VotingNotStarted,
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
		// Kernel cria NOVO socket (e seu FD correspondente) para cada cliente
		// net.Conn é o wrapper Go deste socket TCP
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

// StartVoting inicia a votação com tempo limite em segundos
func (s *Server) StartVoting(durationSeconds int) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.votingState != VotingNotStarted {
        log.Println("Votação já foi iniciada anteriormente")
        return
    }
    
    s.votingState = VotingActive
    s.votingDeadline = time.Now().Add(time.Duration(durationSeconds) * time.Second)
    
    log.Printf("Votação INICIADA. Deadline: %s", s.votingDeadline.Format("15:04:05"))
    
    // Notifica todos os clientes
    announcement := fmt.Sprintf("VOTACAO_INICIADA: %d segundos. Opcoes: [%s]\n", 
        durationSeconds, s.options.DisplayString)
    
    for _, conn := range s.clients {
        conn.Write([]byte(announcement))
    }
    
    // Timer para encerrar automaticamente
    time.AfterFunc(time.Duration(durationSeconds)*time.Second, func() {
        s.endVoting()
    })
}

// endVoting encerra a votação e envia resultado final
func (s *Server) endVoting() {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.votingState != VotingActive {
        return
    }
    
    s.votingState = VotingEnded
    log.Println("Votação ENCERRADA")
    
    // Resultado final
    result := fmt.Sprintf("VOTACAO_ENCERRADA: %v\n", s.voteCounts)
    
    for _, conn := range s.clients {
        conn.Write([]byte(result))
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
	// Lê do socket TCP (internamente usando o FD do kernel)
	idStr, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	id := strings.TrimSpace(idStr)

	// Seção crítica: protege acesso ao mapa compartilhado
	s.mu.Lock()
	if _, exists := s.clients[id]; exists {
		s.mu.Unlock()
		conn.Write([]byte("ERRO: NOME em uso\n"))
		return
	}
	s.clients[id] = conn

	// Envia status da votação
    var statusMsg string
    switch s.votingState {
    case VotingNotStarted:
        statusMsg = "Aguardando inicio da votacao...\n"
    case VotingActive:
        remaining := time.Until(s.votingDeadline).Round(time.Second)
        statusMsg = fmt.Sprintf("Votacao em andamento! Tempo restante: %s\nOpcoes: [%s]\n", 
            remaining, s.options.DisplayString)
    case VotingEnded:
        statusMsg = fmt.Sprintf("Votacao encerrada. Resultado: %v\n", s.voteCounts)
    }

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

	conn := s.clients[id]  // Guarda referência para enviar respostas

    // VALIDAÇÃO 1: Votação não iniciada
    if s.votingState == VotingNotStarted {
        conn.Write([]byte("ERRO: Votacao nao iniciada\n"))
        log.Printf("Voto rejeitado (%s): votação não iniciada", id)
        return  // ← defer garante que mutex será liberado
    }

    // VALIDAÇÃO 2: Votação já encerrada
    if s.votingState == VotingEnded {
        conn.Write([]byte("ERRO: Votacao encerrada\n"))
        log.Printf("Voto rejeitado (%s): votação encerrada", id)
        return
    }

    // VALIDAÇÃO 3: Tempo limite expirado
    if time.Now().After(s.votingDeadline) {
        conn.Write([]byte("ERRO: Tempo limite expirado\n"))
        log.Printf("Voto rejeitado (%s): tempo expirado", id)
        // Encerra votação (este método já tem seu próprio Lock/Unlock)
        go s.endVoting()  // ← async para evitar deadlock
        return
    }

    // VALIDAÇÃO 4: Voto duplicado
    if _, jaVotou := s.votes[id]; jaVotou {
        conn.Write([]byte("ERRO: Voto duplicado\n"))
        log.Printf("Voto rejeitado (%s): já votou", id)
        return
    }

	// VALIDAÇÃO 5: Opção inválida
    isValid := false
    for _, validOption := range s.options.List {
        if option == validOption {
            isValid = true
            break
        }
    }

    if !isValid {
        conn.Write([]byte(fmt.Sprintf("ERRO: Opcao invalida. Use: [%s]\n", s.options.DisplayString)))
        log.Printf("Voto rejeitado (%s): opção inválida '%s'", id, option)
        return
    }

    // VOTO VÁLIDO - Registra
    s.votes[id] = option
    s.voteCounts[option]++

    // CONFIRMAÇÃO para o cliente
    confirmation := fmt.Sprintf("OK: Voto registrado -> %s\n", option)
    conn.Write([]byte(confirmation))
    log.Printf("Voto aceito: %s -> %s", id, option)

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
	log.Println("[SYNC] Iniciando broadcast síncrono (MUTEX LOCK)")
	padding := strings.Repeat("\x00", 256*1024) // 256KB
    msg := fmt.Sprintf("UPDATE: %v | SNAPSHOT: %s\n", s.voteCounts, padding)

	for id, conn := range s.clients {
		if _, votou := s.votes[id]; votou {
			// GARGALO: write() pode bloquear se TCP send buffer estiver cheio
			// (cliente não lê dados, sliding window = 0)
			// Mutex permanece travado durante bloqueio = servidor congelado
			log.Printf("[SYNC] Tentando enviar para %s...", id)
            n, err := conn.Write([]byte(msg))
            
            if err != nil {
                log.Printf("[SYNC] ERRO ao enviar para %s: %v", id, err)
            } else if n < len(msg) {
                log.Printf("[SYNC] PARCIAL para %s: enviados %d/%d bytes", 
                    id, n, len(msg))
            } else {
                log.Printf("[SYNC] Sucesso para %s: %d bytes", id, n)
            }
		}
	}
	log.Println("[SYNC] Fim do broadcast síncrono")
}

// broadcastWorker consome channel e faz broadcast assíncrono.
func (s *Server) broadcastWorker() {
	// Consome canal em loop infinito
	// Bloqueia (sem consumir CPU) quando canal vazio
	for update := range s.broadcastChan {
		log.Println("[ASYNC] Iniciando broadcast assíncrono")

		// DESCOMENTE para simular broadcast com mensagem gigante (256KB)
        // Útil para demonstrar que modo async não trava mesmo com cliente lento

        // padding := strings.Repeat("\x00", 256*1024) // 256KB
        // msg = fmt.Sprintf("UPDATE: %v | SNAPSHOT: %s\n", update, padding)
        // log.Printf("[ASYNC] Modo LARGE PAYLOAD")

		// COMENTAR ESSA LINHA PARA FAZER A SIMULACAO
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

		log.Println("[ASYNC] Fim do broadcast assíncrono")
	}
}
