package main

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
	List          []string // Lista crua das opções (ex: ["A", "B", "C"])
	DisplayString string   // String formatada para envio (ex: "[A, B, C]")
}

// Server representa nosso servidor TCP.
// Ele mantém o estado da votação e gerencia as conexões de rede.
type Server struct {
	// listener é o "porteiro" do SO. É o socket em estado LISTEN
	// que fica aguardando novas conexões chegarem na porta.
	listener net.Listener

	// mu é o nosso Mutex (Exclusão Mútua).
	// Como teremos milhares de goroutines acessando os mapas abaixo ao mesmo tempo,
	// precisamos deste "cadeado" para garantir que apenas UMA goroutine escreva por vez.
	// Sem isso, teríamos "Race Conditions" (corrupção de memória).
	mu          sync.Mutex
	clients     map[string]net.Conn // Mapa de sockets ativos (ID -> Socket)
	votes       map[string]string   // Quem já votou e em quê
	voteCounts  map[string]int      // Placar agregado

	// Opções de voto configuradas para o servidor.
	// Como são apenas leitura após a inicialização, não precisam de Mutex.
	options VotingOptions
	
	// Toggle para demonstração ao vivo:
	// false = Modo Bloqueante (lento, segura o Mutex durante I/O de rede)
	// true  = Modo Assíncrono (rápido, usa Channel e Worker)
	useAsyncBroadcast bool
	
	// Canal (Channel) do Go.
	// É o "tubo de correio" para comunicação entre goroutines.
	// A goroutine que processa o voto coloca o placar aqui, e a goroutine Worker pega.
	// É thread-safe por natureza.
	broadcastChan     chan map[string]int
}

// NewServer inicializa o servidor, suas opções e decide o modo de operação.
// Agora aceita a lista de opções como parâmetro.
func NewServer(async bool, optionsList []string) *Server {
	// Formata a string de opções uma vez só para não repetir trabalho.
	// Exemplo: junta ["A", "B"] em "A, B"
	displayStr := strings.Join(optionsList, ", ")

	s := &Server{
		clients:           make(map[string]net.Conn),
		votes:             make(map[string]string),
		voteCounts:        make(map[string]int),
		// Inicializa as opções disponíveis
		options: VotingOptions{
			List:          optionsList,
			DisplayString: displayStr,
		},
		useAsyncBroadcast: async,
	}

	// Inicializa o mapa de contagem com zero para todas as opções.
	// Isso garante que todas as opções apareçam no placar desde o início.
	for _, op := range optionsList {
		s.voteCounts[op] = 0
	}

	if async {
		// Se o modo assíncrono estiver ligado, criamos o canal.
		// O buffer de 1000 permite enfileirar até 1000 atualizações de placar
		// antes que quem envia precise esperar (não bloqueante até encher).
		s.broadcastChan = make(chan map[string]int, 1000)
		
		// Iniciamos o WORKER em sua própria Goroutine.
		// Ele vai rodar para sempre em background, processando a fila do canal.
		go s.broadcastWorker()
	}

	return s
}

// Start é onde a mágica da rede começa.
func (s *Server) Start(port string) {
	var err error
	// 1. BIND e LISTEN
	// net.Listen pede ao SO para reservar a porta e começar a escutar pacotes SYN (TCP).
	// Isso é o equivalente às syscalls socket(), bind() e listen() em C.
	s.listener, err = net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Erro ao iniciar: %v", err)
	}
	log.Printf("Servidor ouvindo na porta %s", port)
	log.Printf("Opções de voto: [%s]", s.options.DisplayString)

	// O Event Loop principal. O servidor nunca deve parar.
	for {
		// 2. ACCEPT (Bloqueante)
		// O programa principal PARA nesta linha e espera o SO entregar um cliente.
		// Quando um cliente conecta, o SO cria um socket DEDICADO para ele e nos entrega aqui.
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Erro no accept:", err)
			continue
		}

		// 3. CONCORRÊNCIA (Goroutines)
		// Esta é a chave do desempenho do Go.
		// Não atendemos o cliente na thread principal (senão bloquearíamos novos clientes).
		// Usamos 'go' para lançar uma thread leve (goroutine) que cuidará exclusivamente
		// dessa conexão ('conn') do início ao fim.
		go s.handleClient(conn)
	}
}

