package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/provider"
)

// MCPBridgeTool exposes one MCP server tool through the CodeForge registry.
type MCPBridgeTool struct {
	Client *provider.MCPClient
	Def    provider.MCPToolDef
}

func (m *MCPBridgeTool) Name() string {
	// prefix to avoid collisions
	return "mcp_" + sanitizeName(m.Def.Server) + "_" + sanitizeName(m.Def.Name)
}

func (m *MCPBridgeTool) Description() string {
	desc := m.Def.Description
	if desc == "" {
		desc = "MCP tool " + m.Def.Name
	}
	return fmt.Sprintf("[MCP:%s] %s", m.Def.Server, desc)
}

func (m *MCPBridgeTool) Schema() map[string]any {
	if sch, ok := m.Def.InputSchema.(map[string]any); ok && sch != nil {
		return sch
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (m *MCPBridgeTool) Execute(input json.RawMessage) Result {
	if m.Client == nil {
		return Result{Error: "MCP client nil"}
	}
	var args map[string]any
	if len(input) > 0 {
		_ = json.Unmarshal(input, &args)
	}
	if args == nil {
		args = map[string]any{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := m.Client.CallTool(ctx, m.Def.Name, args)
	if err != nil {
		return Result{Success: false, Error: err.Error(), Output: out}
	}
	return Result{Success: true, Output: out}
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "tool"
	}
	return out
}

// RegisterMCPServers connects configured MCP servers and registers their tools.
// Returns human-readable status lines.
func RegisterMCPServers(reg *Registry, servers []provider.MCPServerConfig) []string {
	var status []string
	for _, cfg := range servers {
		if cfg.Command == "" {
			continue
		}
		if cfg.Name == "" {
			cfg.Name = "mcp"
		}
		client, err := provider.ConnectMCP(cfg)
		if err != nil {
			status = append(status, fmt.Sprintf("MCP %s: failed — %v", cfg.Name, err))
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		tools, err := client.ListTools(ctx)
		cancel()
		if err != nil {
			status = append(status, fmt.Sprintf("MCP %s: list tools failed — %v", cfg.Name, err))
			_ = client.Close()
			continue
		}
		for _, t := range tools {
			t.Server = cfg.Name
			reg.Register(&MCPBridgeTool{Client: client, Def: t})
		}
		status = append(status, fmt.Sprintf("MCP %s: %d tool(s)", cfg.Name, len(tools)))
	}
	return status
}
