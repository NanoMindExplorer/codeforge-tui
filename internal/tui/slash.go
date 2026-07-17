package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/checkpoint"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/doctor"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/index"
	"github.com/codeforge/tui/internal/onboarding"
	"github.com/codeforge/tui/internal/pager"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/personas"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/sandbox"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/skills"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/todos"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/components"
)

func isImmediateSlash(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	switch cmd {
	case "/help", "/about", "/cost", "/budget", "/rules", "/index",
		"/status", "/clear", "/quit", "/theme", "/compact-mode", "/vim-mode",
		"/sessions", "/resume", "/new", "/fork", "/rewind", "/compact",
		"/context", "/session-info", "/mode", "/plan", "/view-plan",
		"/permissions", "/sandbox", "/pager", "/hooks", "/todos", "/tasks", "/settings", "/copy",
		"/memory", "/skills", "/personas", "/subagents", "/undo", "/push", "/pull", "/retry":
		return true
	default:
		return false
	}
}

func (m *Model) executeSlashCommand(input string) tea.Cmd {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	raw := input
	if strings.HasPrefix(raw, "/") {
		raw = raw[1:]
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil
	}
	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	argStr := strings.Join(args, " ")

	switch cmd {
	case "help", "h", "?":
		m.chat.AddSystemMessage(helpText())

	case "retry":
		// Q5.3: retry last user turn after provider error (Ctrl+R also works)
		return m.retryLastTurn()

	case "about":
		m.chat.AddSystemMessage(aboutText())

	case "doctor":
		ver := tuiAboutVersion()
		rep := doctor.Run(doctor.Options{
			Registry: m.providerReg,
			WorkDir:  m.workdir,
			Version:  ver,
		})
		m.chat.AddSystemMessage(rep.String())
		if !rep.OK {
			m.toast = components.NewToast(fmt.Sprintf("doctor: %d issue(s)", rep.Issues), "error", 3*time.Second)
		} else {
			m.toast = components.NewToast("doctor OK", "success", 2*time.Second)
		}

	case "provider", "p":
		if len(args) == 0 {
			m.chat.AddSystemMessage(onboarding.FormatKeySourcesWithActive(m.cfg, m.providerReg.CurrentName()))
		} else {
			name := strings.ToLower(args[0])
			if name == "xai" {
				name = "grok"
			}
			if err := m.providerReg.Switch(name); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error() + "\n  → /setup " + name + " <api-key>")
			} else {
				// Persist preference so multi-key bootstrap stays stable
				_ = onboarding.MarkCompleted(name, "")
				_ = config.SaveDefaultProvider(name)
				src, _ := onboarding.KeySource(name)
				model := ""
				if cur, err := m.providerReg.Current(); err == nil {
					model = cur.Model()
					if m.session != nil {
						m.session.Provider = name
						m.session.Model = model
					}
				}
				msg := fmt.Sprintf("✓ Active provider: %s\n  key: %s", name, src)
				if model != "" {
					msg += "\n  model: " + model
				}
				others := onboarding.PresentCloudKeys()
				if len(others) > 1 {
					msg += "\n  (other keys kept — switch anytime with /provider <name>)"
				}
				m.chat.AddSystemMessage(msg)
				m.toast = components.NewToast("Provider → "+name, "success", 2*time.Second)
			}
		}

	case "setup":
		// /setup | /setup status | /setup <provider> | /setup <provider> <key> [model]
		if len(args) == 0 || (len(args) == 1 && (args[0] == "status" || args[0] == "show")) {
			var sb strings.Builder
			sb.WriteString("Setup — multi-provider guide\n\n")
			sb.WriteString(onboarding.FormatKeySourcesWithActive(m.cfg, m.providerReg.CurrentName()))
			sb.WriteString("\nCommands:\n")
			sb.WriteString("  /setup <provider> <api-key> [model]  add/replace key + make active\n")
			sb.WriteString("  /setup grok xai-…\n")
			sb.WriteString("  /setup gemini AIza…\n")
			sb.WriteString("  /setup ollama\n")
			sb.WriteString("  /provider <name>                   switch without re-pasting\n")
			if onboarding.ProviderHealthy(m.providerReg) {
				sb.WriteString("\n✓ Current provider validates OK.")
			} else {
				sb.WriteString("\n⚠ No valid provider yet — paste a key with /setup.")
			}
			m.chat.AddSystemMessage(sb.String())
		} else if len(args) == 1 && strings.ToLower(args[0]) == "ollama" {
			if _, err := onboarding.ApplyKey(m.providerReg, "ollama", "", ""); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("✓ Ollama ready · active\n  other cloud keys (if any) still available via /provider")
				m.toast = components.NewToast("Ollama", "success", 2*time.Second)
			}
		} else if len(args) == 1 {
			name := strings.ToLower(args[0])
			meta := onboarding.MetaFor(name)
			env := onboarding.EnvNameForProvider(name)
			// If key already present, allow /setup grok to just activate
			if src, ok := onboarding.KeySource(name); ok && name != "ollama" {
				_ = onboarding.MarkCompleted(name, meta.DefaultModel)
				_ = config.SaveDefaultProvider(name)
				if err := m.providerReg.Switch(name); err != nil {
					m.chat.AddSystemMessage(fmt.Sprintf("Key exists (%s) but provider not registered.\n  /setup %s <%s>", src, name, env))
				} else {
					m.chat.AddSystemMessage(fmt.Sprintf("✓ Activated %s (existing key: %s)\n  To replace key: /setup %s <new-key>", name, src, name))
					m.toast = components.NewToast("Active → "+name, "success", 2*time.Second)
				}
				break
			}
			m.chat.AddSystemMessage(fmt.Sprintf(
				"Add %s\n  /setup %s <%s>\n  shape: %s\n  docs:  %s\n  or:    export %s=… && restart",
				meta.Title, name, env, meta.KeyHint, meta.DocsURL, env,
			))
		} else {
			name := strings.ToLower(args[0])
			key := args[1]
			model := ""
			if len(args) >= 3 {
				model = strings.Join(args[2:], " ")
			}
			if det := onboarding.DetectProviderFromKey(key); det != "" {
				if det != name {
					m.chat.AddSystemMessage(fmt.Sprintf("ℹ key looks like %s — using that provider", det))
				}
				name = det
			}
			p, err := onboarding.ApplyKey(m.providerReg, name, key, model)
			if err != nil {
				m.chat.AddSystemMessage(provider.FormatUserError(err))
			} else {
				src, _ := onboarding.KeySource(name)
				m.chat.AddSystemMessage(fmt.Sprintf(
					"✓ %s ready · model %s\n  key: %s (%s)\n  switch later: /provider gemini|grok|claude|openai",
					name, p.Model(), src, onboarding.MaskKey(key),
				))
				m.toast = components.NewToast("Setup OK · "+name, "success", 3*time.Second)
				if m.session != nil {
					m.session.Provider = name
					m.session.Model = p.Model()
				}
			}
		}

	case "model", "m":
		if len(args) == 0 {
			if cur, err := m.providerReg.Current(); err == nil {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("Model aktif: %s\n\nModel tersedia:\n", cur.Model()))
				for _, mi := range cur.Models() {
					mark := "  "
					if mi.ID == cur.Model() {
						mark = "* "
					}
					sb.WriteString(fmt.Sprintf("%s%s\n    %s  ctx:%d  $%.2f/$%.2f per 1M\n",
						mark, mi.ID, mi.Name, mi.ContextWindow, mi.InputCost, mi.OutputCost))
				}
				sb.WriteString("\nGanti: /model <id>")
				m.chat.AddSystemMessage(sb.String())
			}
		} else {
			// ROOT FIX: actually switch model via SetModel
			if cur, err := m.providerReg.Current(); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else if err := cur.SetModel(argStr); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("✓ Model: " + argStr)
				m.toast = components.NewToast("Model → "+argStr, "success", 2*time.Second)
				if m.session != nil {
					m.session.Model = argStr
				}
			}
		}

	case "mode":
		if len(args) == 0 {
			m.chat.AddSystemMessage(fmt.Sprintf(
				"Session mode: %s\n  %s\n\n  /mode build   — staged writes (default)\n  /mode design  — plan.md only (DESIGN)\n  /mode yolo    — always-approve writes\n  Shift+Tab     — cycle BUILD → DESIGN → YOLO\n  /plan         — enter DESIGN + optional task",
				m.sessionMode.Label(), m.sessionMode.Description(),
			))
		} else {
			if sm, ok := tool.ParseSessionMode(args[0]); ok {
				m.setSessionMode(sm)
			} else {
				// legacy aliases handled by ParseSessionMode; leftover:
				m.chat.AddSystemMessage("Use: /mode build | design | yolo  (aliases: plan→design, act→yolo)")
			}
		}

	case "plan":
		// Enter DESIGN; optional arg starts an agent plan turn
		m.setSessionMode(tool.SessionDesign)
		if argStr != "" {
			task := "Design a plan (DESIGN mode) for: " + argStr +
				"\n\nExplore the codebase with read/search tools. Write the plan via write_plan. " +
				"Then call exit_plan_mode. Do NOT edit project source files."
			return m.chat.SubmitAgent(task)
		}
		m.chat.AddSystemMessage("◈ DESIGN mode on. Describe the task and press Enter, or:\n  /plan <description>  — start planning turn\n  /view-plan            — open current plan")

	case "view-plan", "show-plan", "plan-view":
		m.openPlanReview("View plan")
		// viewing only — if user approves from here, still OK

	case "permissions", "perms":
		if m.perm == nil {
			m.chat.AddSystemMessage("Permission engine not available")
			return nil
		}
		if len(args) == 0 {
			m.chat.AddSystemMessage(m.perm.Summary() + "\n  /permissions mode <default|always_approve|dont_ask|plan>\n  /permissions allow <tool> [pattern]\n  /permissions deny <tool> [pattern]\n  /permissions clear-remember")
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "mode":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Current: " + string(m.perm.GetMode()))
				return nil
			}
			m.perm.SetMode(permission.ParseMode(args[1]))
			m.chat.AddSystemMessage("Permission mode → " + string(m.perm.GetMode()))
		case "allow":
			toolN := "*"
			pat := "*"
			if len(args) >= 2 {
				toolN = args[1]
			}
			if len(args) >= 3 {
				pat = strings.Join(args[2:], " ")
			}
			m.perm.AddRule(permission.Rule{Tool: toolN, Pattern: pat, Effect: permission.EffectAllow})
			m.chat.AddSystemMessage(fmt.Sprintf("Added allow rule: %s %q", toolN, pat))
		case "deny":
			toolN := "*"
			pat := "*"
			if len(args) >= 2 {
				toolN = args[1]
			}
			if len(args) >= 3 {
				pat = strings.Join(args[2:], " ")
			}
			m.perm.AddRule(permission.Rule{Tool: toolN, Pattern: pat, Effect: permission.EffectDeny})
			m.chat.AddSystemMessage(fmt.Sprintf("Added deny rule: %s %q", toolN, pat))
		case "ask":
			toolN := "run_command"
			pat := "*"
			if len(args) >= 2 {
				toolN = args[1]
			}
			if len(args) >= 3 {
				pat = strings.Join(args[2:], " ")
			}
			m.perm.AddRule(permission.Rule{Tool: toolN, Pattern: pat, Effect: permission.EffectAsk})
			m.chat.AddSystemMessage(fmt.Sprintf("Added ask rule: %s %q", toolN, pat))
		case "clear-remember", "clear":
			m.perm.ClearRemembered()
			m.chat.AddSystemMessage("Cleared remembered grants")
		default:
			m.chat.AddSystemMessage("Usage: /permissions [mode|allow|deny|ask|clear-remember]")
		}

	case "hooks":
		if m.hooks == nil {
			m.chat.AddSystemMessage("No hooks runner")
			return nil
		}
		m.chat.AddSystemMessage(m.hooks.Summary())

	case "pager":
		if len(args) > 0 && (strings.EqualFold(args[0], "reload") || strings.EqualFold(args[0], "refresh")) {
			c := pager.ApplyFromWorkdir(m.workdir)
			m.chat.AddSystemMessage("✓ Reloaded " + c.Summary())
			m.toast = components.NewToast("pager reloaded", "success", 2*time.Second)
			return nil
		}
		c := pager.Global()
		src := c.Source
		if src == "" {
			src = "(defaults)"
		}
		lay := theme.CurrentLayout()
		m.chat.AddSystemMessage(fmt.Sprintf(
			"Pager config: %s\n  layout: vpad=%d hpad=%d/%d block_pad=%d/%d\n  scrollbar=%v sticky=%v thinking=%v bullet=%q fps=%d\n  invert_scroll=%v scroll_speed=%.2fx max_thoughts_w=%d\n\n  /pager reload  — rescan ~/.codeforge/pager.toml · .grok/pager.toml\n  See docs/PAGER.md",
			src, lay.OuterVPad, lay.OuterHPadLeft, lay.OuterHPadRight, lay.BlockPadLeft, lay.BlockPadRight,
			c.ScrollbarEnabled(), c.StickyHeaders(), c.ShowThinking(), c.ToolBulletChar(), c.AnimationFPS(),
			c.InvertScroll(), c.ScrollSpeedMult(), c.MaxThoughtsWidth(),
		))

	case "sandbox", "sbx":
		eng := m.sandboxEng()
		if len(args) == 0 {
			bwrap := "no"
			if sandbox.HasBubblewrap() {
				bwrap = "yes"
			}
			m.chat.AddSystemMessage(fmt.Sprintf(
				"%s\nbubblewrap: %s\n\n  /sandbox off         — unrestricted\n  /sandbox workspace   — write CWD + ~/.codeforge + tmp (recommended)\n  /sandbox read-only   — no project writes; net blocked for shell\n  /sandbox strict      — read CWD+system only; net blocked\n  /sandbox devbox      — write almost everywhere except /data\n\nEnv: CODEFORGE_SANDBOX · flag: --sandbox · docs/SANDBOX.md",
				eng.Summary(), bwrap,
			))
			return nil
		}
		p, ok := sandbox.ParseProfile(args[0])
		if !ok {
			m.chat.AddSystemMessage("Unknown profile. Use: off | workspace | read-only | strict | devbox")
			return nil
		}
		// Preserve deny list from previous engine
		deny := append([]string(nil), eng.Deny...)
		ne := sandbox.Ensure(p, m.workdir)
		ne.Deny = deny
		sandbox.LogEvent("switch", map[string]any{"profile": string(p), "backend": string(ne.Backend)})
		m.chat.AddSystemMessage("✓ " + ne.Summary())
		m.toast = components.NewToast(ne.Summary(), "info", 3*time.Second)

	case "todos", "todo":
		if len(args) == 0 {
			m.chat.AddSystemMessage(m.todosList().Render())
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "add":
			text := strings.Join(args[1:], " ")
			if text == "" {
				m.chat.AddSystemMessage("Usage: /todos add <text>")
				return nil
			}
			it := m.todosList().Add(text)
			m.chat.AddSystemMessage(fmt.Sprintf("Added %s: %s", it.ID, it.Content))
		case "done", "complete":
			if len(args) < 2 || !m.todosList().SetStatus(args[1], todos.Completed) {
				m.chat.AddSystemMessage("Usage: /todos done <id>")
				return nil
			}
			m.chat.AddSystemMessage("Completed " + args[1])
		case "progress", "start":
			if len(args) < 2 || !m.todosList().SetStatus(args[1], todos.InProgress) {
				m.chat.AddSystemMessage("Usage: /todos progress <id>")
				return nil
			}
			m.chat.AddSystemMessage("In progress " + args[1])
		case "clear":
			m.todosList().Clear()
			m.chat.AddSystemMessage("Todos cleared")
		default:
			m.chat.AddSystemMessage("Usage: /todos [add|done|progress|clear]")
		}

	case "subagents", "subagent", "subs":
		if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			m.chat.AddSystemMessage(tool.SubJobs.Summary())
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "show", "view", "get", "output":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /subagents show <id>")
				return nil
			}
			j, ok := tool.SubJobs.Get(args[1])
			if !ok {
				m.chat.AddSystemMessage("Unknown subagent: " + args[1])
				return nil
			}
			m.chat.AddSystemMessage(tool.FormatJobOutput(j))
		case "cancel", "kill", "stop":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /subagents cancel <id>")
				return nil
			}
			if err := tool.SubJobs.Cancel(args[1]); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("Cancel signal sent to " + args[1])
				m.toast = components.NewToast("Subagent cancel "+args[1], "info", 2*time.Second)
			}
		default:
			// treat as id
			j, ok := tool.SubJobs.Get(args[0])
			if !ok {
				m.chat.AddSystemMessage("Usage: /subagents [list|show <id>|cancel <id>]")
				return nil
			}
			m.chat.AddSystemMessage(tool.FormatJobOutput(j))
		}

	case "personas", "persona":
		if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			m.chat.AddSystemMessage(m.personasReg().RenderList())
			return nil
		}
		if strings.EqualFold(args[0], "reload") {
			cfgP := map[string]personas.Persona{}
			var extra []string
			if m.cfg != nil {
				for name, sp := range m.cfg.Subagents.Personas {
					cfgP[name] = personas.Persona{
						Name: name, Description: sp.Description, Instructions: sp.Instructions,
						InstructionsFile: sp.InstructionsFile, Model: sp.Model, DefaultIsolation: sp.DefaultIsolation,
					}
				}
				extra = m.cfg.Subagents.ExtraDirs
			}
			reg := personas.Load(personas.Options{WorkDir: m.workdir, ConfigPersonas: cfgP, ExtraDirs: extra})
			m.chat.AddSystemMessage("✓ Reloaded " + reg.Summary())
			return nil
		}
		if p, ok := m.personasReg().Get(args[0]); ok {
			body := p.Resolved
			if body == "" {
				body = p.Instructions
			}
			if len(body) > 2500 {
				body = body[:2500] + "\n…"
			}
			m.chat.AddSystemMessage(fmt.Sprintf("Persona %s\n%s\n\n%s\n\nUse: spawn_subagent persona=%s",
				p.Name, p.Description, body, p.Name))
			return nil
		}
		m.chat.AddSystemMessage("Unknown persona: " + args[0])

	case "skills", "skill":
		// /skills [list|reload|<name>]
		if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			m.chat.AddSystemMessage(m.skillsReg().RenderList())
			return nil
		}
		if strings.EqualFold(args[0], "reload") || strings.EqualFold(args[0], "refresh") {
			opt := skills.Options{WorkDir: m.workdir, CompatClaude: true, CompatCursor: true}
			if m.cfg != nil {
				opt.ExtraPaths = m.cfg.Skills.Paths
				opt.Ignore = m.cfg.Skills.Ignore
				opt.Disabled = m.cfg.Skills.Disabled
				opt.CompatClaude = m.cfg.SkillsCompatClaude()
				opt.CompatCursor = m.cfg.SkillsCompatCursor()
			}
			reg := skills.Load(opt)
			m.slash.RefreshSkills()
			m.chat.AddSystemMessage("✓ Reloaded " + reg.Summary())
			m.toast = components.NewToast(reg.Summary(), "info", 2*time.Second)
			return nil
		}
		// show one skill
		name := args[0]
		if sk, ok := m.skillsReg().Get(name); ok {
			body := sk.Body
			if len(body) > 3000 {
				body = body[:3000] + "\n… (truncated)"
			}
			m.chat.AddSystemMessage(fmt.Sprintf("Skill /%s [%s]\n%s\n\n%s\n\nRun: /%s [args]",
				sk.Name, sk.Source, sk.Description, body, sk.Name))
			return nil
		}
		m.chat.AddSystemMessage("Unknown skill: " + name + "\nUse /skills to list.")

	case "memory", "mem":
		// /memory [list|add <text>|search <query>]
		if len(args) == 0 {
			m.chat.AddSystemMessage("Memory store (~/.codeforge/memory/):\n  /memory list          — recent notes\n  /memory add <text>    — store a note\n  /memory search <q>    — keyword search\n\nAgents use memory_search / memory_write tools.")
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "list", "ls", "show":
			notes, err := tool.ListMemoryRecent(15)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			if len(notes) == 0 {
				m.chat.AddSystemMessage("(no memory notes yet — try /memory add …)")
				return nil
			}
			var b strings.Builder
			b.WriteString("Recent memory notes:\n")
			for i, n := range notes {
				fmt.Fprintf(&b, "  %d. %s\n", i+1, n)
			}
			m.chat.AddSystemMessage(b.String())
		case "add", "write", "save":
			text := strings.Join(args[1:], " ")
			if text == "" {
				m.chat.AddSystemMessage("Usage: /memory add <text>")
				return nil
			}
			if err := tool.AppendMemory(text, ""); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage("✓ Memory note stored")
			m.toast = components.NewToast("Memory saved", "success", 2*time.Second)
		case "search", "find", "s":
			q := strings.Join(args[1:], " ")
			if q == "" {
				m.chat.AddSystemMessage("Usage: /memory search <query>")
				return nil
			}
			ms := &tool.MemorySearch{}
			res := ms.Execute([]byte(fmt.Sprintf(`{"query":%q,"limit":10}`, q)))
			if res.Error != "" {
				m.chat.AddSystemMessage("⚠ " + res.Error)
				return nil
			}
			m.chat.AddSystemMessage(res.Output)
		default:
			// bare text after /memory → treat as add
			if err := tool.AppendMemory(argStr, ""); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage("✓ Memory note stored (implicit add)")
		}

	case "tasks", "bg":
		if len(args) == 0 {
			m.chat.AddSystemMessage(m.bgTasks().Summary())
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "cancel", "kill", "stop":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /tasks cancel <id>")
				return nil
			}
			if err := m.bgTasks().Cancel(args[1]); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("Cancelled " + args[1])
				m.toast = components.NewToast("Task cancelled", "info", 2*time.Second)
			}
		case "show", "view", "log":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /tasks show <id>")
				return nil
			}
			t, ok := m.bgTasks().Get(args[1])
			if !ok {
				m.chat.AddSystemMessage("Task not found")
				return nil
			}
			m.chat.AddSystemMessage(fmt.Sprintf("[%s] %s\n%s\n\n%s", t.Status, t.ID, t.Command, t.Output))
		default:
			m.chat.AddSystemMessage(m.bgTasks().Summary())
		}

	case "settings", "config":
		m.openSettings()

	case "copy":
		// /copy [meta]
		meta := len(args) > 0 && (args[0] == "meta" || args[0] == "Y")
		m.copySelectedBlock(meta)

	case "sessions", "dashboard":
		// list text view with preview (Q4.2); picker is /resume
		if len(args) == 0 {
			list, err := session.ListForWorkdir(m.workdir, 15)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage(session.FormatResumeList(list, 15))
		} else {
			s, err := session.Load(args[0])
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.applySession(s)
		}

	case "resume":
		// /resume          → interactive picker
		// /resume last     → newest for this cwd (Q4.2)
		// /resume <id>     → load by id
		// /resume list     → text table with previews
		if len(args) == 0 {
			m.openSessionPicker()
			return nil
		}
		if strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			list, err := session.ListForWorkdir(m.workdir, 15)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage(session.FormatResumeList(list, 15))
			return nil
		}
		if strings.EqualFold(args[0], "last") || strings.EqualFold(args[0], "latest") {
			s, err := session.LastForWorkdir(m.workdir)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			if s == nil {
				m.chat.AddSystemMessage("No sessions for this project yet. Chat first, then /resume last.")
				return nil
			}
			m.applySession(s)
			return nil
		}
		s, err := session.Load(args[0])
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.applySession(s)

	case "new":
		m.startNewSession()

	case "fork":
		if m.session == nil {
			m.chat.AddSystemMessage("No session to fork")
			return nil
		}
		m.session.Messages = m.chat.messages
		m.session.TotalCost = m.totalCost
		m.session.Tokens = m.totalTokens
		child, err := m.session.Fork()
		if err != nil {
			m.chat.AddSystemMessage("⚠ fork: " + err.Error())
			return nil
		}
		// optional directive as first user note
		if argStr != "" {
			child.Messages = append(child.Messages, provider.Message{
				Role: provider.RoleUser, Content: argStr,
			})
			_ = child.Save()
		}
		m.applySession(child)
		m.chat.AddSystemMessage("Forked → " + child.ID + " (parent " + child.ParentID + ")")

	case "rewind":
		if len(args) == 0 {
			m.openRewindPicker()
			return nil
		}
		// /rewind last — last point
		pts, err := m.session.LoadRewindPoints()
		if err != nil || len(pts) == 0 {
			pts = synthesizeRewindPoints(m.chat.messages)
		}
		if len(pts) == 0 {
			m.chat.AddSystemMessage("No rewind points")
			return nil
		}
		if strings.EqualFold(args[0], "last") {
			m.applyRewind(pts[len(pts)-1])
			return nil
		}
		// by index 1-based from newest
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			// newest first numbering
			idx := len(pts) - n
			if idx >= 0 && idx < len(pts) {
				m.applyRewind(pts[idx])
				return nil
			}
		}
		m.chat.AddSystemMessage("Usage: /rewind | /rewind last | /rewind <n>")

	case "compact":
		// conversation compact (not compact-mode UI)
		if m.session == nil {
			m.chat.AddSystemMessage("No session")
			return nil
		}
		m.session.Messages = m.chat.messages
		res, err := m.session.Compact(6, argStr)
		if err != nil {
			m.chat.AddSystemMessage("⚠ compact: " + err.Error())
			return nil
		}
		m.chat.LoadMessages(m.session.Messages)
		m.chat.AddSystemMessage(fmt.Sprintf("✓ Compacted %d → %d messages\n%s", res.BeforeMsgs, res.AfterMsgs, truncateStr(res.Summary, 200)))
		m.toast = components.NewToast("Compacted", "success", 2*time.Second)

	case "context", "ctx":
		maxCtx := m.maxContextTokens()
		tok := m.totalTokens
		if tok == 0 {
			tok = session.EstimateTokens(m.chat.messages)
		}
		pct := 0.0
		if maxCtx > 0 {
			pct = float64(tok) * 100 / float64(maxCtx)
		}
		users, assts, tools := 0, 0, 0
		for _, msg := range m.chat.messages {
			switch msg.Role {
			case provider.RoleUser:
				users++
			case provider.RoleAssistant:
				assts++
			case provider.RoleTool:
				tools++
			}
		}
		m.chat.AddSystemMessage(fmt.Sprintf(
			`Context window
  Tokens   : %d / %d  (%.1f%%)
  Messages : %d total (%d user · %d assistant · %d tool)
  Cost     : $%.4f
  Session  : %s

  /compact to compress history · auto-compact at ~85%%`,
			tok, maxCtx, pct, len(m.chat.messages), users, assts, tools,
			m.totalCost, func() string {
				if m.session != nil {
					return m.session.ID
				}
				return "?"
			}(),
		))

	case "session-info", "sessioninfo", "si":
		if m.session == nil {
			m.chat.AddSystemMessage("No session")
			return nil
		}
		m.session.Messages = m.chat.messages
		m.session.Tokens = m.totalTokens
		m.session.TotalCost = m.totalCost
		m.chat.AddSystemMessage(m.session.InfoText(m.maxContextTokens()))

	case "rename", "title":
		if m.session == nil || argStr == "" {
			m.chat.AddSystemMessage("Usage: /rename <title>")
			return nil
		}
		m.session.Title = argStr
		m.session.Slug = sessionSlugify(argStr)
		_ = m.session.Save()
		m.chat.AddSystemMessage("Title → " + argStr)

	case "undo":
		if m.session == nil {
			m.chat.AddSystemMessage("No session for undo")
			return nil
		}
		rel, err := checkpoint.UndoLast(m.session.ID)
		if err != nil {
			m.chat.AddSystemMessage("⚠ undo: " + err.Error())
		} else {
			m.chat.AddSystemMessage("✓ Restored: " + rel)
			m.toast = components.NewToast("Undid "+rel, "success", 3*time.Second)
			m.context.MarkTouched(rel)
		}

	case "cost", "c":
		dur := time.Since(m.startTime).Round(time.Second)
		m.chat.AddSystemMessage(fmt.Sprintf(
			"Session Summary\n  Provider : %s\n  Tokens   : %d\n  Cost     : $%.4f\n  Duration : %s\n  Mode     : %s",
			m.providerReg.CurrentName(), m.totalTokens, m.totalCost, dur, m.status.AgentMode,
		))

	case "status", "s":
		if m.gitRepo != nil {
			status, err := m.gitRepo.Status()
			if err != nil {
				m.chat.AddSystemMessage("Git: " + err.Error())
			} else {
				branch, _ := m.gitRepo.Branch()
				m.chat.AddSystemMessage("Branch: " + branch + "\n\n" + status)
				// refresh context with git glyphs
				gs := parseGitStatus(status)
				nc, _ := m.context.Update(ContextUpdateMsg{Refresh: true, GitStatus: gs})
				m.context = nc.(ContextModel)
			}
		} else {
			m.chat.AddSystemMessage("Not a git repository")
		}

	case "commit":
		if m.gitRepo == nil {
			m.chat.AddSystemMessage("No git repository")
			return nil
		}
		if err := m.gitRepo.AddAll(); err != nil {
			m.chat.AddSystemMessage("⚠ git add: " + err.Error())
			return nil
		}
		cmsg := argStr
		if cmsg == "" {
			cmsg = git.GenerateCommitMessage("feat", "", "AI-assisted changes via CodeForge TUI")
		}
		hash, err := m.gitRepo.Commit(cmsg)
		if err != nil {
			m.chat.AddSystemMessage("⚠ commit: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(fmt.Sprintf("✓ Committed: %s\n  %s", hash, cmsg))
		m.toast = components.NewToast("Committed "+hash, "success", 3*time.Second)

	// ── GitHub integration (gh + API) ─────────────────────
	case "gh", "github":
		return m.handleGitHubCommand(args)

	case "pr":
		// /pr [list|view|create|merge|checks] ...
		if len(args) == 0 {
			return m.handleGitHubCommand([]string{"pr", "list"})
		}
		return m.handleGitHubCommand(append([]string{"pr"}, args...))

	case "issue":
		if len(args) == 0 {
			return m.handleGitHubCommand([]string{"issue", "list"})
		}
		return m.handleGitHubCommand(append([]string{"issue"}, args...))

	case "push":
		return m.handleGitHubCommand([]string{"push"})

	case "pull":
		return m.handleGitHubCommand([]string{"pull"})

	case "act", "a":
		if argStr == "" {
			m.chat.AddSystemMessage("Examples:\n  /act explain auth flow\n  /act fix failing tests")
			return nil
		}
		if m.budgetBlocks() {
			m.chat.AddSystemMessage("⛔ Budget exceeded — raise budget.max_cost_usd or /clear cost via new session")
			return nil
		}
		return m.chat.SubmitAgent(argStr)

	case "rules":
		rb := rules.Get()
		if rb == nil || rb.Text == "" {
			m.chat.AddSystemMessage("No project rules. Create AGENTS.md or .codeforge/rules.md in the project root.")
			return nil
		}
		m.chat.AddSystemMessage(rb.Summary() + "\n\n" + rb.Text)
		return nil

	case "index":
		idx := index.Global()
		if idx == nil {
			m.chat.AddSystemMessage("Index not ready — building…")
			return func() tea.Msg {
				built, err := index.Build(m.workdir)
				if err != nil {
					return errMsg{err: err}
				}
				index.SetGlobal(built)
				f, s := built.Stats()
				return ToastMsg{Text: fmt.Sprintf("Index ready: %d files, %d symbols", f, s), Kind: "success"}
			}
		}
		f, s := idx.Stats()
		m.chat.AddSystemMessage(fmt.Sprintf("Codebase index: %d files, %d symbols\nUse agent tool codebase_search or /act where is X?", f, s))
		return nil

	case "budget":
		if m.cfg == nil {
			m.chat.AddSystemMessage("No config")
			return nil
		}
		m.chat.AddSystemMessage(fmt.Sprintf(
			"Session cost: $%.4f\nBudget max: $%.2f (0=unlimited)\nWarn at: $%.2f\nTokens: %d\n\nSet in config:\n  budget:\n    max_cost_usd: 1.0\n    warn_at_usd: 0.5",
			m.totalCost, m.cfg.Budget.MaxCostUSD, m.cfg.Budget.WarnAtUSD, m.totalTokens,
		))
		return nil

	case "theme", "t":
		if theme.MinimalMode() {
			m.chat.AddSystemMessage("Themes unavailable in --minimal mode (terminal-native 16 colors)")
			return nil
		}
		if argStr == "" {
			// Open live-preview picker (Grok parity); bare cycle via /theme cycle
			m.openThemePicker()
			return nil
		}
		if strings.EqualFold(argStr, "cycle") || strings.EqualFold(argStr, "next") {
			name := theme.Cycle()
			m.onThemeApplied()
			m.chat.AddSystemMessage("Theme → " + name)
			m.toast = components.NewToast("Theme: "+name, "info", 2*time.Second)
			return nil
		}
		if theme.SetByName(argStr) {
			m.onThemeApplied()
			m.chat.AddSystemMessage("Theme → " + theme.Name() + "  · color " + theme.ColorLevelName(theme.DetectColorLevel()))
			m.toast = components.NewToast("Theme: "+theme.Name(), "success", 2*time.Second)
			if theme.IsAuto() {
				return autoThemeTick()
			}
		} else {
			m.chat.AddSystemMessage("Unknown theme. Use: " + strings.Join(theme.ThemeNames(), ", ") + "\n  /theme          open picker\n  /theme cycle    next theme")
		}
		return nil

	case "compact-mode":
		on := theme.ToggleCompact()
		m.recalcSizes()
		state := "off"
		if on {
			state = "on"
		}
		m.chat.AddSystemMessage("Compact mode " + state)
		m.toast = components.NewToast("Compact "+state, "info", 2*time.Second)
		return nil

	case "vim-mode", "vim":
		m.vimMode = !m.vimMode
		state := "off"
		if m.vimMode {
			state = "on"
		}
		m.chat.AddSystemMessage("Vim scrollback keys " + state + " (j/k/h/l/g/G/e/E)")
		m.toast = components.NewToast("Vim mode "+state, "info", 2*time.Second)
		return nil

	case "read", "r":
		if argStr == "" {
			m.chat.AddSystemMessage("Example: /read main.go")
			return nil
		}
		return m.chat.SubmitAgent("Read file " + argStr + " and show its contents")

	case "ls", "list":
		dir := "."
		if argStr != "" {
			dir = argStr
		}
		return m.chat.SubmitAgent("List the contents of directory: " + dir)

	case "grep", "find":
		if argStr == "" {
			m.chat.AddSystemMessage("Example: /grep func main")
			return nil
		}
		return m.chat.SubmitAgent("Search the project for this pattern: " + argStr)

	case "run":
		if argStr == "" {
			m.chat.AddSystemMessage("Example: /run go build ./...")
			return nil
		}
		return m.chat.SubmitAgent("Run this command and show the output: " + argStr)

	case "explain", "e":
		if argStr == "" {
			m.chat.AddSystemMessage("Example: /explain main.go")
			return nil
		}
		return m.chat.SubmitAgent("Read and explain in detail the code at: " + argStr)

	case "fix":
		if argStr == "" {
			m.chat.AddSystemMessage("Example: /fix main.go")
			return nil
		}
		return m.chat.SubmitAgent("Read file " + argStr + ", find bugs or errors, then fix them")

	case "clear":
		// Grok: /clear clears chat in-place; /new rotates session id
		m.chat.Clear()
		m.diff = NewDiffModel()
		if m.session != nil {
			m.session.Messages = nil
			m.session.Preview = ""
			_ = m.session.Save()
		}

	case "quit", "q", "exit":
		m.saveSession()
		m.quitting = true
		return tea.Quit

	default:
		// Phase G5: skill slash invoke (/name or /skill:name)
		skillName := cmd
		if strings.HasPrefix(cmd, "skill:") {
			skillName = strings.TrimPrefix(cmd, "skill:")
		}
		if sk, ok := m.skillsReg().Get(skillName); ok {
			if sk.Disabled {
				m.chat.AddSystemMessage("Skill disabled: " + skillName)
				return nil
			}
			if !sk.UserInvocable {
				m.chat.AddSystemMessage("Skill not user-invocable: " + skillName)
				return nil
			}
			m.toast = components.NewToast("Skill /"+sk.Name, "info", 2*time.Second)
			return m.chat.SubmitAgent(sk.InvokePrompt(argStr))
		}
		return m.chat.SubmitAgent(input)
	}
	return nil
}

