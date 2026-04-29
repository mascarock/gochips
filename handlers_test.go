package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestHandlers_Transfer_JSONDecodeErrors(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)

	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(`not json`)))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandlers_Balance_Success(t *testing.T) {
	s := NewStore()
	_, _ = s.Balance("z")
	h := balanceHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/chip-balance/z", nil)
	req.SetPathValue("playerId", "z")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var br balanceResponse
	if err := json.NewDecoder(rec.Body).Decode(&br); err != nil {
		t.Fatal(err)
	}
	if br.PlayerID != "z" || br.ChipBalance != InitialChips {
		t.Fatalf("%+v", br)
	}
}

func TestHandlers_Balance_EmptyPath(t *testing.T) {
	s := NewStore()
	h := balanceHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/chip-balance/", nil)
	req.SetPathValue("playerId", "")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandlers_Transfer_EmptyBody(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandlers_Transfer_UnknownJSONField(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	body := `{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b","amount":1,"extra":true}`
	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandlers_Transfer_TrailingJSONGarbage(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	body := `{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b","amount":1}{"oops":1}`
	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandlers_Transfer_WrongAmountTypes(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	cases := []string{
		`{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b","amount":"100"}`,
		`{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b","amount":1.5}`,
		`{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b","amount":null}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status %d", body, rec.Code)
		}
	}
}

func TestHandlers_Transfer_MissingFieldsInferZero(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)

	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty object: status %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(
		`{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b"}`,
	)))
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing amount: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandlers_Transfer_NegativeAmountJSON(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	body := `{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"b","amount":-1}`
	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	var er errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if er.Success || !strings.Contains(er.Error, "positive") {
		t.Fatalf("%+v", er)
	}
}

func TestHandlers_Transfer_MissingTransferID(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	body := `{"fromPlayerId":"a","toPlayerId":"b","amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var er errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(er.Error, "transferId") {
		t.Fatalf("error should mention transferId: %s", er.Error)
	}
}

func TestHandlers_Transfer_IdempotentReplay(t *testing.T) {
	s := NewStore()
	h := transferHandler(s)
	body := `{"transferId":"tx-idem","fromPlayerId":"a","toPlayerId":"b","amount":500}`

	req := httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first: status %d body %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/transfer-chips", bytes.NewReader([]byte(body)))
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("replay: status %d want 409, body %s", rec.Code, rec.Body.String())
	}

	bal, _ := s.Balance("a")
	if bal != 9500 {
		t.Fatalf("balance mutated on replay: a=%d want 9500", bal)
	}
}

func TestMux_BalanceForUnknownPlayer(t *testing.T) {
	s := NewStore()
	mux := NewMux(s)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chip-balance/never-touched", nil)
	req.SetPathValue("playerId", "never-touched")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var br balanceResponse
	if err := json.NewDecoder(rec.Body).Decode(&br); err != nil {
		t.Fatal(err)
	}
	if br.ChipBalance != InitialChips || br.PlayerID != "never-touched" {
		t.Fatalf("%+v", br)
	}
}

func TestMux_WrongMethodReturnsNotAllowed(t *testing.T) {
	s := NewStore()
	mux := NewMux(s)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/transfer-chips", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET transfer: status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/chip-balance/x", nil)
	req.SetPathValue("playerId", "x")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST balance: status %d", rec.Code)
	}
}

func TestMux_UnknownPath404(t *testing.T) {
	mux := NewMux(NewStore())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestMux_ConcurrentHTTPClients(t *testing.T) {
	const numClients = 50
	const roundsPerClient = 80

	s := NewStore()
	ts := httptest.NewServer(NewMux(s))
	defer ts.Close()
	client := ts.Client()

	var wg sync.WaitGroup
	for c := 0; c < numClients; c++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			from := fmt.Sprintf("http-%d", id)
			to := fmt.Sprintf("http-%d", (id+1)%numClients)
			for r := 0; r < roundsPerClient; r++ {
				body, err := json.Marshal(transferRequest{
					TransferID:   fmt.Sprintf("http-tx-%d-%d", id, r),
					FromPlayerID: from,
					ToPlayerID:   to,
					Amount:       1,
				})
				if err != nil {
					t.Error(err)
					return
				}
				req, err := http.NewRequest(http.MethodPost, ts.URL+"/transfer-chips", bytes.NewReader(body))
				if err != nil {
					t.Error(err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("client %d round %d: status %d", id, r, resp.StatusCode)
					return
				}
			}
		}(c)
	}
	wg.Wait()

	const wantTotal = numClients * InitialChips
	var sum int64
	for i := 0; i < numClients; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/chip-balance/http-%d", i), nil)
		req.SetPathValue("playerId", fmt.Sprintf("http-%d", i))
		rec := httptest.NewRecorder()
		NewMux(s).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("balance http-%d: status %d %s", i, rec.Code, rec.Body.String())
		}
		var br balanceResponse
		if err := json.NewDecoder(rec.Body).Decode(&br); err != nil {
			t.Fatal(err)
		}
		sum += br.ChipBalance
	}
	if sum != wantTotal {
		t.Fatalf("chip conservation over HTTP: sum=%d want %d", sum, wantTotal)
	}
}
