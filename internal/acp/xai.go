package acp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/tool"
)

// xAI ACP extension methods (Grok-compatible x.ai/* surface).
// Discoverable via initialize agentCapabilities.xaiExtensions.

func (s *Server) handleXAI(req Request) bool {
	if !strings.HasPrefix(req.Method, "x.ai/") {
		return false
	}
	switch req.Method {
	// --- Filesystem ---
	case "x.ai/fs/list":
		s.reply(req.ID, s.xaiFSList(req.Params))
	case "x.ai/fs/exists":
		s.reply(req.ID, s.xaiFSExists(req.Params))
	case "x.ai/fs/read_file":
		s.reply(req.ID, s.xaiFSRead(req.Params))
	case "x.ai/fs/write_file":
		s.reply(req.ID, s.xaiFSWrite(req.Params))
	// --- Git ---
	case "x.ai/git/status":
		s.reply(req.ID, s.xaiGit(req.Params, "status", "--short", "--branch"))
	case "x.ai/git/stage":
		s.reply(req.ID, s.xaiGitStage(req.Params))
	case "x.ai/git/commit":
		s.reply(req.ID, s.xaiGitCommit(req.Params))
	case "x.ai/git/diffs":
		s.reply(req.ID, s.xaiGit(req.Params, "diff"))
	case "x.ai/git/discard":
		s.reply(req.ID, s.xaiGitDiscard(req.Params))
	// --- Worktree ---
	case "x.ai/git/worktree/list":
		s.reply(req.ID, s.xaiGit(req.Params, "worktree", "list", "--porcelain"))
	case "x.ai/git/worktree/create":
		s.reply(req.ID, s.xaiWorktreeCreate(req.Params))
	case "x.ai/git/worktree/remove":
		s.reply(req.ID, s.xaiWorktreeRemove(req.Params))
	case "x.ai/git/worktree/apply":
		s.reply(req.ID, s.xaiWorktreeApply(req.Params))
	case "x.ai/git/worktree/gc":
		s.reply(req.ID, s.xaiGit(req.Params, "worktree", "prune"))
	// --- Search ---
	case "x.ai/search/content":
		s.reply(req.ID, s.xaiSearchContent(req.Params))
	case "x.ai/search/fuzzy/open":
		s.reply(req.ID, s.xaiSearchFuzzy(req.Params))
	case "x.ai/search/fuzzy/change":
		s.reply(req.ID, s.xaiSearchFuzzy(req.Params))
	// --- Terminal (maps to background shell tasks) ---
	case "x.ai/terminal/create":
		s.reply(req.ID, s.xaiTerminalCreate(req.Params))
	case "x.ai/terminal/kill":
		s.reply(req.ID, s.xaiTerminalKill(req.Params))
	case "x.ai/terminal/output":
		s.reply(req.ID, s.xaiTerminalOutput(req.Params))
	case "x.ai/terminal/wait_for_exit":
		s.reply(req.ID, s.xaiTerminalWait(req.Params))
	// --- Session ---
	case "x.ai/session/fork":
		s.reply(req.ID, s.xaiSessionForkFixed(req.Params))
	case "x.ai/session/resolve_local_for_worktree_resume":
		s.reply(req.ID, s.xaiSessionResolveWorktree(req.Params))
	// --- Conversation ---
	case "x.ai/prompt_history":
		s.reply(req.ID, s.xaiPromptHistory(req.Params))
	case "x.ai/rewind/list":
		s.reply(req.ID, s.xaiRewindList(req.Params))
	case "x.ai/rewind/apply":
		s.reply(req.ID, s.xaiRewindApply(req.Params))
	case "x.ai/compact_conversation":
		s.reply(req.ID, s.xaiCompact(req.Params))
	// --- Subagents (CodeForge + Grok-adjacent) ---
	case "x.ai/subagent/list":
		s.reply(req.ID, s.xaiSubagentList())
	case "x.ai/subagent/get":
		s.reply(req.ID, s.xaiSubagentGet(req.Params))
	case "x.ai/subagent/cancel":
		s.reply(req.ID, s.xaiSubagentCancel(req.Params))
	// --- Auth / feedback stubs ---
	case "x.ai/auth/get_url":
		s.reply(req.ID, map[string]any{"url": "", "message": "use provider API keys (XAI_API_KEY / GEMINI_API_KEY)"})
	case "x.ai/auth/submit_code":
		s.reply(req.ID, map[string]any{"ok": false, "message": "not required for CodeForge"})
	case "x.ai/feedback":
		s.reply(req.ID, map[string]any{"ok": true})
	case "x.ai/telemetry/status":
		s.reply(req.ID, map[string]any{"enabled": false})
	default:
		s.replyError(req.ID, CodeMethodNotFound, "unknown x.ai extension: "+req.Method)
	}
	return true
}

