package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/app"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/skills"
	"github.com/codeforge/tui/internal/tool"
)

// Options configure the ACP agent server.
type Options struct {
	Version        string
	WorkDir        string // default cwd when session omits
	Model          string
	AlwaysApprove  bool
	DontAsk        bool
	Plan           bool
	MaxIter        int
	// Quiet suppresses stderr banners during bootstrap
	Quiet bool
	// Runner is optional override for tests (nil = real agent)
	Runner PromptRunner
}

// PromptRunner runs one agent turn (for tests / real).
type PromptRunner interface {
	Run(ctx context.Context, workdir, system string, msgs []provider.Message, auth agent.Authorizer, maxIter int, onEvent func(agent.Event))
}

// Transport writes JSON lines (responses + notifications) to the client.
type Transport interface {
	Write(msg any) error
}

// Server is the ACP agent side.
type Server struct {
	opt    Options
	mu     sync.Mutex
	sess   map[string]*acpSession
	// active prompt cancel
	cancels map[string]context.CancelFunc
	tx      Transport
	// initialized
	ready bool
	// x.ai/terminal/* state
	terminals map[string]*acpTerminal
	// lastToolCallID maps sessionID → last toolCallId for update correlation
	lastToolCallID map[string]string
}

// acpTerminal is a background shell job for x.ai/terminal extensions.
type acpTerminal struct {
	ID       string
	Command  string
	Cwd      string
	Output   string
	Error    string
	ExitCode int
	Done     bool
	Started  time.Time
	Ended    time.Time
}

type acpSession struct {
	ID       string
	WorkDir  string
	Messages []provider.Message
	System   string
	// Runtime wired on first prompt
	rt    *app.Runtime
	tools *tool.Registry
	auth  *permission.Engine
	prov  provider.Provider
	cf    *session.Session // CodeForge durable session
}

// NewServer creates an ACP server (transport set via SetTransport).
func NewServer(opt Options) *Server {
	if opt.Version == "" {
		opt.Version = "1.8.2"
	}
	if opt.MaxIter <= 0 {
		opt.MaxIter = 12
	}
	return &Server{
		opt:            opt,
		sess:           map[string]*acpSession{},
		cancels:        map[string]context.CancelFunc{},
		lastToolCallID: map[string]string{},
	}
}

// SetTransport sets the outbound writer.
func (s *Server) SetTransport(tx Transport) { s.tx = tx }

// Handle processes one JSON-RPC line (request or notification).
func (s *Server) Handle(line []byte) {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.replyError(nil, CodeParseError, "parse error")
		return
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		s.replyError(req.ID, CodeInvalidRequest, "jsonrpc must be 2.0")
		return
	}
	// Notification (no id)
	if len(req.ID) == 0 || string(req.ID) == "null" {
		s.handleNotification(req)
		return
	}
	s.handleRequest(req)
}

func (s *Server) handleNotification(req Request) {
	switch req.Method {
	case "session/cancel":
		var p SessionCancelParams
		_ = json.Unmarshal(req.Params, &p)
		s.cancelSession(p.SessionID)
	default:
		// ignore unknown notifications
	}
}

func (s *Server) handleRequest(req Request) {
	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.doInitialize(req.Params))
	case "authenticate":
		// no-op auth for now
		s.reply(req.ID, map[string]any{})
	case "session/new":
		res, err := s.doSessionNew(req.Params)
		if err != nil {
			s.replyError(req.ID, CodeInvalidParams, err.Error())
			return
		}
		s.reply(req.ID, res)
	case "session/load":
		res, err := s.doSessionLoad(req.Params)
		if err != nil {
			s.replyError(req.ID, CodeInvalidParams, err.Error())
			return
		}
		s.reply(req.ID, res)
	case "session/prompt":
		// long-running: process async so cancel works
		go s.doSessionPrompt(req)
	default:
		if s.handleXAI(req) {
			return
		}
		s.replyError(req.ID, CodeMethodNotFound, "method not found: "+req.Method)
	}
}

func (s *Server) doInitialize(params json.RawMessage) InitializeResult {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	var p InitializeParams
	_ = json.Unmarshal(params, &p)
	ver := 1
	if p.ProtocolVersion > 0 {
		ver = p.ProtocolVersion
	}
	return InitializeResult{
		ProtocolVersion: ver,
		AgentCapabilities: AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: PromptCapabilities{
				EmbeddedContext: true,
			},
			XAIExtensions: XAIExtensions(),
		},
		AgentInfo: ImplementationInfo{
			Name:    "codeforge",
			Title:   "CodeForge Agent",
			Version: s.opt.Version,
		},
		AuthMethods: []any{},
	}
}

