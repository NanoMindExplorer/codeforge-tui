package tool

import "strings"

// NewReadOnlyRegistry builds a registry without write/github mutate tools.
func NewReadOnlyRegistry(workdir string, parent *Registry) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	reader := &FileReader{WorkDir: workdir}
	lister := &DirLister{WorkDir: workdir}
	grep := &GrepSearch{WorkDir: workdir}
	glob := &GlobSearch{WorkDir: workdir}
	fetch := &URLFetch{}
	r.Register(reader)
	r.Register(lister)
	r.Register(grep)
	r.Register(&CodebaseSearch{WorkDir: workdir})
	r.Register(glob)
	r.Register(&Diagnostics{WorkDir: workdir})
	r.Register(fetch)
	r.Register(&WebSearch{})
	r.Register(&MemorySearch{})
	// Grok-compatible aliases (explore subagents)
	r.Register(&Alias{AliasName: "grep", Inner: grep})
	r.Register(&Alias{AliasName: "list_directory", Inner: lister})
	r.Register(&Alias{AliasName: "web_fetch", Inner: fetch})
	r.Register(&Alias{AliasName: "glob", Inner: glob})
	r.Register(&Alias{AliasName: "find_files", Inner: glob})
	if parent != nil {
		for _, t := range parent.List() {
			name := t.Name()
			if strings.HasPrefix(name, "mcp_") {
				r.Register(t)
			}
		}
	}
	return r
}