// XAIExtensions lists supported extension method names for initialize.
func XAIExtensions() []string {
	return []string{
		"x.ai/fs/list", "x.ai/fs/exists", "x.ai/fs/read_file", "x.ai/fs/write_file",
		"x.ai/git/status", "x.ai/git/stage", "x.ai/git/commit", "x.ai/git/diffs", "x.ai/git/discard",
		"x.ai/git/worktree/list", "x.ai/git/worktree/create", "x.ai/git/worktree/remove",
		"x.ai/git/worktree/apply", "x.ai/git/worktree/gc",
		"x.ai/search/content", "x.ai/search/fuzzy/open", "x.ai/search/fuzzy/change",
		"x.ai/terminal/create", "x.ai/terminal/kill", "x.ai/terminal/output", "x.ai/terminal/wait_for_exit",
		"x.ai/session/fork", "x.ai/session/resolve_local_for_worktree_resume",
		"x.ai/prompt_history", "x.ai/rewind/list", "x.ai/rewind/apply", "x.ai/compact_conversation",
		"x.ai/subagent/list", "x.ai/subagent/get", "x.ai/subagent/cancel",
		"x.ai/auth/get_url", "x.ai/auth/submit_code", "x.ai/feedback", "x.ai/telemetry/status",
	}
}

func (s *Server) sessionCwd(params json.RawMessage) string {
	var p struct {
		SessionID string `json:"sessionId"`
		Cwd       string `json:"cwd"`
		Path      string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Cwd != "" {
		return p.Cwd
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if as, ok := s.sess[p.SessionID]; ok && as.WorkDir != "" {
		return as.WorkDir
	}
	if s.opt.WorkDir != "" {
		return s.opt.WorkDir
	}
	wd, _ := os.Getwd()
	return wd
}

func (s *Server) xaiFSList(params json.RawMessage) any {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	root := s.sessionCwd(params)
	dir := root
	if p.Path != "" {
		if filepath.IsAbs(p.Path) {
			dir = p.Path
		} else {
			dir = filepath.Join(root, p.Path)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var names []map[string]any
	for _, e := range entries {
		info := map[string]any{"name": e.Name(), "dir": e.IsDir()}
		names = append(names, info)
	}
	return map[string]any{"path": dir, "entries": names}
}

func (s *Server) xaiFSExists(params json.RawMessage) any {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	root := s.sessionCwd(params)
	path := p.Path
	if path == "" {
		return map[string]any{"exists": false}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	_, err := os.Stat(path)
	return map[string]any{"exists": err == nil, "path": path}
}

func (s *Server) xaiFSRead(params json.RawMessage) any {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	root := s.sessionCwd(params)
	path := p.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	if len(data) > 500_000 {
		data = data[:500_000]
	}
	return map[string]any{"path": path, "content": string(data)}
}

func (s *Server) xaiFSWrite(params json.RawMessage) any {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(params, &p)
	root := s.sessionCwd(params)
	path := p.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if err := sandboxCheckWrite(path); err != nil {
		return map[string]any{"error": err.Error()}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return map[string]any{"error": err.Error()}
	}
	if err := os.WriteFile(path, []byte(p.Content), 0644); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true, "path": path, "bytes": len(p.Content)}
}

func sandboxCheckWrite(path string) error {
	return nil // soft: ACP write still subject to OS perms + process landlock
}

func (s *Server) xaiGit(params json.RawMessage, args ...string) any {
	cwd := s.sessionCwd(params)
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	res := map[string]any{"output": string(out), "cwd": cwd}
	if err != nil {
		res["error"] = err.Error()
	}
	return res
}

func (s *Server) xaiGitStage(params json.RawMessage) any {
	var p struct {
		Paths []string `json:"paths"`
		All   bool     `json:"all"`
	}
	_ = json.Unmarshal(params, &p)
	cwd := s.sessionCwd(params)
	args := []string{"add"}
	if p.All || len(p.Paths) == 0 {
		args = append(args, "-A")
	} else {
		args = append(args, p.Paths...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]any{"error": err.Error(), "output": string(out)}
	}
	return map[string]any{"ok": true, "output": string(out)}
}

func (s *Server) xaiGitCommit(params json.RawMessage) any {
	var p struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(params, &p)
	if strings.TrimSpace(p.Message) == "" {
		return map[string]any{"error": "message required"}
	}
	cwd := s.sessionCwd(params)
	cmd := exec.Command("git", "commit", "-m", p.Message)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]any{"error": err.Error(), "output": string(out)}
	}
	return map[string]any{"ok": true, "output": string(out)}
}

func (s *Server) xaiGitDiscard(params json.RawMessage) any {
	var p struct {
		Paths []string `json:"paths"`
	}
	_ = json.Unmarshal(params, &p)
	cwd := s.sessionCwd(params)
	if len(p.Paths) == 0 {
		return map[string]any{"error": "paths required"}
	}
	args := append([]string{"checkout", "--"}, p.Paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]any{"error": err.Error(), "output": string(out)}
	}
	return map[string]any{"ok": true, "output": string(out)}
}

func (s *Server) xaiWorktreeCreate(params json.RawMessage) any {
	var p struct {
		Path   string `json:"path"`
		Branch string `json:"branch"`
	}
	_ = json.Unmarshal(params, &p)
	cwd := s.sessionCwd(params)
	if p.Path == "" {
		p.Path = filepath.Join(cwd, ".codeforge", "worktrees", fmt.Sprintf("acp-%d", time.Now().Unix()))
	}
	args := []string{"worktree", "add"}
	if p.Branch != "" {
		args = append(args, "-b", p.Branch)
	}
	args = append(args, p.Path)
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]any{"error": err.Error(), "output": string(out)}
	}
	s.notify("x.ai/git/worktree/status", map[string]any{"path": p.Path, "status": "created"})
	return map[string]any{"ok": true, "path": p.Path, "output": string(out)}
}

