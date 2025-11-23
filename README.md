<h1 align="center">GoVoteTCP: Concurrent Voting Server</h1>

## 📝 Summary <a name="summary"></a>

- [📖 About](#about)
- [📂 Project Structure](#structure)
- [🏁 Getting Started](#getting_started)
- [📱 Usage](#usage)
- [🧪 Load Testing](#load_testing)
- [📚 Documentation](#documentation)
- [⛏️ Technologies Used](#technologies_used)

---

## 📂 Project Structure <a name="structure"></a>

The project follows Go best practices:

```
tcp-vote/
├── cmd/                    # Executable binaries
│   ├── server/            # Server main package
│   └── client/            # Client main package
│
├── internal/              # Private application code
│   └── server/           # Server business logic
│
├── test/                  # Load testing utilities
│   └── loadtest.go
│
├── docs/                  # Technical documentation
│   ├── socket-internals.md   # Deep dive into TCP syscalls
│   └── architecture.md       # System architecture
│
├── logs/                  # Server logs (gitignored)
├── go.mod                # Go module definition
└── README.md            # This file
```

### Directory Purposes

- **`cmd/`**: Entry points for executables. Clean, production-ready code.
- **`internal/`**: Private packages that cannot be imported by other projects.
- **`docs/*.md`**: Technical documentation about syscalls, architecture, and design patterns.
- **`test/`**: Load testing and benchmarking tools.

---

## 📖 About <a name="about"></a>

**GoVoteTCP** is a concurrent voting server written in Go.

It is built specifically as an educational project to demonstrate critical aspects of network programming and concurrency in Go:

- How thousands of TCP clients can connect simultaneously handled by lightweight **goroutines**.
- How shared memory (maps, counters) must be protected using `sync.Mutex`.
- **The critical pitfall:** How holding a mutex during blocking operations (like network I/O) can freeze the entire server.
- **The architectural solution:** The difference between a naive blocking broadcast and a async broadcast using **Go Channels** and the **Worker Pattern**.

The server maintains active TCP client connections, individual votes, aggregated vote counts, and broadcasts updates in real-time according to the selected operational mode.

---

## 🏁 Getting Started <a name="getting_started"></a>

Follow these instructions to get a copy running locally for development and demonstration purposes.

### Cloning the repository

```bash
git clone https://github.com/yLukas077/tcp-vote.git
cd tcp-vote
```

### Prerequisites

You need to have Go 1.21 or higher installed.  
Download Go here: https://go.dev/dl/

Verify the installation:

```bash
go version
```

### Setup

The project includes a `go.mod` file. To ensure all dependencies are downloaded and linked correctly, run:

```bash
go mod tidy
```

---

## 📱 Usage <a name="usage"></a>

### Running the Production Server

```bash
# Run directly
go run cmd/server/main.go

# Or build first
go build -o bin/server cmd/server/main.go
./bin/server
```

The server will:
- Listen on port `:9000`
- Write logs to `logs/server.log`
- Run in asynchronous mode (non-blocking broadcasts)

---

### Running a Client

In a separate terminal:

```bash
# Run directly
go run cmd/client/main.go

# Or build first
go build -o bin/client cmd/client/main.go
./bin/client
```

**Client commands:**
1. Enter your ID when prompted
2. Vote using: `VOTE A` (or B, C)
3. Receive real-time broadcast updates

---

## 🧪 Load Testing <a name="load_testing"></a>

### Running the Load Test

```bash
go run test/loadtest.go
```

**What it does:**
- Spawns 1 "slow client" that never reads data (simulates DoS attack)
- Spawns 50 "fast clients" that vote and read broadcasts normally
- Demonstrates the difference between sync and async modes

**Expected behavior:**
- **Async Mode (default):** Server remains responsive despite slow client
- **Sync Mode:** Server freezes when trying to broadcast to slow client

---

### Using netcat for quick tests

```bash
# Spawn multiple clients
for i in {1..100}; do  
  echo "VOTE A" | nc localhost 9000 &  
done
```

---

## 📚 Documentation <a name="documentation"></a>

### Understanding the Code

The server implementation in `internal/server/server.go` includes extensive inline comments explaining:
- TCP syscalls (`socket`, `bind`, `listen`, `accept`, `read`, `write`)
- How TCP buffers work
- Why mutex + I/O is problematic
- How channels solve the concurrency problem

**Read the source code:**

```bash
# Server implementation with detailed comments
cat internal/server/server.go

# Client implementation
cat cmd/client/main.go

# Load testing utilities
cat test/loadtest.go
```

---

### Technical Deep Dives

1. **[socket-internals.md](docs/socket-internals.md)**
   - Syscalls breakdown (`socket`, `bind`, `listen`, `accept`, `read`, `write`)
   - TCP buffer mechanics (send/receive buffers, sliding window)
   - Why `bufio` reduces syscalls
   - The mutex + blocking I/O anti-pattern

2. **[architecture.md](docs/architecture.md)**
   - System architecture diagrams
   - Goroutine concurrency model
   - Sync vs Async broadcast comparison
   - Step-by-step vote processing flow

---

## 🔧 Building Binaries

```bash
# Build server
go build -o bin/server cmd/server/main.go

# Build client
go build -o bin/client cmd/client/main.go

# Build all at once
mkdir -p bin
go build -o bin/ ./cmd/...
```

---

## 🚀 Running in Different Modes

To switch between sync and async modes, edit `cmd/server/main.go`:

```go
// Async mode (recommended)
srv := server.NewServer(true, opcoes)

// Sync mode (for demonstration of failure)
srv := server.NewServer(false, opcoes)
```

---

The server is designed to highlight a concurrency problem and its solution.  
It supports two distinct modes of operation.

---

## 📝 Key Concepts Demonstrated

1. **TCP Socket Programming**
   - `net.Listen()` → `socket()`, `bind()`, `listen()` syscalls
   - `Accept()` → creating dedicated file descriptors per client
   - `Read()`/`Write()` → TCP buffer interaction

2. **Concurrency Patterns**
   - Goroutine-per-connection model
   - Mutex for shared memory protection
   - Channel-based producer-consumer pattern

3. **Critical Anti-Pattern**
   - Holding mutex during blocking I/O operations
   - Impact of slow clients on entire system

4. **Professional Solution**
   - Async broadcast using channels
   - Worker goroutine for I/O
   - Snapshot pattern for data consistency

---

## ⛏️ Technologies Used <a name="technologies_used"></a>

- **Go (Golang) 1.21+**
- **net package** - TCP socket operations
- **sync.Mutex** - Concurrent memory protection
- **Goroutines** - Lightweight concurrency
- **Channels** - Inter-goroutine communication
- **bufio** - Buffered I/O for syscall reduction
