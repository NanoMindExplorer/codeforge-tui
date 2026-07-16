package tool

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io/fs"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "runtime"
    "strings"
    "time"

    "github.com/codeforge/tui/internal/diff"
)

type Result struct {
    Success bool   `json:"success"`
    Output  string `json:"output"`
    Error   string `json:"error,omitempty"`

    // Diff is populated by write_file with a unified diff of the change.
    // It is for UI display only and is never sent back to the model.
    Diff string `json:"-"`
}

type Tool interface {
    Name() string
    Description() string
    // Schema returns the tool's input as a JSON Schema object (map with
    // "type", "properties", "required", etc). Used to build provider tool
    // definitions for function/tool calling.
    Schema() map[string]any
    Execute(input json.RawMessage) Result
}

// resolvePath joins path against workdir (if relative) and rejects any
// result that escapes workdir, as a lightweight sandbox against the AI
// reading or writing files outside the project.
func resolvePath(workdir, path string) (string, error) {
    full := path
    if !filepath.IsAbs(full) {
        full = filepath.Join(workdir, full)
    }
    full = filepath.Clean(full)

    rel, err := filepath.Rel(workdir, full)
    if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
        return "", fmt.Errorf("path %q is outside the project directory", path)
    }
    return full, nil
}

// ---------------------------------------------------------------------
// read_file
// ---------------------------------------------------------------------

type FileReader struct{ WorkDir string }

func (f *FileReader) Name() string        { return "read_file" }
func (f *FileReader) Description() string { return "Read the contents of a file in the project" }

func (f *FileReader) Schema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "path": map[string]any{
                "type":        "string",
                "description": "File path, relative to the project root",
            },
        },
        "required": []string{"path"},
    }
}

type readInput struct {
    Path string `json:"path"`
}

func (f *FileReader) Execute(input json.RawMessage) Result {
    var in readInput
    if err := json.Unmarshal(input, &in); err != nil {
        return Result{Error: fmt.Sprintf("invalid: %v", err)}
    }
    if in.Path == "" {
        return Result{Error: "path required"}
    }
    path, err := resolvePath(f.WorkDir, in.Path)
    if err != nil {
        return Result{Error: err.Error()}
    }
    data, err := os.ReadFile(path)
    if err != nil {
        return Result{Error: fmt.Sprintf("read: %v", err)}
    }
    return Result{Success: true, Output: string(data)}
}

// ---------------------------------------------------------------------
// write_file
// ---------------------------------------------------------------------

type FileWriter struct{ WorkDir string }

func (f *FileWriter) Name() string        { return "write_file" }
func (f *FileWriter) Description() string { return "Create or overwrite a file in the project with new content" }

func (f *FileWriter) Schema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "path": map[string]any{
                "type":        "string",
                "description": "File path, relative to the project root",
            },
            "content": map[string]any{
                "type":        "string",
                "description": "The full new content of the file",
            },
        },
        "required": []string{"path", "content"},
    }
}

type writeInput struct {
    Path    string `json:"path"`
    Content string `json:"content"`
}

func (f *FileWriter) Execute(input json.RawMessage) Result {
    var in writeInput
    if err := json.Unmarshal(input, &in); err != nil {
        return Result{Error: fmt.Sprintf("invalid: %v", err)}
    }
    if in.Path == "" {
        return Result{Error: "path required"}
    }
    path, err := resolvePath(f.WorkDir, in.Path)
    if err != nil {
        return Result{Error: err.Error()}
    }

    oldContent := ""
    if data, err := os.ReadFile(path); err == nil {
        oldContent = string(data)
    }

    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        return Result{Error: fmt.Sprintf("mkdir: %v", err)}
    }
    if err := os.WriteFile(path, []byte(in.Content), 0644); err != nil {
        return Result{Error: fmt.Sprintf("write: %v", err)}
    }

    rel, _ := filepath.Rel(f.WorkDir, path)
    d := diff.Unified(rel, oldContent, in.Content)

    return Result{
        Success: true,
        Output:  fmt.Sprintf("Wrote %d bytes to %s", len(in.Content), rel),
        Diff:    d,
    }
}

