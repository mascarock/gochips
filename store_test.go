package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
)

func txID(prefix string, n int) string {
	return fmt.Sprintf("%s-%d", prefix, n)
}

func TestBalance_InitialAndLazyInit(t *testing.T) {
	s := NewStore()
	b, err := s.Balance("player-1")
	if err != nil {
		t.Fatal(err)
	}
	if b != InitialChips {
		t.Fatalf("initial balance: got %d want %d", b, InitialChips)
	}
	b2, err := s.Balance("player-1")
	if err != nil || b2 != InitialChips {
		t.Fatalf("second read: %v, %d", err, b2)
	}
}

func TestTransfer_Success(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("tx-1", "a", "b", 2000); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Balance("a")
	b, _ := s.Balance("b")
	if a != 8000 || b != 12000 {
		t.Fatalf("balances a=%d b=%d", a, b)
	}
}

func TestTransfer_Self(t *testing.T) {
	s := NewStore()
	err := s.Transfer("tx-self", "a", "a", 100)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrSelfTransfer {
		t.Fatalf("got %v", err)
	}
}

func TestTransfer_Insufficient(t *testing.T) {
	s := NewStore()
	_ = s.Transfer("tx-i1", "a", "b", 5000)
	_ = s.Transfer("tx-i2", "a", "b", 4000)
	err := s.Transfer("tx-i3", "a", "b", 2000)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrInsufficientChips {
		t.Fatalf("got %v", err)
	}
}

func TestTransfer_ExceedsMax(t *testing.T) {
	s := NewStore()
	err := s.Transfer("tx-max", "a", "b", 5001)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrAmountExceedsMax {
		t.Fatalf("got %v", err)
	}
}

func TestTransfer_InvalidAmount(t *testing.T) {
	s := NewStore()
	for i, amt := range []int64{0, -1, -9223372036854775808} {
		err := s.Transfer(txID("tx-inv", i), "a", "b", amt)
		if err != ErrAmountInvalid {
			t.Fatalf("amount %d: got %v", amt, err)
		}
	}
}