// handleGitHubCommand runs GitHub ops from slash commands.
// Forms:
//
//	/gh auth|repo|push|pull|log|branch <name>
//	/gh pr list|view [n]|create <title> [| body]|merge <n>|checks [n]
//	/gh issue list|view <n>|create <title> [| body]
//	/pr … and /issue … are aliases without the leading "pr"/"issue" word duplicated.

func modeString(m Mode) string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeCommand:
		return "COMMAND"
	case ModePalette:
		return "COMMAND"
	case ModeReview:
		return "REVIEW"
	case ModeFilePick:
		return "INSERT"
	case ModeAskUser:
		return "ASK"
	case ModePermAsk:
		return "PERM"
	}
	return "?"
}

var slashCommands = []string{
	"/act", "/read", "/ls", "/grep", "/run", "/explain", "/fix",
	"/status", "/commit", "/push", "/pull", "/pr", "/issue", "/gh",
	"/provider", "/setup", "/doctor", "/model", "/mode", "/cost", "/budget", "/rules", "/index",
	"/theme", "/compact-mode", "/vim-mode",
	"/resume", "/new", "/rename", "/fork", "/rewind", "/compact", "/context", "/session-info",
	"/mode", "/plan", "/view-plan", "/permissions", "/sandbox", "/pager", "/hooks",
	"/todos", "/tasks", "/memory", "/skills", "/personas", "/subagents", "/settings", "/copy",
	"/sessions", "/undo", "/retry", "/clear", "/help", "/about", "/quit",
}