// handleClient roda em sua própria goroutine. Uma para cada cliente conectado.
func (s *Server) handleClient(conn net.Conn) {
	// Garante que o socket será fechado quando o cliente desconectar, liberando recursos do SO.
	defer conn.Close()
	
	// bufio.Reader nos dá uma interface de alto nível (ReadString) sobre o stream de bytes cru do socket.
	reader := bufio.NewReader(conn)

	// --- ETAPA DE REGISTRO ---

	// Lê do stream até encontrar uma quebra de linha ('\n').
	// Isso é bloqueante: a goroutine pausa aqui se não houver dados na rede.
	idStr, err := reader.ReadString('\n')
	if err != nil { return }
	id := strings.TrimSpace(idStr)

	// Entramos na Seção Crítica: Vamos mexer na memória compartilhada (mapa s.clients).
	s.mu.Lock() 
	if _, exists := s.clients[id]; exists {
		s.mu.Unlock()
		conn.Write([]byte("ERRO: ID em uso\n"))
		return
	}
	// Guardamos o socket na memória para poder enviar mensagens depois.
	s.clients[id] = conn
	s.mu.Unlock() // Saímos da Seção Crítica. Liberamos o cadeado.

	log.Printf("Conectado: %s", id)
	
	// --- MENSAGEM DE BOAS-VINDAS COM OPÇÕES ---
	// Formata a mensagem usando a string de opções pré-calculada.
	welcomeMsg := fmt.Sprintf("Bem-vindo! Opcoes disponiveis: [%s]. Digite: VOTE [Opcao]\n", s.options.DisplayString)
	// Escreve no stream de saída do socket, enviando bytes de volta ao cliente.
	conn.Write([]byte(welcomeMsg))

	// --- LOOP PRINCIPAL DE COMUNICAÇÃO ---
	for {
		// Fica lendo comandos do cliente indefinidamente.
		msg, err := reader.ReadString('\n')
		if err != nil { 
			// Se der erro (ex: EOF), significa que o cliente fechou o socket.
			// Saímos do loop.
			break 
		}

		msg = strings.TrimSpace(msg)
		// Protocolo Simples: Se começa com "VOTE ", processamos.
		if strings.HasPrefix(msg, "VOTE ") {
			s.processVote(id, strings.TrimPrefix(msg, "VOTE "))
		}
	}

	// --- CLEANUP ---
	// Cliente desconectou. Removemos ele do mapa de forma segura.
	s.mu.Lock()
	delete(s.clients, id)
	s.mu.Unlock()

	log.Printf("Desconectado: %s", id)
	// A função termina e essa goroutine morre.
}

// processVote é a função chamada quando uma goroutine que atende um cliente
// recebe um comando "VOTE X".
func (s *Server) processVote(id, option string) {
	// --- INÍCIO DA SEÇÃO CRÍTICA (Memória Compartilhada) ---
	// Trancamos o Mutex. A partir de agora, nenhuma outra goroutine pode
	// ler ou escrever nos mapas 'votes' e 'voteCounts'.
	// Isso evita "Race Conditions" (corrupção de dados por acesso simultâneo).
	s.mu.Lock()

	// Usamos 'defer' para garantir que o Mutex será destrancado
	// imediatamente antes desta função retornar, não importa o que aconteça (ex: return antecipado ou panic).
	// É uma boa prática em Go para evitar deadlocks.
	defer s.mu.Unlock()

	// Verifica se o cliente já votou.
	// Leitura segura pois estamos com o Lock.
	if _, jaVotou := s.votes[id]; jaVotou {
		// Se já votou, sai da função. O defer s.mu.Unlock() será executado aqui.
		return
	}

	// --- VALIDAÇÃO DA OPÇÃO ---
	// Verifica se a opção votada é válida.
	isValid := false
	for _, validOption := range s.options.List {
		if option == validOption {
			isValid = true
			break
		}
	}

	if !isValid {
		// Se a opção for inválida, ignoramos o voto.
		// (Em um sistema real, enviaríamos uma mensagem de erro ao cliente).
		log.Printf("Voto inválido de %s: %s", id, option)
		return
	}

	// Atualiza o estado na memória.
	// Escrita segura pois estamos com o Lock.
	s.votes[id] = option
	s.voteCounts[option]++
	log.Printf("Voto: %s -> %s", id, option)

	// --- DECISÃO DE ARQUITETURA (Didático) ---
	// Aqui escolhemos entre o modo lento (bloqueante) e o rápido (assíncrono)
	// para demonstrar o impacto de I/O de rede na concorrência.
	if s.useAsyncBroadcast {
		// MODO RÁPIDO (Assíncrono com Channels)

		// 1. Tiramos um "Snapshot" (cópia) do placar atual.
		// Isso é vital pois não podemos passar o mapa original para outra goroutine,
		// já que o mapa original continuará mudando.
		snapshot := make(map[string]int, len(s.voteCounts))
		for k, v := range s.voteCounts {
			snapshot[k] = v
		}

		// 2. Enviamos a cópia para o CANAL (Channel).
		// O Channel é um mecanismo de comunicação seguro entre goroutines.
		// Esta operação é muito rápida e não bloqueia (a menos que o buffer do canal esteja cheio).
		// Assim que enviamos, esta função 'processVote' termina e libera o Mutex rapidamente.
		s.broadcastChan <- snapshot
	} else {
		// MODO LENTO (Bloqueante - A Falha Estratégica para Demonstração)

		// Chamamos a função de broadcast AINDA SEGURANDO O MUTEX PRINCIPAL.
		// Estamos prestes a fazer I/O de Rede (lento) segurando o cadeado da memória RAM (rápido).
		s.broadcastLocked()
		// O Mutex só será liberado (pelo defer) DEPOIS que o broadcastLocked terminar.
	}
}

