# TCP Sockets no Go

## 1. `net.Listen()` → *3 syscalls fundamentais*

Quando você faz:

```
net.Listen("tcp", ":9000")
```

O Go chama no kernel:

1. **`socket()`**
   Cria um file descriptor TCP (ex: `fd=3`).

2. **`bind()`**
   Reserva a porta (ex: 9000).
   Se outra app estiver usando → `EADDRINUSE`.

3. **`listen()`**
   Transforma o socket em modo de escuta e cria duas filas internas:

### 🔹 **SYN Queue (half-open)**

Conexões que ainda não concluíram o 3-way handshake.

### 🔹 **Accept Queue (estabelecidas)**

Conexões prontas para o Go retirar com `Accept()`.
Se encher → conexões novas são descartadas.

---

## 2. `Accept()` → *Entrega um novo fd por cliente*

```
conn, _ := listener.Accept()
```

O Go chama:

### **`accept()`**

* Bloqueia se a Accept Queue estiver vazia.
* Remove a próxima conexão da fila.
* Cria **novo fd exclusivo para esse cliente** (ex: `fd=4`).
* Limite total: **ulimit** (ex: 1024 ou 65536 fds).

Cada cliente = 1 fd + 1 goroutine → Go escala porque goroutines são baratas.

---

## 3. `Write()` → *Pode bloquear!*

```
conn.Write([]byte("msg"))
```

Chamado internamente:

### **`write()`**

Fluxo:

1. Copia dados para o **TCP send buffer** do kernel.
2. TCP fragmenta em MSS (~1460 bytes).
3. Controle de congestionamento decide quando enviar.

### 🔥 Por que pode bloquear?

Porque **TCP é backpressure**:

1. Cliente para de ler.
2. TCP buffer do cliente enche → manda janela zero.
3. Servidor não pode enviar mais.
4. **Send buffer do servidor enche.**
5. `write()` **bloqueia** até liberar espaço.

Esse bloqueio pode durar **segundos**.

---

## 4. `Read()` com bufio → *Redução massiva de syscalls*

Sem `bufio`:

* Cada byte → 1 syscall.
* 100 bytes → 100 syscalls.

Com `bufio.NewReader`:

* Envia **1 syscall** para ler ~4KB.
* O resto é leitura em RAM (nanosegundos).

Eficiência cresce *ordens de magnitude*.

---

## 5. O Problema Real: Mutex + Write Bloqueante

### ❌ Design Problemático

```go
mu.Lock()
for _, conn := range clients {
    conn.Write(data)  // BLOQUEIA se buffer do cliente estiver cheio
}
mu.Unlock()  // Nunca executado durante bloqueio
```

**O que acontece:**

1. Cliente **para de chamar `read()`** (pode ser por qualquer motivo: CPU ocupada, rede congestionada, app pausado)
2. TCP receive buffer do cliente **enche** (típico: 128KB)
3. Servidor tenta enviar **256KB** (maior que buffer disponível)
4. Kernel **retorna EAGAIN/EWOULDBLOCK** → Go bloqueia a goroutine
5. Mutex **permanece travado** durante bloqueio
6. **TODAS as outras votações param** (precisam do mutex)

**Isso NÃO é um ataque, é TCP funcionando corretamente!**

### 🎯 Por que o Teste Usa Payload de 256KB?

**Motivo técnico:** Broadcasts pequenos (~30 bytes) levam **milhares de mensagens** para encher buffer TCP de 128KB.

**Solução para teste:** Enviar **256KB por broadcast** (maior que buffer):

```go
padding := strings.Repeat("\x00", 256*1024)  // 256KB
msg := fmt.Sprintf("UPDATE: %v | SNAPSHOT: %s\n", voteCounts, padding)
```

**Resultado:**
- **1ª ou 2ª mensagem** já excede capacidade do buffer
- `write()` **bloqueia imediatamente**
- Demonstra problema de design **rapidamente**

**Cenários reais equivalentes:**
- Servidores de jogos: snapshots completos de estado (50-500KB)
- Sistemas de log: buffers agregados (100KB-1MB)
- APIs de streaming: chunks de vídeo/dados (256KB-2MB)

### ⚠️ Pré-requisito do Teste

Cliente precisa **votar primeiro** para entrar na lista de broadcast:

```go
fmt.Fprintf(conn, "VOTE A\n")  // Entra na lista
time.Sleep(∞)                   // Para de ler
```

Se não votar, servidor nunca chama `conn.Write()` nele.

### ✅ Solução Arquitetural

```go
mu.Lock()
snapshot := copy(state)  // Microssegundos
mu.Unlock()              // Liberado ANTES do I/O

broadcastChan <- snapshot  // Worker faz I/O separadamente
```

**Worker isolado:**
```go
for update := range broadcastChan {
    conn.Write(update)  // Pode bloquear, mas mutex já foi liberado
}
```

**Resultado:**
- Mutex travado por **< 100 microssegundos**
- Votações **continuam mesmo com buffers cheios**
- Worker bloqueia **isoladamente** sem afetar sistema

---

## 6. Solução: Worker Async + Channels

Modelo correto:

### 🔹 Atualização de estado = rápido

### 🔹 Broadcast = assíncrono em worker separado

```
mu.Lock()
atualiza memória
copia snapshot
broadcastChan <- snapshot
mu.Unlock()
```

Worker:

```
for update := range broadcastChan {
    conn.Write(update)   // pode bloquear, mas fora da seção crítica
}
```

Resultado:

* Mutex fica travado por microssegundos.
* Votações continuam mesmo com clientes lentos.
* Throughput dispara.

---

## 7. Por que o Go escala? (Goroutines vs Threads)

* Thread OS → ~1–2 MB
* Goroutine → ~2 KB
* Go runtime multiplexa milhares de goroutines em poucos threads SO.

Isso permite:

* 10.000 conexões = **10.000 goroutines**
* Sem custo de thread OS
* Sem travar o kernel

---

# 📌 O que realmente importa entender

### ✔ Syscalls criam filas internas no kernel (SYN queue e Accept queue).

### ✔ `Accept()` entrega 1 fd por cliente.

### ✔ `Write()` pode BLOQUEAR por segundos se o cliente não ler.

### ✔ Nunca segure mutex durante operações de rede.

### ✔ Use channels + workers para tornar o servidor imune a clientes lentos.

### ✔ bufio reduz drasticamente a quantidade de syscalls.

### ✔ Go escala usando goroutines muito mais leves que threads.