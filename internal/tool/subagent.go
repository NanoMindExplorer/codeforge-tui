package tool

import "strings"

// NewReadOnlyRegistry builds a registry without write/github mutate tools.
func NewReadOnlyRegistry(workdir string, parent *Registry) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	r.Register(&FileReader{WorkDir: workdir})
	r.Register(&DirLister{WorkDir: workdir})
	r.Register(&GrepSearch{WorkDir: workdir})
	r.Register(&CodebaseSearch{WorkDir: workdir})
	r.Register(&Diagnostics{WorkDir: workdir})
	r.Register(&URLFetch{})
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
