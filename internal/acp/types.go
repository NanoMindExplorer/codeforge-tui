// Package acp implements a subset of the Agent Client Protocol (JSON-RPC 2.0)
// for IDE integration: stdio and WebSocket transports (Phase 8).
package acp

import "encoding/json"

// JSON-RPC 2.0 envelope
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // number or string
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// --- initialize ---

type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities map[string]any     `json:"clientCapabilities,omitempty"`
	ClientInfo         *ImplementationInfo `json:"clientInfo,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion   int                 `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities   `json:"agentCapabilities"`
	AgentInfo         ImplementationInfo  `json:"agentInfo"`
	AuthMethods       []any               `json:"authMethods"`
}

type ImplementationInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type AgentCapabilities struct {
	LoadSession        bool               `json:"loadSession"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities"`
	// XAIExtensions lists Grok-compatible x.ai/* methods this agent implements.
	XAIExtensions []string `json:"xaiExtensions,omitempty"`
}

type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// --- session/new ---

type SessionNewParams struct {
	Cwd        string         `json:"cwd"`
	MCPServers []any          `json:"mcpServers,omitempty"`
	Meta       map[string]any `json:"_meta,omitempty"`
}

type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

// --- session/load ---

type SessionLoadParams struct {
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd,omitempty"`
}

type SessionLoadResult struct {
	SessionID string `json:"sessionId"`
}

// --- session/prompt ---

type ContentBlock struct {
	Type     string         `json:"type"` // text | resource | ...
	Text     string         `json:"text,omitempty"`
	Resource map[string]any `json:"resource,omitempty"`
}

type SessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type SessionPromptResult struct {
	StopReason string `json:"stopReason"` // end_turn | cancelled | max_tokens | refusal
}

// --- session/cancel ---

type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// --- session/update notification ---

type SessionUpdateParams struct {
	SessionID string         `json:"sessionId"`
	Update    map[string]any `json:"update"`
}

// Stop reasons
const (
	StopEndTurn   = "end_turn"
	StopCancelled = "cancelled"
	StopMaxTokens = "max_tokens"
	StopRefusal   = "refusal"
)
