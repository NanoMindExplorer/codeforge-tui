package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// StdioTransport reads newline-delimited JSON-RPC from in and writes to out.
type StdioTransport struct {
	in  *bufio.Reader
	out io.Writer
	mu  sync.Mutex
}

// NewStdioTransport uses the given reader/writer (typically os.Stdin/Stdout).
func NewStdioTransport(in io.Reader, out io.Writer) *StdioTransport {
	return &StdioTransport{
		in:  bufio.NewReaderSize(in, 1024*1024),
		out: out,
	}
}

// Write encodes one JSON message followed by newline.
func (t *StdioTransport) Write(msg any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = t.out.Write(append(data, '\n'))
	return err
}

// Serve runs the read loop until EOF or error.
func ServeStdio(srv *Server, in io.Reader, out io.Writer) error {
	tx := NewStdioTransport(in, out)
	srv.SetTransport(tx)
	// Log to stderr only — stdout is protocol
	fmt.Fprintln(os.Stderr, "codeforge agent stdio — ACP JSON-RPC ready")
	for {
		line, err := tx.in.ReadBytes('\n')
		if len(line) > 0 {
			// trim newline
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				srv.Handle(line)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