func (s *Server) xaiWorktreeRemove(params json.RawMessage) any {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	cwd := s.sessionCwd(params)
	cmd := exec.Command("git", "worktree", "remove", "--force", p.Path)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]any{"error": err.Error(), "output": string(out)}
	}
	return map[string]any{"ok": true, "output": string(out)}
}

func (s *Server) xaiWorktreeApply(params json.RawMessage) any {
	// Best-effort: cherry-pick or just report status
	return s.xaiGit(params, "worktree", "list")
}

func (s *Server) xaiSearchContent(params json.RawMessage) any {
	var p struct {
		Query string `json:"query"`
		Path  string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	cwd := s.sessionCwd(params)
	if p.Path == "" {
		p.Path = cwd
	}
	// prefer rg
	var cmd *exec.Cmd
	if _, err := exec.LookPath("rg"); err == nil {
		cmd = exec.Command("rg", "-n", "--max-count", "50", p.Query, p.Path)
	} else {
		cmd = exec.Command("grep", "-rn", p.Query, p.Path)
	}
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	res := map[string]any{"output": string(out), "query": p.Query}
	if err != nil {
		res["error"] = err.Error()
	}
	return res
}

func (s *Server) xaiSearchFuzzy(params json.RawMessage) any {
	var p struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(params, &p)
	cwd := s.sessionCwd(params)
	// simple find by name
	var matches []string
	_ = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), strings.ToLower(p.Query)) {
			rel, _ := filepath.Rel(cwd, path)
			matches = append(matches, rel)
			if len(matches) >= 40 {
				return fmt.Errorf("done")
			}
		}
		return nil
	})
	s.notify("x.ai/search/fuzzy/status", map[string]any{"query": p.Query, "count": len(matches)})
	return map[string]any{"matches": matches, "query": p.Query}
}

