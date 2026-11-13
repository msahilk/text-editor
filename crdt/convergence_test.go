package crdt

import (
	"fmt"
	"math/rand"
	"testing"
)

// op is a single edit broadcast from one replica to the others.
type op struct {
	insert bool
	char   Character
}

// replica is one simulated user with its own copy of the document and an
// inbox of edits from other users that it has not applied yet.
type replica struct {
	site    int
	doc     Document
	inbox   []op
	pending []op
}

// apply integrates an op, parking it if its dependencies have not arrived yet.
func (r *replica) apply(o op) {
	r.pending = append(r.pending, o)
	for progress := true; progress; {
		progress = false
		remaining := r.pending[:0]
		for _, p := range r.pending {
			var err error
			if p.insert {
				err = r.doc.RemoteInsert(p.char)
			} else {
				err = r.doc.RemoteDelete(p.char)
			}
			if err == ErrNotReady {
				remaining = append(remaining, p)
				continue
			}
			progress = true
		}
		r.pending = remaining
	}
}

// simulate runs numReplicas users making numOps random edits each, with edits
// delivered to every other user in a random, independently shuffled order.
// It returns the final content of every replica.
func simulate(rng *rand.Rand, numReplicas, numOps int) []string {
	replicas := make([]*replica, numReplicas)
	for i := range replicas {
		replicas[i] = &replica{site: i + 1, doc: New()}
	}

	letters := "abcdefghijklmnopqrstuvwxyz"
	remaining := numReplicas * numOps
	made := make([]int, numReplicas)

	for remaining > 0 || anyInbox(replicas) {
		r := replicas[rng.Intn(numReplicas)]

		// Either deliver a random queued edit (out of order) or make a new edit.
		if len(r.inbox) > 0 && (made[r.site-1] == numOps || rng.Intn(2) == 0) {
			k := rng.Intn(len(r.inbox))
			o := r.inbox[k]
			r.inbox = append(r.inbox[:k], r.inbox[k+1:]...)
			r.apply(o)
			continue
		}
		if made[r.site-1] == numOps {
			continue
		}
		made[r.site-1]++
		remaining--

		SiteID = r.site
		n := len(Content(r.doc))
		var o op
		if n > 3 && rng.Intn(4) == 0 {
			char, ok := r.doc.GenerateDeleteChar(1 + rng.Intn(n))
			if !ok {
				continue
			}
			o = op{insert: false, char: char}
		} else {
			char, err := r.doc.GenerateInsertChar(1+rng.Intn(n+1), string(letters[rng.Intn(26)]))
			if err != nil {
				panic(err)
			}
			o = op{insert: true, char: char}
		}
		for _, other := range replicas {
			if other != r {
				other.inbox = append(other.inbox, o)
			}
		}
	}

	out := make([]string, numReplicas)
	for i, r := range replicas {
		if len(r.pending) != 0 {
			panic(fmt.Sprintf("replica %d has %d unapplied ops", r.site, len(r.pending)))
		}
		out[i] = Content(r.doc)
	}
	return out
}

func anyInbox(rs []*replica) bool {
	for _, r := range rs {
		if len(r.inbox) > 0 {
			return true
		}
	}
	return false
}

// TestConvergence checks that all replicas end with identical documents after
// concurrent edits delivered in random orders.
func TestConvergence(t *testing.T) {
	const trials = 200
	for trial := 0; trial < trials; trial++ {
		rng := rand.New(rand.NewSource(int64(trial)))
		numReplicas := 2 + rng.Intn(7) // 2-8 users
		results := simulate(rng, numReplicas, 30)
		for i := 1; i < len(results); i++ {
			if results[i] != results[0] {
				t.Fatalf("trial %d: replica %d diverged\n  got:  %q\n  want: %q", trial, i+1, results[i], results[0])
			}
		}
	}
}

// TestCharIDsUnique guards against IDs colliding across sites,
// e.g. site 1 / clock 12 and site 11 / clock 2.
func TestCharIDsUnique(t *testing.T) {
	SiteID, LocalClock = 1, 11
	a := newCharID()
	SiteID, LocalClock = 11, 1
	b := newCharID()
	if a == b {
		t.Fatalf("IDs collided: %q", a)
	}
}
