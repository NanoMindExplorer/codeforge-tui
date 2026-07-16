package theme

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// themeFile is the YAML shape of ~/.codeforge/theme.yaml
type themeFile struct {
	BgBase          string `yaml:"bg_base"`
	BgSurface       string `yaml:"bg_surface"`
	BgElevated      string `yaml:"bg_elevated"`
	BgOverlay       string `yaml:"bg_overlay"`
	BgLight         string `yaml:"bg_light"`
	BorderDim       string `yaml:"border_dim"`
	BorderActive    string `yaml:"border_active"`
	BorderGlow      string `yaml:"border_glow"`
	PromptBorder    string `yaml:"prompt_border"`
	SelectionBorder string `yaml:"selection_border"`
	TextPrimary     string `yaml:"text_primary"`
	TextSecondary   string `yaml:"text_secondary"`
	TextMuted       string `yaml:"text_muted"`
	TextDisabled    string `yaml:"text_disabled"`
	AccentAI        string `yaml:"accent_ai"`
	AccentAgent     string `yaml:"accent_agent"`
	AccentUser      string `yaml:"accent_user"`
	AccentFocus     string `yaml:"accent_focus"`
	AccentThinking  string `yaml:"accent_thinking"`
	AccentRunning   string `yaml:"accent_running"`
	AccentPlan      string `yaml:"accent_plan"`
	AccentSystem    string `yaml:"accent_system"`
	AccentAssistant string `yaml:"accent_assistant"`
	AccentTool      string `yaml:"accent_tool"`
	Success         string `yaml:"success"`
	Danger          string `yaml:"danger"`
	Warning         string `yaml:"warning"`
	Info            string `yaml:"info"`
	DiffAddBg       string `yaml:"diff_add_bg"`
	DiffAddFg       string `yaml:"diff_add_fg"`
	DiffDelBg       string `yaml:"diff_del_bg"`
	DiffDelFg       string `yaml:"diff_del_fg"`
	DiffCtxFg       string `yaml:"diff_ctx_fg"`
	ScrollbarBg     string `yaml:"scrollbar_bg"`
	ScrollbarFg     string `yaml:"scrollbar_fg"`
	MdHeading       string `yaml:"md_heading"`
	MdLink          string `yaml:"md_link"`
	MdCode          string `yaml:"md_code"`
	MdCodeBg        string `yaml:"md_code_bg"`
}

// LoadFromFile loads theme overrides from path (default: ~/.codeforge/theme.yaml).
func LoadFromFile(path string) (*Tokens, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".codeforge", "theme.yaml")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f themeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	t := GrokNight()
	override := func(dst *lipgloss.Color, src string) {
		if src != "" {
			*dst = lipgloss.Color(src)
		}
	}
	override(&t.BgBase, f.BgBase)
	override(&t.BgSurface, f.BgSurface)
	override(&t.BgElevated, f.BgElevated)
	override(&t.BgOverlay, f.BgOverlay)
	override(&t.BgLight, f.BgLight)
	override(&t.BorderDim, f.BorderDim)
	override(&t.BorderActive, f.BorderActive)
	override(&t.BorderGlow, f.BorderGlow)
	override(&t.PromptBorder, f.PromptBorder)
	override(&t.SelectionBorder, f.SelectionBorder)
	override(&t.TextPrimary, f.TextPrimary)
	override(&t.TextSecondary, f.TextSecondary)
	override(&t.TextMuted, f.TextMuted)
	override(&t.TextDisabled, f.TextDisabled)
	override(&t.AccentAI, f.AccentAI)
	override(&t.AccentAgent, f.AccentAgent)
	override(&t.AccentUser, f.AccentUser)
	override(&t.AccentFocus, f.AccentFocus)
	override(&t.AccentThinking, f.AccentThinking)
	override(&t.AccentRunning, f.AccentRunning)
	override(&t.AccentPlan, f.AccentPlan)
	override(&t.AccentSystem, f.AccentSystem)
	override(&t.AccentAssistant, f.AccentAssistant)
	override(&t.AccentTool, f.AccentTool)
	// Keep aliases in sync when only one side set
	if f.AccentAssistant != "" {
		t.AccentAI = t.AccentAssistant
	} else if f.AccentAI != "" {
		t.AccentAssistant = t.AccentAI
	}
	if f.AccentTool != "" {
		t.AccentAgent = t.AccentTool
	} else if f.AccentAgent != "" {
		t.AccentTool = t.AccentAgent
	}
	override(&t.Success, f.Success)
	override(&t.Danger, f.Danger)
	override(&t.Warning, f.Warning)
	override(&t.Info, f.Info)
	override(&t.DiffAddBg, f.DiffAddBg)
	override(&t.DiffAddFg, f.DiffAddFg)
	override(&t.DiffDelBg, f.DiffDelBg)
	override(&t.DiffDelFg, f.DiffDelFg)
	override(&t.DiffCtxFg, f.DiffCtxFg)
	override(&t.ScrollbarBg, f.ScrollbarBg)
	override(&t.ScrollbarFg, f.ScrollbarFg)
	override(&t.MdHeading, f.MdHeading)
	override(&t.MdLink, f.MdLink)
	override(&t.MdCode, f.MdCode)
	override(&t.MdCodeBg, f.MdCodeBg)
	t.Name = "custom"
	return &t, nil
}
