package main

import (
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestE2E_CurlTransferAndBalance(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not in PATH")
	}

	ts := httptest.NewServer(NewMux(NewStore()))
	defer ts.Close()
	base := ts.URL

	body, code := curl(t, "-sS", "-X", "POST", base+"/transfer-chips",
		"-H", "Content-Type: application/json",
		"-d", `{"transferId":"e2e-1","fromPlayerId":"player-123","toPlayerId":"player-456","amount":2000}`)
	if code != "200" {
		t.Fatalf("transfer status %s body %s", code, body)
	}
	var tr successResponse
	if err := json.Unmarshal([]byte(body), &tr); err != nil {
		t.Fatal(err)
	}
	if !tr.Success || tr.Message != "Transfer completed successfully" {
		t.Fatalf("unexpected transfer response: %+v", tr)
	}

	body, code = curl(t, "-sS", base+"/chip-balance/player-123")
	if code != "200" {
		t.Fatalf("balance status %s body %s", code, body)
	}
	var br balanceResponse
	if err := json.Unmarshal([]byte(body), &br); err != nil {
		t.Fatal(err)
	}
	if br.PlayerID != "player-123" || br.ChipBalance != 8000 {
		t.Fatalf("unexpected balance: %+v", br)
	}

	body, code = curl(t, "-sS", base+"/chip-balance/player-456")
	if code != "200" {
		t.Fatalf("balance status %s body %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &br); err != nil {
		t.Fatal(err)
	}
	if br.PlayerID != "player-456" || br.ChipBalance != 12000 {
		t.Fatalf("unexpected balance: %+v", br)
	}
}

func TestE2E_CurlIdempotentReplay(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not in PATH")
	}
	ts := httptest.NewServer(NewMux(NewStore()))
	defer ts.Close()
	base := ts.URL

	payload := `{"transferId":"e2e-idem","fromPlayerId":"a","toPlayerId":"b","amount":1000}`

	_, code := curl(t, "-sS", "-X", "POST", base+"/transfer-chips",
		"-H", "Content-Type: application/json", "-d", payload)
	if code != "200" {
		t.Fatalf("first: want 200 got %s", code)
	}

	body, code := curl(t, "-sS", "-X", "POST", base+"/transfer-chips",
		"-H", "Content-Type: application/json", "-d", payload)
	if code != "409" {
		t.Fatalf("replay: want 409 got %s body %s", code, body)
	}

	balBody, code := curl(t, "-sS", base+"/chip-balance/a")
	if code != "200" {
		t.Fatalf("balance: %s %s", code, balBody)
	}
	var br balanceResponse
	if err := json.Unmarshal([]byte(balBody), &br); err != nil {
		t.Fatal(err)
	}
	if br.ChipBalance != 9000 {
		t.Fatalf("replay mutated balance: got %d want 9000", br.ChipBalance)
	}
}

func TestE2E_CurlBadRequestCases(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not in PATH")
	}
	ts := httptest.NewServer(NewMux(NewStore()))
	defer ts.Close()
	base := ts.URL

	tests := []struct {
		name string
		json string
	}{
		{"self", `{"transferId":"tx-1","fromPlayerId":"a","toPlayerId":"a","amount":100}`},
		{"max", `{"transferId":"tx-2","fromPlayerId":"a","toPlayerId":"b","amount":5001}`},
		{"insufficient", `{"transferId":"tx-3","fromPlayerId":"a","toPlayerId":"b","amount":20000}`},
		{"negative_amount", `{"transferId":"tx-4","fromPlayerId":"a","toPlayerId":"b","amount":-1}`},
		{"zero_amount", `{"transferId":"tx-5","fromPlayerId":"a","toPlayerId":"b","amount":0}`},
		{"unknown_field", `{"transferId":"tx-6","fromPlayerId":"a","toPlayerId":"b","amount":1,"hack":true}`},
		{"double_json", `{"transferId":"tx-7","fromPlayerId":"a","toPlayerId":"b","amount":1}{}`},
		{"missing_transfer_id", `{"fromPlayerId":"a","toPlayerId":"b","amount":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, code := curl(t, "-sS", "-X", "POST", base+"/transfer-chips",
				"-H", "Content-Type: application/json", "-d", tc.json)
			if code != "400" {
				t.Fatalf("want 400 got %s: %s", code, body)
			}
			var er errorResponse
			if err := json.Unmarshal([]byte(body), &er); err != nil {
				t.Fatal(err)
			}
			if er.Success {
				t.Fatal("expected success false")
			}
		})
	}
}

func TestE2E_CurlTransferEmptyBody(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not in PATH")
	}
	ts := httptest.NewServer(NewMux(NewStore()))
	defer ts.Close()
	body, code := curl(t, "-sS", "-X", "POST", ts.URL+"/transfer-chips",
		"-H", "Content-Type: application/json", "-d", "")
	if code != "400" {
		t.Fatalf("want 400 got %s: %s", code, body)
	}
}

func TestE2E_CurlWrongMethod(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not in PATH")
	}
	ts := httptest.NewServer(NewMux(NewStore()))
	defer ts.Close()
	_, code := curl(t, "-sS", "-X", "GET", ts.URL+"/transfer-chips")
	if code != "405" {
		t.Fatalf("want 405 got %s", code)
	}
}

func TestE2E_CurlBalanceNewPlayer(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not in PATH")
	}
	ts := httptest.NewServer(NewMux(NewStore()))
	defer ts.Close()
	body, code := curl(t, "-sS", ts.URL+"/chip-balance/fresh-player")
	if code != "200" {
		t.Fatalf("status %s %s", code, body)
	}
	var br balanceResponse
	if err := json.Unmarshal([]byte(body), &br); err != nil {
		t.Fatal(err)
	}
	if br.PlayerID != "fresh-player" || br.ChipBalance != InitialChips {
		t.Fatalf("%+v", br)
	}
}

func curl(t *testing.T, args ...string) (body string, statusCode string) {
	t.Helper()
	args = append(args, "-w", "\n%{http_code}")
	out, err := exec.Command("curl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("curl %v: %v: %s", args, err, out)
	}
	s := strings.TrimSpace(string(out))
	idx := strings.LastIndex(s, "\n")
	if idx < 0 {
		t.Fatalf("no status in curl output: %q", s)
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
}
