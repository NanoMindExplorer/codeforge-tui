package acp

import (
	"encoding/json"

	"github.com/codeforge/tui/internal/provider"
)

func (s *Server) xaiSessionForkFixed(params json.RawMessage) any {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(params, &p)
	s.mu.Lock()
	as, ok := s.sess[p.SessionID]
	s.mu.Unlock()
	if !ok {
		return map[string]any{"error": "unknown session"}
	}
	if as.cf == nil {
		return map[string]any{"error": "no durable session"}
	}
	child, err := as.cf.Fork()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	msgs := make([]provider.Message, len(as.Messages))
	copy(msgs, as.Messages)
	nas := &acpSession{
		ID: child.ID, WorkDir: as.WorkDir, Messages: msgs,
		System: as.System, rt: as.rt, tools: as.tools, auth: as.auth, prov: as.prov, cf: child,
	}
	s.mu.Lock()
	s.sess[child.ID] = nas
	s.mu.Unlock()
	return map[string]any{"sessionId": child.ID}
}