// ---------------------------------------------------------------------
// list_dir
// ---------------------------------------------------------------------

type DirLister struct{ WorkDir string }

func (d *DirLister) Name() string        { return "list_dir" }
func (d *DirLister) Description() string { return "List the contents of a directory in the project" }

func (d *DirLister) Schema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "path": map[string]any{
                "type":        "string",
                "description": "Directory path, relative to the project root (default: project root)",
            },
        },
    }
}

type listInput struct {
    Path string `json:"path"`
}

func (d *DirLister) Execute(input json.RawMessage) Result {
    var in listInput
    if err := json.Unmarshal(input, &in); err != nil {
        return Result{Error: fmt.Sprintf("invalid: %v", err)}
    }
    path := in.Path
    if path == "" {
        path = "."
    }
    resolved, err := resolvePath(d.WorkDir, path)
    if err != nil {
        return Result{Error: err.Error()}
    }
    entries, err := os.ReadDir(resolved)
    if err != nil {
        return Result{Error: fmt.Sprintf("readdir: %v", err)}
    }
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("Listing: %s\n\n", path))
    for _, e := range entries {
        typ := "DIR "
        if !e.IsDir() {
            typ = "FILE"
        }
        sb.WriteString(fmt.Sprintf("[%s]  %s\n", typ, e.Name()))
    }
    return Result{Success: true, Output: sb.String()}
}

// ---------------------------------------------------------------------
// grep_search
// ---------------------------------------------------------------------

type GrepSearch struct{ WorkDir string }

func (g *GrepSearch) Name() string { return "grep_search" }
func (g *GrepSearch) Description() string {
    return "Search project files for a regex pattern. Returns matching file:line: text entries"
}

func (g *GrepSearch) Schema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "pattern": map[string]any{
                "type":        "string",
                "description": "Regular expression to search for",
            },
            "path": map[string]any{
                "type":        "string",
                "description": "Directory to search in, relative to project root (default: project root)",
            },
            "glob": map[string]any{
                "type":        "string",
                "description": "Optional filename glob filter, e.g. *.go",
            },
        },
        "required": []string{"pattern"},
    }
}

type grepInput struct {
    Pattern string `json:"pattern"`
    Path    string `json:"path"`
    Glob    string `json:"glob"`
}

const grepMaxMatches = 200

func (g *GrepSearch) Execute(input json.RawMessage) Result {
    var in grepInput
    if err := json.Unmarshal(input, &in); err != nil {
        return Result{Error: fmt.Sprintf("invalid: %v", err)}
    }
    if in.Pattern == "" {
        return Result{Error: "pattern required"}
    }
    re, err := regexp.Compile(in.Pattern)
    if err != nil {
        return Result{Error: fmt.Sprintf("invalid regex: %v", err)}
    }

    root := in.Path
    if root == "" {
        root = "."
    }
    resolvedRoot, err := resolvePath(g.WorkDir, root)
    if err != nil {
        return Result{Error: err.Error()}
    }

    var sb strings.Builder
    matches := 0
    walkErr := filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return nil // skip unreadable entries
        }
        if matches >= grepMaxMatches {
            return fs.SkipAll
        }
        if d.IsDir() {
            switch d.Name() {
            case ".git", "node_modules", "vendor", "dist", "build":
                return filepath.SkipDir
            }
            return nil
        }
        if in.Glob != "" {
            if ok, _ := filepath.Match(in.Glob, d.Name()); !ok {
                return nil
            }
        }
        data, err := os.ReadFile(path)
        if err != nil || looksBinary(data) {
            return nil
        }
        rel, _ := filepath.Rel(g.WorkDir, path)
        for i, line := range strings.Split(string(data), "\n") {
            if matches >= grepMaxMatches {
                break
            }
            if re.MatchString(line) {
                sb.WriteString(fmt.Sprintf("%s:%d: %s\n", rel, i+1, strings.TrimSpace(line)))
                matches++
            }
        }
        return nil
    })
    if walkErr != nil {
        return Result{Error: fmt.Sprintf("walk: %v", walkErr)}
    }
    if matches == 0 {
        return Result{Success: true, Output: "No matches found."}
    }
    if matches >= grepMaxMatches {
        sb.WriteString(fmt.Sprintf("... (truncated at %d matches)\n", grepMaxMatches))
    }
    return Result{Success: true, Output: sb.String()}
}