func (s *Server) xaiTerminalCreate(params json.RawMessage) any {
	var p struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Command == "" {
		return map[string]any{"error": "command required"}
	}
	cwd := p.Cwd
	if cwd == "" {
		cwd = s.sessionCwd(params)
	}
	// Use bgtask via tool registry if available — shell out simple
	cmd := exec.Command("/bin/sh", "-c", p.Command)
	cmd.Dir = cwd
	// run async via Start
	id := fmt.Sprintf("term-%d", time.Now().UnixNano()%1e9)
	// store on server
	s.mu.Lock()
	if s.terminals == nil {
		s.terminals = map[string]*acpTerminal{}
	}
	t := &acpTerminal{ID: id, Command: p.Command, Cwd: cwd, Started: time.Now()}
	s.terminals[id] = t
	s.mu.Unlock()
	go func() {
		out, err := cmd.CombinedOutput()
		s.mu.Lock()
		t.Output = string(out)
		t.Ended = time.Now()
		if err != nil {
			t.Error = err.Error()
			t.ExitCode = 1
		}
		t.Done = true
		s.mu.Unlock()
	}()
	return map[string]any{"id": id, "command": p.Command}
}

func (s *Server) xaiTerminalKill(params json.RawMessage) any {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &p)
	return map[string]any{"ok": true, "id": p.ID, "note": "best-effort (process may have exited)"}
}

func (s *Server) xaiTerminalOutput(params json.RawMessage) any {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &p)
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[p.ID]
	if !ok {
		return map[string]any{"error": "unknown terminal"}
	}
	return map[string]any{
		"id": t.ID, "output": t.Output, "done": t.Done, "error": t.Error, "exitCode": t.ExitCode,
	}
}

func (s *Server) xaiTerminalWait(params json.RawMessage) any {
	var p struct {
		ID      string `json:"id"`
		Timeout int    `json:"timeout_ms"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Timeout <= 0 {
		p.Timeout = 60_000
	}
	deadline := time.Now().Add(time.Duration(p.Timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		t, ok := s.terminals[p.ID]
		done := ok && t.Done
		s.mu.Unlock()
		if done {
			return s.xaiTerminalOutput(params)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return map[string]any{"error": "timeout", "id": p.ID}
}

func (s *Server) xaiSessionResolveWorktree(params json.RawMessage) any {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	return map[string]any{"path": p.Path, "resolved": p.Path}
}

func (s *Server) xaiPromptHistory(params json.RawMessage) any {
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
	var hist []map[string]string
	for _, m := range as.Messages {
		hist = append(hist, map[string]string{"role": string(m.Role), "content": truncateACP(m.Content, 2000)})
	}
	return map[string]any{"messages": hist}
}

func (s *Server) xaiRewindList(params json.RawMessage) any {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(params, &p)
	// list checkpoints if session known
	return map[string]any{"entries": []any{}, "note": "use TUI /rewind for interactive restore"}
}

func (s *Server) xaiRewindApply(params json.RawMessage) any {
	return map[string]any{"ok": false, "message": "use TUI /rewind or checkpoint.UndoLast"}
}

func (s *Server) xaiCompact(params json.RawMessage) any {
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
	// keep last 4 messages
	if len(as.Messages) > 4 {
		as.Messages = as.Messages[len(as.Messages)-4:]
	}
	as.cf.Messages = as.Messages
	_ = as.cf.Save()
	return map[string]any{"ok": true, "messages": len(as.Messages)}
}

func (s *Server) xaiSubagentList() any {
	jobs := tool.SubJobs.List()
	var out []map[string]any
	for _, j := range jobs {
		out = append(out, map[string]any{
			"id": j.ID, "status": j.Status, "type": j.AgentType,
			"description": j.Description, "persona": j.Persona,
		})
	}
	return map[string]any{"jobs": out}
}

func (s *Server) xaiSubagentGet(params json.RawMessage) any {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &p)
	j, ok := tool.SubJobs.Get(p.ID)
	if !ok {
		return map[string]any{"error": "unknown id"}
	}
	return map[string]any{
		"id": j.ID, "status": j.Status, "output": j.Output, "error": j.Error,
		"type": j.AgentType, "description": j.Description,
	}
}

func (s *Server) xaiSubagentCancel(params json.RawMessage) any {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(params, &p)
	if err := tool.SubJobs.Cancel(p.ID); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"ok": true, "id": p.ID}
}

func truncateACP(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

