# Arquitetura do Sistema TCP-Vote ß

Este documento apresenta uma visão clara, direta e essencial da arquitetura do servidor de votação TCP concorrente desenvolvido em Go. Ele resume como o sistema funciona, seus componentes principais e o fluxo geral de comunicação.

---

## 🏗️ Visão Geral da Arquitetura

```
┌─────────────────────────────────────────────────────────────┐
│                           CLIENTES                           │
│   Client 1   Client 2   Client 3   ...   Client N            │
└─────────────────────────────────────────────────────────────┘
                              │ TCP/IP
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     SERVIDOR (Port 9000)                     │
│                                                             │
│  Main Goroutine                                              │
│  └── listener.Accept()                                       │
│        └── go handleClient(conn)                             │
│                                                             │
│  Cada cliente → 1 goroutine própria                          │
│                                                             │
│  Estruturas protegidas por mutex:                            │
│    - clients: conexões ativas                                │
│    - votes: voto de cada cliente                             │
│    - voteCounts: contagem global                             │
│                                                             │
│  Broadcast:                                                  │
│    - Modo Sync (bloqueante)                                  │
│    - Modo Async (channel + worker dedicado)                  │
└─────────────────────────────────────────────────────────────┘
```

---

## 🔄 Fluxo Básico do Sistema

### 1. Conexão do Cliente

* Cliente realiza o handshake TCP.
* Servidor recebe via `Accept()` e cria uma nova goroutine para tratá-lo.

### 2. Registro

* Cliente envia seu identificador.
* Servidor armazena o ID no mapa de clientes (mutex).

### 3. Votação

* Cliente envia: `VOTE X`
* Servidor:

  * Valida o voto.
  * Atualiza mapas protegidos.
  * Dispara broadcast com o estado atualizado.

### 4. Broadcast

* Pode ser:

  * **Sync**: envio dentro do mutex (bloqueante, pode travar).
  * **Async**: snapshot enviado para worker em canal (não trava votações).

---

## ⚙️ Concorrência e Estruturas Internas

### Goroutines principais

* **Main Goroutine** → Aceita conexões.
* **N Client Goroutines** → Uma goroutine por cliente.
* **Broadcast Worker (Async)** → Envia mensagens sem bloquear votações.

### Estrutura protegida por mutex

```
Server {
  mu          sync.Mutex
  clients     map[string]net.Conn
  votes       map[string]string
  voteCounts  map[string]int
}
```

### Padrão de Acesso

* Todas as leituras/escritas nos mapas ocorrem dentro de `mu.Lock()` / `mu.Unlock()`.
* No modo async, o mutex é liberado rapidamente (< 1 ms).

---

## 📡 Broadcast: Sync vs Async (Resumo)

### Sync (Bloqueante)

* Envia mensagens dentro do mutex.
* Se um cliente for lento → trava todos.
* Baixo throughput.

**Importante:** Apenas clientes que **já votaram** recebem broadcasts. Isso é implementado pela verificação:

```go
for id, conn := range s.clients {
    if _, votou := s.votes[id]; votou {  // ← Filtro crítico
        conn.Write(msgBytes)
    }
}
```

### Async (Recomendado)

* Captura do snapshot sob mutex.
* Envia snapshot para canal.
* Worker faz o broadcast fora do mutex.
* Clientes lentos não afetam o processamento do voto.

---

## 🚦 Ciclo de Vida do Cliente

```
DISCONNECTED → CONNECTED → REGISTERED → VOTED → DISCONNECTED
```

* Clientes recebem atualizações sempre que o estado global muda.
* Ao desconectar, o servidor remove o cliente do mapa.

---

## 🧱 Componentes do Sistema

### 1. Listener (Main Goroutine)

Aceita conexões e inicia goroutines de cliente.

### 2. Client Handler

Responsável por:

* Registrar ID
* Ler comandos
* Invocar processamento de voto
* Fazer cleanup ao desconectar

### 3. Processador de Voto

Realiza:

* Validação da opção
* Atualização de `votes` e `voteCounts`
* Disparo do broadcast (sync ou async)

### 4. Broadcast Worker (modo async)

Envia atualizações para todos os clientes de forma desacoplada.

---

## 🎯 Princípios Arquiteturais Utilizados

* **Goroutine-per-connection**: simples e altamente escalável.
* **Mutex apenas para memória**, nunca para operações de rede.
* **Channels para desacoplamento** entre etapas rápidas e lentas.
* **Snapshot pattern** para garantir segurança e não bloquear o sistema.
* **I/O assíncrono** para máxima escalabilidade.

---

## 📊 Resumo de Performance

| Métrica                    | Sync  | Async      |
| -------------------------- | ----- | ---------- |
| Bloqueio no mutex          | Alto  | Quase zero |
| Throughput                 | Baixo | Altíssimo  |
| Cliente lento afeta todos? | Sim   | Não        |
| Escalabilidade             | Ruim  | Excelente  |
