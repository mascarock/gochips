package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type transferRequest struct {
	TransferID   string `json:"transferId"`
	FromPlayerID string `json:"fromPlayerId"`
	ToPlayerID   string `json:"toPlayerId"`
	Amount       int64  `json:"amount"`
}

type successResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type errorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type balanceResponse struct {
	PlayerID    string `json:"playerId"`
	ChipBalance int64  `json:"chipBalance"`
}

func NewMux(s *Store) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /transfer-chips", transferHandler(s))
	mux.HandleFunc("GET /chip-balance/{playerId}", balanceHandler(s))
	return mux
}

func transferHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}
		var req transferRequest
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}
		if len(bytes.TrimSpace(raw[dec.InputOffset():])) > 0 {
			writeErr(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if err := s.Transfer(req.TransferID, req.FromPlayerID, req.ToPlayerID, req.Amount); err != nil {
			writeErr(w, statusFor(err), err)
			return
		}

		writeJSON(w, http.StatusOK, successResponse{
			Success: true,
			Message: "Transfer completed successfully",
		})
	}
}

func balanceHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("playerId")
		bal, err := s.Balance(id)
		if err != nil {
			writeErr(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, balanceResponse{PlayerID: id, ChipBalance: bal})
	}
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrDuplicateTransfer):
		return http.StatusConflict
	case errors.Is(err, ErrSelfTransfer),
		errors.Is(err, ErrAmountInvalid),
		errors.Is(err, ErrAmountExceedsMax),
		errors.Is(err, ErrInsufficientChips),
		errors.Is(err, ErrEmptyPlayerID),
		errors.Is(err, ErrEmptyTransferID):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Success: false, Error: err.Error()})
}
