//go:build !windows

package install

import (
	"fmt"
	"os"
	"strings"
)

// AddUserPath does not edit any shell configuration file — silently
// rewriting a user's .bashrc/.zshrc is exactly the kind of surprising,
// hard-to-notice side effect this tool avoids elsewhere (the wizard's
// location step exists specifically because silently picking a
// directory on the user's behalf was the wrong call — see
// cli/internal/prompt/wizard.go). Instead this returns the exact line
// to add and which file most likely wants it, so the action stays
// visible, reversible, and under the user's own control.
func AddUserPath(dir string, dryRun bool) (Result, error) {
	var res Result
	if PathContainsDir(dir) {
		res.Actions = append(res.Actions, fmt.Sprintf("%s is already on PATH", dir))
		return res, nil
	}
	rc := shellConfigFile()
	line := fmt.Sprintf(`export PATH="%s:$PATH"`, dir)
	res.Actions = append(res.Actions,
		fmt.Sprintf("add this line to %s (not done automatically):", rc),
		"  "+line,
	)
	return res, nil
}

// RemoveUserPath is Uninstall's counterpart: since Install never
// edited a dotfile, there's nothing to automatically undo — this just
// reminds the user what line to remove if they added it by hand.
func RemoveUserPath(dir string, dryRun bool) (Result, error) {
	var res Result
	rc := shellConfigFile()
	res.Actions = append(res.Actions, fmt.Sprintf("if you added %s to PATH via %s, remove that line by hand", dir, rc))
	return res, nil
}

// shellConfigFile guesses which shell config file the PATH export
// line belongs in, from $SHELL — a guess, not a guarantee, which is
// exactly why it's printed as a suggestion rather than edited
// directly.
func shellConfigFile() string {
	shell := os.Getenv("SHELL")
	home, _ := os.UserHomeDir()
	switch {
	case strings.Contains(shell, "zsh"):
		return home + "/.zshrc"
	case strings.Contains(shell, "fish"):
		return home + "/.config/fish/config.fish"
	default:
		return home + "/.bashrc (or ~/.profile / ~/.bash_profile)"
	}
}
