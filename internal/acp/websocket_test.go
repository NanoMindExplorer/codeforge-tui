package acp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Q6.1 — health, auth, WebSocket initialize round-trip.
func TestServeMuxHealth(t *testing.T) {
	mux := NewServeMux(ServeOptions{Secret: "tok", ACP: Options{Version: "test", Quiet: true, Runner: &fakeRunner{}}})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	buf := make([]byte, 8)
	n, _ := res.Body.Read(buf)
	if string(buf[:n]) != "ok" {
		t.Fatalf("%q", buf[:n])
	}
}

func TestServeMuxUnauthorized(t *testing.T) {
	mux := NewServeMux(ServeOptions{Secret: "secret", ACP: Options{Version: "test", Quiet: true}})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestServeMuxWSInitialize(t *testing.T) {
	mux := NewServeMux(ServeOptions{
		Secret: "s3cr3t",
		ACP: Options{
			Version: "test", Quiet: true, AlwaysApprove: true,
			WorkDir: t.TempDir(), Runner: &fakeRunner{},
		},
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/?secret=s3cr3t"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1},
	}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != nil {
		t.Fatal(resp)
	}
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatalf("no result: %v", resp)
	}
	if result["protocolVersion"].(float64) != 1 {
		t.Fatal(result)
	}
	// capabilities present
	caps, _ := result["agentCapabilities"].(map[string]any)
	if caps["loadSession"] != true {
		t.Fatal(caps)
	}
}

func TestPrepareServeDefaults(t *testing.T) {
	// Cover ServeWebSocket option defaults without blocking on ListenAndServe.
	opt := ServeOptions{ACP: Options{Quiet: true, Runner: &fakeRunner{}, WorkDir: t.TempDir()}}
	// NewServeMux generates secret when empty
	mux := NewServeMux(opt)
	if mux == nil {
		t.Fatal("mux")
	}
	// empty secret on server still serves health
	ts := httptest.NewServer(mux)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/health")
	if err != nil || res.StatusCode != 200 {
		t.Fatal(err, res)
	}
	_ = res.Body.Close()
}

func TestServeMuxBearerAuth(t *testing.T) {
	mux := NewServeMux(ServeOptions{
		Secret: "bearer-tok",
		ACP:    Options{Version: "test", Quiet: true, Runner: &fakeRunner{}, WorkDir: t.TempDir()},
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/"
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer bearer-tok")
	conn, _, err := websocket.DefaultDialer.Dial(u, hdr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": 9, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1},
	})
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"result"`) {
		t.Fatal(string(data))
	}
}
