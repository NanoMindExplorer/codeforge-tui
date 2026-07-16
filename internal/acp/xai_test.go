package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memTX struct {
	msgs []any
}

func (m *memTX) Write(msg any) error {
	m.msgs = append(m.msgs, msg)
	return nil
}

func TestXAIExtensionsListed(t *testing.T) {
	ext := XAIExtensions()
	if len(ext) < 20 {
		t.Fatal(len(ext))
	}
	found := false
	for _, e := range ext {
		if e == "x.ai/fs/read_file" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing fs/read_file")
	}
}

func TestXAIInitializeHasExtensions(t *testing.T) {
	tx := &memTX{}
	srv := NewServer(Options{Version: "test", WorkDir: t.TempDir(), AlwaysApprove: true})
	srv.SetTransport(tx)
	srv.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`))
	if len(tx.msgs) == 0 {
		t.Fatal("no reply")
	}
	b, _ := json.Marshal(tx.msgs[0])
	if !strings.Contains(string(b), "xaiExtensions") && !strings.Contains(string(b), "x.ai/fs") {
		t.Fatal(string(b))
	}
}

func TestXAIFSRoundtrip(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	tx := &memTX{}
	srv := NewServer(Options{Version: "test", WorkDir: dir, AlwaysApprove: true})
	srv.SetTransport(tx)

	// list
	srv.Handle([]byte(`{"jsonrpc":"2.0","id":2,"method":"x.ai/fs/list","params":{"path":"."}}`))
	// exists
	srv.Handle([]byte(`{"jsonrpc":"2.0","id":3,"method":"x.ai/fs/exists","params":{"path":"a.txt"}}`))
	// read
	srv.Handle([]byte(`{"jsonrpc":"2.0","id":4,"method":"x.ai/fs/read_file","params":{"path":"a.txt"}}`))

	foundHello := false
	for _, m := range tx.msgs {
		b, _ := json.Marshal(m)
		if strings.Contains(string(b), "hello") {
			foundHello = true
		}
	}
	if !foundHello {
		t.Fatal(tx.msgs)
	}
}

func TestXAISubagentList(t *testing.T) {
	tx := &memTX{}
	srv := NewServer(Options{Version: "test", WorkDir: t.TempDir()})
	srv.SetTransport(tx)
	srv.Handle([]byte(`{"jsonrpc":"2.0","id":5,"method":"x.ai/subagent/list","params":{}}`))
	if len(tx.msgs) == 0 {
		t.Fatal("no reply")
	}
}