func autocomplete(input string) string {
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, input) {
			return cmd + " "
		}
	}
	return ""
}

func helpText() string {
	return `CodeForge · Grok 4.5 parity  ·  Phases 1–9 + G1–G7

SCROLLBACK
  Enter · y/Y · j/k · fold · G follow-tail

MODES
  Shift+Tab      BUILD → DESIGN → YOLO

PRODUCT
  /setup /provider /doctor /model · multi-key: see docs/ONBOARDING.md
  /resume /new /fork /rewind /compact /context
  /plan /todos /tasks /subagents /memory /skills /personas /settings
  /theme /permissions /sandbox /hooks /vim-mode /compact-mode
  /retry or Ctrl+R   re-send last turn after a provider error

AGENT / IDE
  spawn_subagent background=true · get_subagent_output · resume_from
  /subagents list|show|cancel  ·  /skills  ·  /personas  ·  /sandbox
  See docs/SUBAGENTS.md · docs/SKILLS.md · docs/SANDBOX.md
`
}

func aboutText() string {
	return `CodeForge TUI v1.9.3
Created by NanoMind — 2026 — Apache 2.0

Grok Build TUI–compatible (Phases 1–9 + G1–G10 + W1–W4):
  blocks · input · themes · sessions · design plan
  permissions/hooks · todos/tasks · ACP + x.ai/* extensions
  Grok 4.5 · native thinking · Landlock · skills · personas
  pager.toml · /setup · /doctor · release gate
See docs/PAGER.md · docs/REASONING.md · docs/RELEASE_GATE.md
`
}

// tuiAboutVersion extracts X.Y.Z from aboutText first line.
func tuiAboutVersion() string {
	lines := strings.Split(aboutText(), "\n")
	if len(lines) == 0 {
		return ""
	}
	for _, w := range strings.Fields(lines[0]) {
		if strings.HasPrefix(w, "v") && strings.Count(w, ".") >= 2 {
			return strings.TrimPrefix(w, "v")
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