func (s *Server) doSessionNew(params json.RawMessage) (SessionNewResult, error) {
	var p SessionNewParams
	if err := json.Unmarshal(params, &p); err != nil {
		return SessionNewResult{}, err
	}
	cwd := p.Cwd
	if cwd == "" {
		cwd = s.opt.WorkDir
	}
	if cwd == "" {
		cwd, _ = filepath.Abs(".")
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	// Bootstrap runtime once per session
	rt, err := app.Bootstrap(app.Options{
		WorkDir:   cwd,
		Quiet:     true,
		ActMode:   s.opt.AlwaysApprove || !s.opt.Plan,
		PlanMode:  s.opt.Plan,
		SkipIndex: false,
	})
	if err != nil {
		return SessionNewResult{}, err
	}
	prov, err := rt.ProvReg.Current()
	if err != nil {
		return SessionNewResult{}, err
	}
	if s.opt.Model != "" {
		_ = prov.SetModel(s.opt.Model)
	}

	eng := permission.FromConfig(rt.Cfg, cwd)
	eng.Headless = true
	if s.opt.AlwaysApprove {
		eng.SetMode(permission.ModeAlwaysApprove)
	}
	if s.opt.DontAsk {
		eng.SetMode(permission.ModeDontAsk)
	}
	if s.opt.Plan {
		eng.SetMode(permission.ModePlan)
	}
	tool.SubagentAuthorizer = eng

	sys := `You are CodeForge ACP agent for IDE integration. Be concise and complete tasks.
Prefer search_replace/apply_patch. Use tools when needed. Reply in the user's language.`
	sys = rules.Inject(sys, rt.Rules)
	sys = skills.Global().InjectCatalog(sys)
	if p.Meta != nil {
		if r, ok := p.Meta["rules"].(string); ok && r != "" {
			sys += "\n\n" + r
		}
		if o, ok := p.Meta["systemPromptOverride"].(string); ok && o != "" {
			sys = o
		}
	}

	// Persist CodeForge session id
	cfSess := session.New(rt.ProvReg.CurrentName(), prov.Model(), cwd)
	_ = cfSess.Save()
	id := cfSess.ID

	as := &acpSession{
		ID: id, WorkDir: cwd, System: sys,
		rt: rt, tools: rt.ToolReg, auth: eng, prov: prov, cf: cfSess,
	}
	// Wire plan path
	if sw := rt.ToolReg.GetStagedWriter(); sw != nil {
		if pp, err := cfSess.PlanPath(); err == nil {
			sw.SetPlanPath(pp)
		}
		if s.opt.AlwaysApprove {
			sw.SetMode(tool.ModeAct)
		} else if s.opt.Plan {
			sw.SetMode(tool.ModeDesign)
		} else {
			sw.SetMode(tool.ModePlan)
		}
	}

	s.mu.Lock()
	s.sess[id] = as
	s.mu.Unlock()
	return SessionNewResult{SessionID: id}, nil
}

func (s *Server) doSessionLoad(params json.RawMessage) (SessionLoadResult, error) {
	var p SessionLoadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return SessionLoadResult{}, err
	}
	if p.SessionID == "" {
		return SessionLoadResult{}, fmt.Errorf("sessionId required")
	}
	loaded, err := session.Load(p.SessionID)
	if err != nil {
		return SessionLoadResult{}, err
	}
	cwd := loaded.Workdir
	if p.Cwd != "" {
		cwd = p.Cwd
	}
	// Bootstrap under loaded cwd
	res, err := s.doSessionNew(mustJSON(SessionNewParams{Cwd: cwd}))
	if err != nil {
		return SessionLoadResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	as, ok := s.sess[res.SessionID]
	if !ok {
		return SessionLoadResult{}, fmt.Errorf("session init failed")
	}
	// Re-key to loaded ID and restore messages
	delete(s.sess, res.SessionID)
	as.ID = loaded.ID
	as.Messages = append([]provider.Message(nil), loaded.Messages...)
	as.cf = loaded
	s.sess[loaded.ID] = as
	if sw := as.tools.GetStagedWriter(); sw != nil {
		if pp, err := loaded.PlanPath(); err == nil {
			sw.SetPlanPath(pp)
		}
	}
	return SessionLoadResult{SessionID: loaded.ID}, nil
}

func (s *Server) doSessionPrompt(req Request) {
	var p SessionPromptParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.replyError(req.ID, CodeInvalidParams, err.Error())
		return
	}
	s.mu.Lock()
	as, ok := s.sess[p.SessionID]
	s.mu.Unlock()
	if !ok {
		s.replyError(req.ID, CodeInvalidParams, "unknown sessionId")
		return
	}

	text := extractPromptText(p.Prompt)
	if strings.TrimSpace(text) == "" {
		s.replyError(req.ID, CodeInvalidParams, "empty prompt")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	// cancel previous if any
	if prev, ok := s.cancels[p.SessionID]; ok {
		prev()
	}
	s.cancels[p.SessionID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, p.SessionID)
		s.mu.Unlock()
		cancel()
	}()

	userMsg := provider.Message{Role: provider.RoleUser, Content: text}
	as.Messages = append(as.Messages, userMsg)

	stop := StopEndTurn
	onEvent := func(ev agent.Event) {
		s.emitAgentEvent(p.SessionID, ev)
	}

	if s.opt.Runner != nil {
		s.opt.Runner.Run(ctx, as.WorkDir, as.System, as.Messages, as.auth, s.opt.MaxIter, onEvent)
		if ctx.Err() != nil {
			stop = StopCancelled
		}
	} else {
		ch := agent.Run(ctx, agent.Config{
			Provider:      as.prov,
			Tools:         as.tools,
			System:        as.System,
			MaxTokens:     4096,
			MaxIterations: s.opt.MaxIter,
			Auth:          as.auth,
		}, as.Messages)
		var asst strings.Builder
		for ev := range ch {
			if ctx.Err() != nil {
				stop = StopCancelled
			}
			onEvent(ev)
			switch ev.Kind {
			case agent.EventText:
				asst.WriteString(ev.Text)
			case agent.EventDone:
				// ok
			case agent.EventError:
				if ctx.Err() != nil {
					stop = StopCancelled
				} else {
					// still end turn; surface error as message chunk
					s.notifyUpdate(p.SessionID, map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "\n⚠ " + ev.Error.Error()},
					})
				}
			}
		}
		if asst.Len() > 0 {
			as.Messages = append(as.Messages, provider.Message{
				Role: provider.RoleAssistant, Content: asst.String(),
			})
		}
	}

	// Persist conversation
	if as.cf != nil {
		as.cf.Messages = as.Messages
		as.cf.Provider = as.rt.ProvReg.CurrentName()
		as.cf.Model = as.prov.Model()
		_ = as.cf.Save()
	}

	s.reply(req.ID, SessionPromptResult{StopReason: stop})
}