func TestTransfer_BoundaryAmounts(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("tx-b1", "a", "b", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Transfer("tx-b2", "a", "b", MaxTransfer); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Balance("a")
	b, _ := s.Balance("b")
	if a != InitialChips-1-MaxTransfer || b != InitialChips+1+MaxTransfer {
		t.Fatalf("a=%d b=%d", a, b)
	}
}

func TestTransfer_ExactBalanceToZero(t *testing.T) {
	s := NewStore()
	_ = s.Transfer("tx-z1", "a", "b", 5000)
	_ = s.Transfer("tx-z2", "a", "b", 5000)
	if err := s.Transfer("tx-z3", "a", "b", 1); err != ErrInsufficientChips {
		t.Fatalf("got %v", err)
	}
	a, _ := s.Balance("a")
	if a != 0 {
		t.Fatalf("a=%d want 0", a)
	}
}

func TestTransfer_ToPlayerNeverSeen(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("tx-new", "only", "brand-new", 100); err != nil {
		t.Fatal(err)
	}
	u, _ := s.Balance("only")
	v, _ := s.Balance("brand-new")
	if u != InitialChips-100 || v != InitialChips+100 {
		t.Fatalf("only=%d new=%d", u, v)
	}
}

func TestTransfer_InsufficientLeavesMapConsistent(t *testing.T) {
	s := NewStore()
	_ = s.Transfer("tx-c1", "a", "b", 5000)
	_ = s.Transfer("tx-c2", "a", "b", 4000)
	err := s.Transfer("tx-c3", "a", "c", 2000)
	if err != ErrInsufficientChips {
		t.Fatalf("got %v", err)
	}
	a, _ := s.Balance("a")
	b, _ := s.Balance("b")
	c, _ := s.Balance("c")
	if a != 1000 || b != InitialChips+9000 || c != InitialChips {
		t.Fatalf("a=%d b=%d c=%d", a, b, c)
	}
}

func TestTransfer_PlayerIDsWithOddCharacters(t *testing.T) {
	s := NewStore()
	from := " player-with-prefix"
	to := "unicode-💰"
	if err := s.Transfer("tx-odd", from, to, 50); err != nil {
		t.Fatal(err)
	}
	x, _ := s.Balance(from)
	y, _ := s.Balance(to)
	if x != InitialChips-50 || y != InitialChips+50 {
		t.Fatalf("%d %d", x, y)
	}
}

func TestTransfer_EmptyPlayerID(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("tx-ep1", "", "b", 1); err != ErrEmptyPlayerID {
		t.Fatalf("from empty: %v", err)
	}
	if err := s.Transfer("tx-ep2", "a", "", 1); err != ErrEmptyPlayerID {
		t.Fatalf("to empty: %v", err)
	}
}

func TestTransfer_EmptyTransferID(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("", "a", "b", 1); err != ErrEmptyTransferID {
		t.Fatalf("got %v", err)
	}
}

func TestBalance_EmptyID(t *testing.T) {
	s := NewStore()
	_, err := s.Balance("")
	if err != ErrEmptyPlayerID {
		t.Fatalf("got %v", err)
	}
}

// --- Idempotency tests ---

func TestTransfer_Idempotent_ReplayDoesNotDoubleDebit(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("tx-once", "a", "b", 1000); err != nil {
		t.Fatal(err)
	}
	err := s.Transfer("tx-once", "a", "b", 1000)
	if err != ErrDuplicateTransfer {
		t.Fatalf("replay: got %v want ErrDuplicateTransfer", err)
	}
	a, _ := s.Balance("a")
	b, _ := s.Balance("b")
	if a != 9000 || b != 11000 {
		t.Fatalf("replay mutated balances: a=%d b=%d", a, b)
	}
}

func TestTransfer_Idempotent_DifferentIDsAreDistinct(t *testing.T) {
	s := NewStore()
	if err := s.Transfer("tx-A", "a", "b", 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.Transfer("tx-B", "a", "b", 1000); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Balance("a")
	b, _ := s.Balance("b")
	if a != 8000 || b != 12000 {
		t.Fatalf("a=%d b=%d", a, b)
	}
}

func TestTransfer_Idempotent_FailedTransferIDNotRecorded(t *testing.T) {
	s := NewStore()
	_ = s.Transfer("drain-1", "a", "b", 5000)
	_ = s.Transfer("drain-2", "a", "b", 5000)

	err := s.Transfer("tx-fail", "a", "b", 1)
	if err != ErrInsufficientChips {
		t.Fatalf("expected insufficient, got %v", err)
	}

	_ = s.Transfer("refund", "b", "a", 500)
	if err := s.Transfer("tx-fail", "a", "b", 1); err != nil {
		t.Fatalf("reuse of failed transfer ID should succeed: %v", err)
	}
}

func TestTransfer_Idempotent_ConcurrentReplays(t *testing.T) {
	s := NewStore()
	const workers = 100
	var okCount int64
	var dupCount int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Transfer("same-tx", "a", "b", 500)
			switch err {
			case nil:
				atomic.AddInt64(&okCount, 1)
			case ErrDuplicateTransfer:
				atomic.AddInt64(&dupCount, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if okCount != 1 {
		t.Fatalf("expected exactly 1 success, got %d", okCount)
	}
	if dupCount != int64(workers)-1 {
		t.Fatalf("expected %d duplicates, got %d", workers-1, dupCount)
	}
	a, _ := s.Balance("a")
	b, _ := s.Balance("b")
	if a != 9500 || b != 10500 {
		t.Fatalf("a=%d b=%d", a, b)
	}
}

// --- Concurrency tests (updated with unique transfer IDs) ---

func TestTransfer_Concurrent_NoLostUpdates(t *testing.T) {
	s := NewStore()
	const n = 200
	const chunk = int64(10)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Transfer(txID("nlu", i), "p-from", "p-to", chunk)
		}(i)
	}
	wg.Wait()
	from, _ := s.Balance("p-from")
	to, _ := s.Balance("p-to")
	wantFrom := InitialChips - n*chunk
	wantTo := InitialChips + n*chunk
	if from != wantFrom || to != wantTo {
		t.Fatalf("from=%d to=%d want %d,%d", from, to, wantFrom, wantTo)
	}
	if from+to != 2*InitialChips {
		t.Fatalf("chip conservation: %d + %d != %d", from, to, 2*InitialChips)
	}
}

func TestTransfer_Concurrent_MixedWithFailures(t *testing.T) {
	s := NewStore()
	const workers = 50
	var okCount int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if s.Transfer(txID("mf", i), "x", "y", 5000) == nil {
				atomic.AddInt64(&okCount, 1)
			}
		}(i)
	}
	wg.Wait()
	if okCount != 2 {
		t.Fatalf("exactly two full max transfers should succeed, got %d", okCount)
	}
	x, _ := s.Balance("x")
	y, _ := s.Balance("y")
	if x != 0 || y != 20000 {
		t.Fatalf("x=%d y=%d", x, y)
	}
	if x+y != 2*InitialChips {
		t.Fatal("chip conservation broken")
	}
}

