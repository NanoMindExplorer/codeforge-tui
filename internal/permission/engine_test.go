package permission

import (
	"context"
	"testing"
)

func TestDenyRmRf(t *testing.T) {
	e := NewEngine(t.TempDir())
	res := e.Evaluate("run_command", `{"command":"rm -rf /tmp/foo"}`)
	if res.Decision != DecisionDeny {
		t.Fatalf("got %v %s", res.Decision, res.Reason)
	}
}

func TestReadOnlyAutoAllow(t *testing.T) {
	e := NewEngine(t.TempDir())
	res := e.Evaluate("read_file", `{"path":"main.go"}`)
	if res.Decision != DecisionAllow {
		t.Fatal(res)
	}
	res = e.Evaluate("run_command", `{"command":"git status"}`)
	if res.Decision != DecisionAllow {
		t.Fatalf("git status: %v %s", res.Decision, res.Reason)
	}
	if IsReadOnlyShell("ls && rm -rf x") {
		t.Fatal("expected not read-only")
	}
}

func TestAlwaysApproveStillDenies(t *testing.T) {
	e := NewEngine(t.TempDir())
	e.SetMode(ModeAlwaysApprove)
	res := e.Evaluate("run_command", `{"command":"rm -rf /"}`)
	if res.Decision != DecisionDeny {
		t.Fatalf("deny must win: %v", res)
	}
}

func TestDontAskDeniesWrites(t *testing.T) {
	e := NewEngine(t.TempDir())
	e.SetMode(ModeDontAsk)
	res := e.Evaluate("write_file", `{"path":"a.go","content":"x"}`)
	if res.Decision != DecisionDeny {
		t.Fatal(res)
	}
	res = e.Evaluate("read_file", `{"path":"a.go"}`)
	if res.Decision != DecisionAllow {
		t.Fatal(res)
	}
}

func TestAllowRule(t *testing.T) {
	e := NewEngine(t.TempDir())
	e.SetMode(ModeDontAsk)
	e.AddRule(Rule{Tool: "run_command", Pattern: "go test *", Effect: EffectAllow})
	res := e.Evaluate("run_command", `{"command":"go test ./..."}`)
	if res.Decision != DecisionAllow {
		t.Fatal(res)
	}
}

func TestRemember(t *testing.T) {
	dir := t.TempDir()
	e := NewEngine(dir)
	key := rememberKey("run_command", "go test")
	e.Remember(key, true)
	e2 := NewEngine(dir)
	e2.LoadRemembered()
	e2.Mode = ModeDefault
	e2.Rules = nil
	res := e2.Evaluate("run_command", `{"command":"go test"}`)
	if res.Decision != DecisionAllow {
		t.Fatalf("expected remembered allow: %+v remembered=%v", res, e2.Remembered)
	}
}

func TestDangerous(t *testing.T) {
	if !IsDangerous("sudo rm -rf /") {
		t.Fatal("expected dangerous")
	}
	if IsDangerous("git status") {
		t.Fatal("not dangerous")
	}
}

func TestAuthorizeWithAsk(t *testing.T) {
	e := NewEngine(t.TempDir())
	e.Mode = ModeDefault
	e.Rules = nil
	called := false
	e.Ask = func(ctx context.Context, tool, input, reason string, dangerous bool) (bool, bool, error) {
		called = true
		return true, false, nil
	}
	// shell asks in default mode
	if err := e.Authorize(context.Background(), "run_command", `{"command":"make build"}`); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("ask not called")
	}
}

func TestAuthorizeHeadlessDeniesAsk(t *testing.T) {
	e := NewEngine(t.TempDir())
	e.Mode = ModeDefault
	e.Rules = nil
	e.Headless = true
	err := e.Authorize(context.Background(), "run_command", `{"command":"make build"}`)
	if err == nil {
		t.Fatal("expected deny")
	}
}

func TestDefaultAllowsEdits(t *testing.T) {
	e := NewEngine(t.TempDir())
	e.Rules = nil
	res := e.Evaluate("write_file", `{"path":"x","content":"y"}`)
	if res.Decision != DecisionAllow {
		t.Fatal(res)
	}
}

func TestPatternMatch(t *testing.T) {
	r := Rule{Tool: "run_command", Pattern: "rm *", Effect: EffectDeny}
	if !r.Match("run_command", `{"command":"rm -rf foo"}`) {
		t.Fatal("should match")
	}
}