func (s *Server) emitAgentEvent(sessionID string, ev agent.Event) {
	switch ev.Kind {
	case agent.EventThinking:
		if ev.Thinking == "" {
			return
		}
		s.notifyUpdate(sessionID, map[string]any{
			"sessionUpdate": "agent_thought_chunk",
			"content":       map[string]any{"type": "text", "text": ev.Thinking},
		})
	case agent.EventText:
		if ev.Text == "" {
			return
		}
		s.notifyUpdate(sessionID, map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": ev.Text},
		})
	case agent.EventToolCall:
		id := ev.ToolName + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1e9)
		s.mu.Lock()
		if s.lastToolCallID == nil {
			s.lastToolCallID = map[string]string{}
		}
		s.lastToolCallID[sessionID] = id
		s.mu.Unlock()
		s.notifyUpdate(sessionID, map[string]any{
			"sessionUpdate": "tool_call",
			"toolCallId":    id,
			"title":         ev.ToolName,
			"kind":          toolKind(ev.ToolName),
			"status":        "pending",
			"rawInput":      ev.ToolInput,
		})
	case agent.EventToolProgress:
		// optional — skip or map to tool_call_update
	case agent.EventToolResult:
		status := "completed"
		if !ev.ToolSuccess {
			status = "failed"
		}
		content := []map[string]any{}
		if ev.ToolOutput != "" {
			content = append(content, map[string]any{
				"type": "content",
				"content": map[string]any{
					"type": "text",
					"text": ev.ToolOutput,
				},
			})
		}
		s.mu.Lock()
		tid := s.lastToolCallID[sessionID]
		s.mu.Unlock()
		upd := map[string]any{
			"sessionUpdate": "tool_call_update",
			"title":         ev.ToolName,
			"status":        status,
			"content":       content,
		}
		if tid != "" {
			upd["toolCallId"] = tid
		}
		s.notifyUpdate(sessionID, upd)
	}
}

func toolKind(name string) string {
	switch name {
	case "read_file", "list_dir":
		return "read"
	case "write_file", "search_replace", "apply_patch":
		return "edit"
	case "run_command":
		return "execute"
	case "grep_search", "codebase_search":
		return "search"
	default:
		return "other"
	}
}

func (s *Server) cancelSession(id string) {
	s.mu.Lock()
	if c, ok := s.cancels[id]; ok {
		c()
	}
	s.mu.Unlock()
}

func (s *Server) notifyUpdate(sessionID string, update map[string]any) {
	s.notify("session/update", SessionUpdateParams{
		SessionID: sessionID,
		Update:    update,
	})
}

func (s *Server) notify(method string, params any) {
	if s.tx == nil {
		return
	}
	_ = s.tx.Write(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (s *Server) reply(id json.RawMessage, result any) {
	if s.tx == nil {
		return
	}
	_ = s.tx.Write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) replyError(id json.RawMessage, code int, msg string) {
	if s.tx == nil {
		return
	}
	_ = s.tx.Write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	})
}

func extractPromptText(blocks []ContentBlock) string {
	var b strings.Builder
	for _, c := range blocks {
		switch c.Type {
		case "text", "":
			if c.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(c.Text)
			}
		case "resource":
			if c.Resource != nil {
				if t, ok := c.Resource["text"].(string); ok && t != "" {
					uri, _ := c.Resource["uri"].(string)
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					if uri != "" {
						b.WriteString("### " + uri + "\n")
					}
					b.WriteString(t)
				}
			}
		}
	}
	return b.String()
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
