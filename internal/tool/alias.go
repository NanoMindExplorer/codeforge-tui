package tool

import "encoding/json"

// Alias exposes an existing tool under a Grok-compatible name.
type Alias struct {
	AliasName string
	Inner     Tool
	// DescriptionOverride optional; defaults to Inner.Description() with alias note.
	DescriptionOverride string
}

func (a *Alias) Name() string { return a.AliasName }

func (a *Alias) Description() string {
	if a.DescriptionOverride != "" {
		return a.DescriptionOverride
	}
	if a.Inner == nil {
		return a.AliasName
	}
	return a.Inner.Description() + " (Grok-compatible alias for " + a.Inner.Name() + ")"
}

func (a *Alias) Schema() map[string]any {
	if a.Inner == nil {
		return map[string]any{"type": "object"}
	}
	return a.Inner.Schema()
}

func (a *Alias) Execute(input json.RawMessage) Result {
	if a.Inner == nil {
		return Result{Error: "alias target missing"}
	}
	return a.Inner.Execute(input)
}

// ExecuteStream forwards if inner supports streaming.
func (a *Alias) ExecuteStream(input json.RawMessage, progress ProgressFunc) Result {
	if st, ok := a.Inner.(StreamingExecutor); ok {
		return st.ExecuteStream(input, progress)
	}
	return a.Execute(input)
}