func TestStore_Concurrency_ChipConservationManyPlayers(t *testing.T) {
	const numPlayers = 40
	const clients = 50
	const iterationsPerClient = 300

	s := NewStore()
	for i := 0; i < numPlayers; i++ {
		id := fmt.Sprintf("p%d", i)
		b, err := s.Balance(id)
		if err != nil || b != InitialChips {
			t.Fatalf("seed %s: %v %d", id, err, b)
		}
	}
	const wantTotal = numPlayers * InitialChips

	var counter int64
	var wg sync.WaitGroup
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := rand.New(rand.NewSource(seed))
			for k := 0; k < iterationsPerClient; k++ {
				a := r.Intn(numPlayers)
				b := r.Intn(numPlayers)
				if a == b {
					continue
				}
				amt := int64(r.Intn(int(MaxTransfer))) + 1
				id := fmt.Sprintf("cc-%d", atomic.AddInt64(&counter, 1))
				_ = s.Transfer(id, fmt.Sprintf("p%d", a), fmt.Sprintf("p%d", b), amt)
			}
		}(int64(1000 + c))
	}
	wg.Wait()

	var sum int64
	for i := 0; i < numPlayers; i++ {
		b, err := s.Balance(fmt.Sprintf("p%d", i))
		if err != nil {
			t.Fatal(err)
		}
		sum += b
	}
	if sum != wantTotal {
		t.Fatalf("chip conservation: sum=%d want %d", sum, wantTotal)
	}
}

func TestBalance_ConcurrentLazyInitSameID(t *testing.T) {
	s := NewStore()
	const workers = 100
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Balance("singleton")
		}()
	}
	wg.Wait()
	b, err := s.Balance("singleton")
	if err != nil || b != InitialChips {
		t.Fatalf("got %v %d want %d", err, b, InitialChips)
	}
}

func TestTransfer_Concurrency_RingLoad(t *testing.T) {
	var counter int64
	for _, clients := range []int{20, 50} {
		clients := clients
		t.Run(fmt.Sprintf("%d_clients", clients), func(t *testing.T) {
			const ring = 30
			const hops = 150
			s := NewStore()
			for i := 0; i < ring; i++ {
				_, _ = s.Balance(fmt.Sprintf("r%d", i))
			}
			wantTotal := int64(ring) * InitialChips

			var wg sync.WaitGroup
			for w := 0; w < clients; w++ {
				wg.Add(1)
				go func(off int) {
					defer wg.Done()
					for h := 0; h < hops; h++ {
						from := (off + h) % ring
						to := (from + 1) % ring
						id := fmt.Sprintf("ring-%d", atomic.AddInt64(&counter, 1))
						_ = s.Transfer(id, fmt.Sprintf("r%d", from), fmt.Sprintf("r%d", to), 1)
					}
				}(w)
			}
			wg.Wait()

			var sum int64
			for i := 0; i < ring; i++ {
				b, _ := s.Balance(fmt.Sprintf("r%d", i))
				sum += b
			}
			if sum != wantTotal {
				t.Fatalf("clients=%d sum=%d want %d", clients, sum, wantTotal)
			}
		})
	}
}

func TestTransfer_Concurrent_Bidirectional(t *testing.T) {
	s := NewStore()
	const workers = 200
	var counter int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("bi-ab-%d", atomic.AddInt64(&counter, 1))
			_ = s.Transfer(id, "alice", "bob", 1)
		}()
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("bi-ba-%d", atomic.AddInt64(&counter, 1))
			_ = s.Transfer(id, "bob", "alice", 1)
		}()
	}
	wg.Wait()
	a, _ := s.Balance("alice")
	b, _ := s.Balance("bob")
	if a+b != 2*InitialChips {
		t.Fatalf("chip conservation: alice=%d bob=%d sum=%d", a, b, a+b)
	}
	if a < 0 || b < 0 {
		t.Fatalf("negative balance: alice=%d bob=%d", a, b)
	}
}

func TestTransfer_Concurrent_ReadsWhileWriting(t *testing.T) {
	s := NewStore()
	const writers = 50
	const readers = 100
	const iters = 200
	var counter int64
	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < iters; k++ {
				id := fmt.Sprintf("rw-%d", atomic.AddInt64(&counter, 1))
				_ = s.Transfer(id, "src", "dst", 1)
			}
		}()
	}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < iters; k++ {
				bal, err := s.Balance("src")
				if err != nil {
					t.Errorf("balance err: %v", err)
					return
				}
				if bal < 0 {
					t.Errorf("negative balance observed: %d", bal)
					return
				}
			}
		}()
	}
	wg.Wait()

	src, _ := s.Balance("src")
	dst, _ := s.Balance("dst")
	if src+dst != 2*InitialChips {
		t.Fatalf("conservation: src=%d dst=%d", src, dst)
	}
}

