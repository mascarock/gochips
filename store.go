package main

import (
	"errors"
	"sync"
)

// business rules: apply the same rules as the ledger.go file
const (
	InitialChips int64 = 10_000
	MaxTransfer  int64 = 5_000
)

// definition of basic erros to avoid issues

var (
	ErrSelfTransfer      = errors.New("self transfers are not allowed")
	ErrAmountInvalid     = errors.New("amount must be a positive integer")
	ErrAmountExceedsMax  = errors.New("amount exceeds maximum of 5000")
	ErrInsufficientChips = errors.New("insufficient chips")
	ErrEmptyPlayerID     = errors.New("playerId is required")
	ErrEmptyTransferID   = errors.New("transferId is required")
	ErrDuplicateTransfer = errors.New("duplicate transfer")
)

// we implement a simple store in memory.
type Store struct {
	mu          sync.RWMutex
	balances    map[string]int64
	completedTx map[string]struct{}
}

func NewStore() *Store {
	return &Store{
		balances:    make(map[string]int64),
		completedTx: make(map[string]struct{}),
	}
}

func (s *Store) Balance(id string) (int64, error) {
	if id == "" {
		return 0, ErrEmptyPlayerID
	}
	s.mu.RLock()
	if v, ok := s.balances[id]; ok {
		s.mu.RUnlock()
		return v, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.balances[id]; ok {
		return v, nil
	}
	s.balances[id] = InitialChips
	return InitialChips, nil
}

func (s *Store) Transfer(transferID, from, to string, amount int64) error {
	if transferID == "" {
		return ErrEmptyTransferID
	}
	if from == "" || to == "" {
		return ErrEmptyPlayerID
	}
	if from == to {
		return ErrSelfTransfer
	}
	if amount <= 0 {
		return ErrAmountInvalid
	}
	if amount > MaxTransfer {
		return ErrAmountExceedsMax
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, seen := s.completedTx[transferID]; seen {
		return ErrDuplicateTransfer
	}

	fromBal, ok := s.balances[from]
	if !ok {
		fromBal = InitialChips
	}
	if fromBal < amount {
		return ErrInsufficientChips
	}

	toBal, ok := s.balances[to]
	if !ok {
		toBal = InitialChips
	}

	s.balances[from] = fromBal - amount
	s.balances[to] = toBal + amount
	s.completedTx[transferID] = struct{}{}
	return nil
}
