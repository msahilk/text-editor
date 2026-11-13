// Command client is a headless editor client used by the load test.
// It connects to the server, makes random edits, and prints a JSON summary of
// its final document and observed broadcast latencies.
package main

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"text-editor/commons"
	"text-editor/crdt"

	"github.com/gorilla/websocket"
)

type summary struct {
	Name      string    `json:"name"`
	Hash      string    `json:"hash"`
	Length    int       `json:"length"`
	Applied   int       `json:"applied"`
	Pending   int       `json:"pending"`
	Latencies []float64 `json:"latencies_ms"`
}

func main() {
	addr := flag.String("addr", "localhost:8080", "server address")
	ops := flag.Int("ops", 100, "edits to make")
	interval := flag.Duration("interval", 20*time.Millisecond, "time between edits")
	start := flag.Int64("start", 0, "unix-nano time to start editing")
	expected := flag.Int("expect", 0, "remote edits to wait for before finishing")
	timeout := flag.Duration("timeout", 60*time.Second, "max time to wait for remote edits")
	name := flag.String("name", "client", "client name")
	seed := flag.Int64("seed", 1, "random seed")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+*addr+"/", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()

	var writeMu sync.Mutex
	write := func(m commons.Message) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteJSON(m)
	}
	write(commons.Message{Username: *name, Text: "has joined the session.", Type: commons.JoinMessage})

	var (
		mu        sync.Mutex
		doc       = crdt.New()
		pending   []commons.Operation
		latencies []float64
		applied   int
		synced    bool
		preSync   []commons.Operation
	)

	apply := func(op commons.Operation) {
		pending = append(pending, op)
		for progress := true; progress; {
			progress = false
			remaining := pending[:0]
			for _, p := range pending {
				var err error
				if p.Type == "insert" {
					err = doc.RemoteInsert(p.Char)
				} else {
					err = doc.RemoteDelete(p.Char)
				}
				if errors.Is(err, crdt.ErrNotReady) {
					remaining = append(remaining, p)
					continue
				}
				progress = true
			}
			pending = remaining
		}
	}

	go func() {
		for {
			var msg commons.Message
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			now := time.Now().UnixNano()

			mu.Lock()
			switch msg.Type {
			case commons.SiteIDMessage:
				id, _ := strconv.Atoi(msg.Text)
				crdt.SiteID = id
			case commons.DocReqMessage:
				snapshot := crdt.Document{Characters: append([]crdt.Character(nil), doc.Characters...)}
				mu.Unlock()
				write(commons.Message{Type: commons.DocSyncMessage, Document: snapshot, ID: msg.ID})
				continue
			case commons.DocSyncMessage:
				doc = msg.Document
				synced = true
				for _, op := range preSync {
					apply(op)
				}
				preSync = nil
			case commons.JoinMessage, commons.UsersMessage:
			default:
				if msg.Operation.Type == "insert" || msg.Operation.Type == "delete" {
					if !synced {
						preSync = append(preSync, msg.Operation)
					}
					apply(msg.Operation)
					applied++
					if ts, err := strconv.ParseInt(msg.Text, 10, 64); err == nil {
						latencies = append(latencies, float64(now-ts)/1e6)
					}
				}
			}
			mu.Unlock()
		}
	}()

	for time.Now().UnixNano() < *start {
		time.Sleep(time.Millisecond)
	}

	letters := "abcdefghijklmnopqrstuvwxyz"
	for i := 0; i < *ops; i++ {
		mu.Lock()
		n := len(crdt.Content(doc))
		var op commons.Operation
		if n > 5 && rng.Intn(4) == 0 {
			char, ok := doc.GenerateDeleteChar(1 + rng.Intn(n))
			if !ok {
				mu.Unlock()
				continue
			}
			op = commons.Operation{Type: "delete", Char: char}
		} else {
			char, err := doc.GenerateInsertChar(1+rng.Intn(n+1), string(letters[rng.Intn(26)]))
			if err != nil {
				mu.Unlock()
				continue
			}
			op = commons.Operation{Type: "insert", Char: char}
		}
		if !synced {
			preSync = append(preSync, op)
		}
		mu.Unlock()

		// The send time travels in Text so receivers can measure latency.
		write(commons.Message{Type: "operation", Operation: op, Text: strconv.FormatInt(time.Now().UnixNano(), 10)})
		time.Sleep(*interval)
	}

	// Wait until every other client's edits have arrived.
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := applied >= *expected && len(pending) == 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Grace period so slower clients finish before we disconnect.
	time.Sleep(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	content := crdt.Content(doc)
	_ = json.NewEncoder(os.Stdout).Encode(summary{
		Name:      *name,
		Hash:      fmt.Sprintf("%x", sha1.Sum([]byte(content)))[:12],
		Length:    len(content),
		Applied:   applied,
		Pending:   len(pending),
		Latencies: latencies,
	})
}
