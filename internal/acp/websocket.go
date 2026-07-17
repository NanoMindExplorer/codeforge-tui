package acp

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ServeOptions for WebSocket ACP server.
type ServeOptions struct {
	Bind   string // e.g. 127.0.0.1:2419
	Secret string // required bearer / query token
	ACP    Options
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// large messages for file embeds
	ReadBufferSize:  1024 * 64,
	WriteBufferSize: 1024 * 64,
}

// NewServeMux builds the HTTP handler for agent serve (Q6.1 — testable).
// Routes:
//
//	GET /health  → "ok"
//	GET /        → WebSocket ACP (requires secret)
func NewServeMux(opt ServeOptions) http.Handler {
	if opt.Secret == "" {
		opt.Secret = os.Getenv("CODEFORGE_AGENT_SECRET")
	}
	if opt.Secret == "" {
		opt.Secret = fmt.Sprintf("cf-%d", time.Now().UnixNano()%1e12)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Auth: Authorization: Bearer <secret> or ?secret=
		tok := r.URL.Query().Get("secret")
		if tok == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				tok = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if tok != opt.Secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Non-WS GET returns a tiny help page (useful for browsers)
		if !websocket.IsWebSocketUpgrade(r) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprintf(w, "codeforge ACP WebSocket — upgrade required\nsecret via ?secret= or Authorization: Bearer\n")
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade: %v", err)
			return
		}
		defer conn.Close()
		handleWS(conn, opt.ACP)
	})
	return mux
}

// ServeWebSocket starts an HTTP server that upgrades to ACP JSON-RPC over WS.
func ServeWebSocket(opt ServeOptions) error {
	if opt.Bind == "" {
		opt.Bind = "127.0.0.1:2419"
	}
	if opt.Secret == "" {
		opt.Secret = os.Getenv("CODEFORGE_AGENT_SECRET")
	}
	if opt.Secret == "" {
		// generate ephemeral
		opt.Secret = fmt.Sprintf("cf-%d", time.Now().UnixNano()%1e12)
		fmt.Fprintf(os.Stderr, "codeforge agent serve: generated secret (also set CODEFORGE_AGENT_SECRET):\n  %s\n", opt.Secret)
	}

	fmt.Fprintf(os.Stderr, "codeforge agent serve — WebSocket ACP on ws://%s/\n", opt.Bind)
	fmt.Fprintf(os.Stderr, "  auth: ?secret=… or Authorization: Bearer …\n")
	return http.ListenAndServe(opt.Bind, NewServeMux(opt))
}

type wsTransport struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (t *wsTransport) Write(msg any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteJSON(msg)
}

func handleWS(conn *websocket.Conn, opt Options) {
	srv := NewServer(opt)
	tx := &wsTransport{conn: conn}
	srv.SetTransport(tx)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// allow multi-line payloads
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			srv.Handle([]byte(line))
		}
	}
}
