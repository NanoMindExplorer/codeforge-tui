package tool

import "github.com/codeforge/tui/internal/bgtask"

func ensureBackgroundWire() {
	if bgStart != nil {
		return
	}
	SetBackgroundStarter(func(workdir, command string) (string, error) {
		t, err := bgtask.Global.Start(workdir, command)
		if err != nil {
			return "", err
		}
		return t.ID, nil
	})
}
