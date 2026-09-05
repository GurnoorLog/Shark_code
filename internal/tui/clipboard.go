package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		for _, name := range []string{"xclip", "wl-copy", "xsel"} {
			if _, err := exec.LookPath(name); err != nil {
				continue
			}
			if name == "xclip" {
				cmd = exec.Command(name, "-selection", "clipboard")
			} else {
				cmd = exec.Command(name)
			}
			break
		}
		if cmd == nil {
			return fmt.Errorf("no clipboard tool found (install xclip or wl-copy)")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
