// Command loadtest starts the server and many headless clients that edit the
// same document at the same time, then checks that every client ended with an
// identical document and reports broadcast latency.
//
// Usage:
//
//	go build -o server.exe ./server
//	go build -o loadclient.exe ./loadtest/client
//	go run ./loadtest -clients 20 -ops 100
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"
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
	serverBin := flag.String("server", "./server.exe", "server binary")
	clientBin := flag.String("client", "./loadclient.exe", "load-test client binary")
	port := flag.Int("port", 8080, "server port")
	clients := flag.Int("clients", 10, "number of concurrent clients")
	ops := flag.Int("ops", 100, "edits per client")
	interval := flag.Duration("interval", 20*time.Millisecond, "time between each client's edits")
	flag.Parse()

	addr := "localhost:" + strconv.Itoa(*port)
	srv := exec.Command(*serverBin, "-addr", ":"+strconv.Itoa(*port))
	if err := srv.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start server:", err)
		os.Exit(1)
	}
	defer srv.Process.Kill()
	time.Sleep(500 * time.Millisecond)

	// Clients join one at a time (so each receives the document snapshot),
	// then all start editing at the same instant.
	start := time.Now().Add(time.Duration(*clients)*100*time.Millisecond + 2*time.Second).UnixNano()
	expect := (*clients - 1) * *ops

	results := make([]summary, *clients)
	var wg sync.WaitGroup
	for i := 0; i < *clients; i++ {
		cmd := exec.Command(*clientBin,
			"-addr", addr,
			"-ops", strconv.Itoa(*ops),
			"-interval", interval.String(),
			"-start", strconv.FormatInt(start, 10),
			"-expect", strconv.Itoa(expect),
			"-name", "client"+strconv.Itoa(i),
			"-seed", strconv.Itoa(i+1),
		)
		wg.Add(1)
		go func(i int, cmd *exec.Cmd) {
			defer wg.Done()
			out, err := cmd.Output()
			if err != nil {
				fmt.Fprintf(os.Stderr, "client %d: %v\n", i, err)
				return
			}
			_ = json.Unmarshal(out, &results[i])
		}(i, cmd)
		time.Sleep(100 * time.Millisecond)
	}
	wg.Wait()

	hashes := map[string]int{}
	var lat []float64
	minApplied := -1
	for _, r := range results {
		hashes[r.Hash]++
		lat = append(lat, r.Latencies...)
		if minApplied == -1 || r.Applied < minApplied {
			minApplied = r.Applied
		}
	}
	sort.Float64s(lat)
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return lat[int(float64(len(lat)-1)*p)]
	}

	fmt.Printf("clients=%d edits/client=%d total edits=%d\n", *clients, *ops, *clients**ops)
	fmt.Printf("remote edits delivered per client: min %d / expected %d\n", minApplied, expect)
	fmt.Printf("distinct final documents: %d (1 = converged), length %d\n", len(hashes), results[0].Length)
	fmt.Printf("broadcast latency ms: p50=%.2f p95=%.2f p99=%.2f max=%.2f (n=%d)\n",
		pct(0.50), pct(0.95), pct(0.99), pct(1), len(lat))
	if len(hashes) != 1 || minApplied != expect {
		os.Exit(1)
	}
}