// broadcastLocked faz o envio segurando o lock principal (Modo Lento).
// ATENÇÃO: Esta função assume que já está rodando DENTRO de um s.mu.Lock().
func (s *Server) broadcastLocked() {
	// Prepara a mensagem string com os dados do mapa de contagem.
	msg := fmt.Sprintf("UPDATE: %v\n", s.voteCounts)
	msgBytes := []byte(msg)

	// Itera sobre todos os clientes conectados.
	for id, conn := range s.clients {

		// --- REGRA DE NEGÓCIO: FILTRO DE ENVIO ---
		// Só enviamos o broadcast para quem JÁ VOTOU.
		// Verificamos se o ID do cliente existe no mapa de votos.
		// (Leitura segura, pois o chamador desta função já segura o Lock).
		if _, votou := s.votes[id]; votou {

			// --- GARGALO DE I/O DE REDE ---
			// conn.Write escreve os bytes no Socket TCP.
			// O Sistema Operacional tenta enviar esses pacotes pela rede.
			// Se a conexão deste cliente específico for lenta (ex: 3G ruim),
			// esta chamada BLOQUEIA a execução até conseguir entregar os dados ao SO.
			// Como estamos segurando o Mutex, TODO O SERVIDOR TRAVA aqui.
			conn.Write(msgBytes)
		}
	}
}

// broadcastWorker é uma GOROUTINE separada que roda em background (Modo Rápido).
// Ela atua como um consumidor da fila de tarefas (o channel).
func (s *Server) broadcastWorker() {
	// Loop infinito utilizando 'range' no canal.
	// Esta é a forma idiomática de Go para consumir dados.
	// Se o canal estiver vazio, esta goroutine "dorme" (fica bloqueada aguardando dados)
	// e não gasta CPU do sistema operacional.
	for update := range s.broadcastChan {

		// A goroutine acordou! Chegou um novo placar (snapshot) para enviar.

		// Prepara os bytes da mensagem uma única vez antes do loop.
		msg := fmt.Sprintf("UPDATE: %v\n", update)
		msgBytes := []byte(msg)

		// Precisamos obter o Lock novamente. Por quê?
		// Porque vamos ler os mapas 's.clients' e 's.votes'.
		// Outras goroutines (handleClient) podem estar adicionando/removendo clientes
		// ou registrando votos simultaneamente.
		s.mu.Lock()

		// Itera sobre os clientes conectados.
		// Nota: Nesta implementação didática, ainda estamos segurando o lock
		// durante o I/O de rede, mas o impacto é menor pois libera a goroutine de votação mais rápido.
		// (A solução "estado da arte" seria usar o padrão Snapshot de clientes aqui também).
		for id, conn := range s.clients {

			// --- REGRA DE NEGÓCIO: FILTRO DE ENVIO ---
			// Verifica se este cliente conectado já registrou seu voto.
			// (Leitura segura do mapa 'votes' graças ao Lock acima).
			if _, votou := s.votes[id]; votou {

				// Realiza a escrita no Socket TCP.
				// É o ponto de contato com a rede.
				conn.Write(msgBytes)
			}
		}
		// Liberamos o Lock após tentar enviar para todos os elegíveis.
		s.mu.Unlock()
	}
}