func looksBinary(data []byte) bool {
    n := len(data)
    if n > 512 {
        n = 512
    }
    for i := 0; i < n; i++ {
        if data[i] == 0 {
            return true
        }
    }
    return false
}

// ---------------------------------------------------------------------
// run_command
// ---------------------------------------------------------------------

type ShellExec struct{ WorkDir string }

func (s *ShellExec) Name() string { return "run_command" }
func (s *ShellExec) Description() string {
    return "Execute a shell command in the project directory (30s timeout). Use for running tests, builds, or git commands"
}

func (s *ShellExec) Schema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "command": map[string]any{
                "type":        "string",
                "description": "The shell command to execute",
            },
        },
        "required": []string{"command"},
    }
}

type shellInput struct {
    Command string `json:"command"`
}

const (
    shellTimeout   = 30 * time.Second
    shellMaxOutput = 8000
)

func (s *ShellExec) Execute(input json.RawMessage) Result {
    var in shellInput
    if err := json.Unmarshal(input, &in); err != nil {
        return Result{Error: fmt.Sprintf("invalid: %v", err)}
    }
    if strings.TrimSpace(in.Command) == "" {
        return Result{Error: "command required"}
    }

    ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
    defer cancel()

    shell, flag := "/bin/sh", "-c"
    if runtime.GOOS == "windows" {
        shell, flag = "cmd", "/C"
    }

    cmd := exec.CommandContext(ctx, shell, flag, in.Command)
    cmd.Dir = s.WorkDir
    var outBuf bytes.Buffer
    cmd.Stdout = &outBuf
    cmd.Stderr = &outBuf
    runErr := cmd.Run()

    output := outBuf.String()
    if len(output) > shellMaxOutput {
        output = output[:shellMaxOutput] + "\n... (output truncated)"
    }

    if ctx.Err() == context.DeadlineExceeded {
        return Result{Success: false, Output: output, Error: fmt.Sprintf("command timed out after %s", shellTimeout)}
    }
    if runErr != nil {
        return Result{Success: false, Output: output, Error: fmt.Sprintf("exit error: %v", runErr)}
    }
    return Result{Success: true, Output: output}
}

// ---------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------

type Registry struct {
    tools map[string]Tool
    order []string
}

func NewRegistry(workDir string) *Registry {
    r := &Registry{tools: make(map[string]Tool)}
    r.Register(&FileReader{WorkDir: workDir})
    // StagedWriter gates writes in Plan mode (default); Act mode writes immediately.
    r.Register(NewStagedWriter(workDir))
    r.Register(&DirLister{WorkDir: workDir})
    r.Register(&GrepSearch{WorkDir: workDir})
    r.Register(&ShellExec{WorkDir: workDir})
    return r
}

func (r *Registry) Register(t Tool) {
    if _, exists := r.tools[t.Name()]; !exists {
        r.order = append(r.order, t.Name())
    }
    r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
    t, ok := r.tools[name]
    return t, ok
}

// List returns tools in a stable, deterministic order (registration order),
// rather than Go's randomized map iteration order.
func (r *Registry) List() []Tool {
    tools := make([]Tool, 0, len(r.order))
    for _, name := range r.order {
        tools = append(tools, r.tools[name])
    }
    return tools
}
