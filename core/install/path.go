package install

import (
	"os"
	"runtime"
	"strings"
)

// PathContainsDir reports whether dir is already on the *current
// process's* PATH — a live check of this process's own environment,
// not the persisted registry value on Windows (a fresh shell picks up
// a change AddUserPath just made; this already-running process's
// environment won't until it's restarted).
func PathContainsDir(dir string) bool {
	return pathListContains(os.Getenv("PATH"), dir, string(os.PathListSeparator), runtime.GOOS == "windows")
}

// mergePath appends dir to a PATH-like value if it isn't already
// present, returning the merged value and whether anything changed.
// Pure — no file or registry access — so it's fully unit-testable
// regardless of host OS, unlike the registry/dotfile I/O that calls
// it. caseInsensitive should be true for Windows PATH values, false
// elsewhere.
func mergePath(current, dir, sep string, caseInsensitive bool) (merged string, changed bool) {
	if pathListContains(current, dir, sep, caseInsensitive) {
		return current, false
	}
	if current == "" {
		return dir, true
	}
	return current + sep + dir, true
}

// removePath removes dir from a PATH-like value, returning the
// result and whether anything changed. Same testability rationale as
// mergePath.
func removePath(current, dir, sep string, caseInsensitive bool) (result string, changed bool) {
	if current == "" {
		return current, false
	}
	var kept []string
	for _, p := range strings.Split(current, sep) {
		if pathEqual(p, dir, caseInsensitive) {
			changed = true
			continue
		}
		kept = append(kept, p)
	}
	if !changed {
		return current, false
	}
	return strings.Join(kept, sep), true
}

func pathListContains(list, dir, sep string, caseInsensitive bool) bool {
	if list == "" {
		return false
	}
	for _, p := range strings.Split(list, sep) {
		if pathEqual(p, dir, caseInsensitive) {
			return true
		}
	}
	return false
}

func pathEqual(a, b string, caseInsensitive bool) bool {
	a = strings.TrimRight(strings.TrimSpace(a), `\/`)
	b = strings.TrimRight(strings.TrimSpace(b), `\/`)
	if caseInsensitive {
		return strings.EqualFold(a, b)
	}
	return a == b
}
