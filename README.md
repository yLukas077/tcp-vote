<h1 align="center">GoVoteTCP: Concurrent Voting Server</h1>

## 📝 Summary <a name="summary"></a>

- [📖 About](#about)
- [🏁 Getting Started](#getting_started)
- [📱 Usage](#usage)
- [🧪 Load Testing](#load_testing)
- [⛏️ Technologies Used](#technologies_used)

---

## 📖 About <a name="about"></a>

**GoVoteTCP** is a high-performance concurrent voting server written in Go.

It is built specifically as an educational project to demonstrate critical aspects of network programming and concurrency in Go:

- How thousands of TCP clients can connect simultaneously handled by lightweight **goroutines**.
- How shared memory (maps, counters) must be protected using `sync.Mutex`.
- **The critical pitfall:** How holding a mutex during blocking operations (like network I/O) can freeze the entire server.
- **The architectural solution:** The difference between a naive blocking broadcast and a professional async broadcast using **Go Channels** and the **Worker Pattern**.

The server maintains active TCP client connections, individual votes, aggregated vote counts, and broadcasts updates in real-time according to the selected operational mode.

---

## 🏁 Getting Started <a name="getting_started"></a>

Follow these instructions to get a copy running locally for development and demonstration purposes.

### Cloning the repository

```bash
git clone https://github.com/SEU_USER/SEU_REPO.git  
cd SEU_REPO  
```

*(Replace SEU_USER and SEU_REPO with your actual GitHub username and repository name.)*

### Prerequisites

You need to have Go 1.21 or higher installed.  
Download Go here: https://go.dev/dl/

Verify the installation:

```bash
go version
```

### Setup

The project includes a go.mod file. To ensure all dependencies are downloaded and linked correctly, run:

```bash
go mod tidy
```

---

## 📱 Usage <a name="usage"></a>

The server is designed to highlight a concurrency problem and its solution.  
It supports two distinct modes of operation:

---

### 1️⃣ Sync Broadcast Mode (Blocking — Demonstration of Failure)

Every broadcast operation holds the global mutex while iterating through clients and performing network I/O (`conn.Write`).

**Why use it:**  
To demonstrate how a single slow client can cause the entire server to "freeze" for everyone else, blocking new connections and votes.

---

### 2️⃣ Async Broadcast Mode (Non-Blocking — Professional Solution)

The vote handler:

- acquires the lock briefly  
- updates memory  
- takes a snapshot of the data  
- sends it to a buffered channel  
- releases the lock immediately  

A dedicated worker goroutine consumes the channel and handles the slow network broadcast independently.

**Why use it:**  
To keep the server responsive under high load, even with slow clients.

---

## ▶️ Running the Server

Run using `go run`:

```bash
go run main.go  
go run main.go -async
```

Build and run:

```bash
go build -o server  
./server -async  
./server -sync
```

*(Ensure your main.go implements flags if required.)*

---

## 🧪 Load Testing <a name="load_testing"></a>

### 🔹 Using the included Go load tester

```bash
go run teste_carga.go
```

To increase load, adjust inside loadtest.go:

totalBots := 2000

### 🔹 Using basic shell + netcat

```bash
for i in {1..2000}; do  
&nbsp;&nbsp;echo "VOTE A" | nc localhost 9000 &  
done
```

### What to expect

- **Sync Mode:** freezes, delays, blocked voting  
- **Async Mode:** stays responsive even with slow clients

---

## ⛏️ Technologies Used <a name="technologies_used"></a>

- Go (Golang)  
- net.TCP  
- sync.Mutex  
- bufio  