func TestTransfer_Concurrent_DrainOnePlayer(t *testing.T) {
	s := NewStore()
	const amt = int64(100)
	const workers = 500
	var okCount int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			to := fmt.Sprintf("recv-%d", i)
			if s.Transfer(txID("drain", i), "drain-src", to, amt) == nil {
				atomic.AddInt64(&okCount, 1)
			}
		}(i)
	}
	wg.Wait()

	wantOK := InitialChips / amt
	if okCount != wantOK {
		t.Fatalf("okCount=%d want %d", okCount, wantOK)
	}
	src, _ := s.Balance("drain-src")
	if src != 0 {
		t.Fatalf("src=%d want 0", src)
	}

	var recvSum int64
	for i := 0; i < workers; i++ {
		b, _ := s.Balance(fmt.Sprintf("recv-%d", i))
		recvSum += b
	}
	totalChips := src + recvSum
	wantTotal := InitialChips + int64(workers)*InitialChips
	if totalChips != wantTotal {
		t.Fatalf("total=%d want %d", totalChips, wantTotal)
	}
}

func TestTransfer_Concurrent_LazyInitViaTransfer(t *testing.T) {
	s := NewStore()
	const workers = 100
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Transfer(txID("lazy", i), "lazy-sender", fmt.Sprintf("lazy-recv-%d", i), 1)
		}(i)
	}
	wg.Wait()

	sender, _ := s.Balance("lazy-sender")
	if sender != InitialChips-int64(workers) {
		t.Fatalf("sender=%d want %d", sender, InitialChips-int64(workers))
	}
	for i := 0; i < workers; i++ {
		b, _ := s.Balance(fmt.Sprintf("lazy-recv-%d", i))
		if b != InitialChips+1 {
			t.Fatalf("recv-%d=%d want %d", i, b, InitialChips+1)
		}
	}
}

func TestTransfer_Concurrent_StarTopology(t *testing.T) {
	s := NewStore()
	const senders = 100
	const amt = int64(500)
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Transfer(txID("star", i), fmt.Sprintf("star-%d", i), "hub", amt)
		}(i)
	}
	wg.Wait()

	hub, _ := s.Balance("hub")
	wantHub := InitialChips + int64(senders)*amt
	if hub != wantHub {
		t.Fatalf("hub=%d want %d", hub, wantHub)
	}
	var sum int64
	for i := 0; i < senders; i++ {
		b, _ := s.Balance(fmt.Sprintf("star-%d", i))
		sum += b
	}
	sum += hub
	wantTotal := int64(senders+1) * InitialChips
	if sum != wantTotal {
		t.Fatalf("conservation: sum=%d want %d", sum, wantTotal)
	}
}

func TestTransfer_Concurrent_PingPong(t *testing.T) {
	s := NewStore()
	const rounds = 500
	var counter int64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			id := fmt.Sprintf("pp-ab-%d", atomic.AddInt64(&counter, 1))
			_ = s.Transfer(id, "ping", "pong", 1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			id := fmt.Sprintf("pp-ba-%d", atomic.AddInt64(&counter, 1))
			_ = s.Transfer(id, "pong", "ping", 1)
		}
	}()
	wg.Wait()

	ping, _ := s.Balance("ping")
	pong, _ := s.Balance("pong")
	if ping+pong != 2*InitialChips {
		t.Fatalf("conservation: ping=%d pong=%d", ping, pong)
	}
	if ping < 0 || pong < 0 {
		t.Fatalf("negative: ping=%d pong=%d", ping, pong)
	}
}

func TestTransfer_Concurrent_AllFail(t *testing.T) {
	s := NewStore()
	_, _ = s.Balance("only")
	var wg sync.WaitGroup
	const workers = 200
	for i := 0; i < workers; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			_ = s.Transfer(txID("af-self", i), "only", "only", 1)
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = s.Transfer(txID("af-max", i), "only", "other", MaxTransfer+1)
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = s.Transfer(txID("af-neg", i), "only", "other", -5)
		}(i)
	}
	wg.Wait()
	b, _ := s.Balance("only")
	if b != InitialChips {
		t.Fatalf("balance changed to %d after only invalid transfers", b)
	}
}
