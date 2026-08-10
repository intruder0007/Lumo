//go:build windows

package install

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// AddUserPath adds dir to HKCU\Environment\Path (the per-user PATH,
// no admin rights needed) if it isn't already there, then broadcasts
// WM_SETTINGCHANGE so processes that listen for it (Explorer, and
// shells started afterward) pick it up without a logoff. Already-open
// shells keep their own snapshot of PATH regardless — that's how
// Windows environment variables work, not something a broadcast can
// override.
//
// This function's own read of the current value (registry.GetStringValue)
// is non-mutating even when dryRun is false and nothing needs to
// change; the actual write only happens when there's a real change to
// make and dryRun is false.
func AddUserPath(dir string, dryRun bool) (Result, error) {
	return editUserPath(dir, dryRun, true)
}

// RemoveUserPath reverses AddUserPath.
func RemoveUserPath(dir string, dryRun bool) (Result, error) {
	return editUserPath(dir, dryRun, false)
}

func editUserPath(dir string, dryRun, add bool) (Result, error) {
	var res Result

	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return res, fmt.Errorf("opening HKCU\\Environment: %w", err)
	}
	defer k.Close()

	current, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return res, fmt.Errorf("reading current user PATH: %w", err)
	}

	var newVal string
	var changed bool
	if add {
		newVal, changed = mergePath(current, dir, ";", true)
	} else {
		newVal, changed = removePath(current, dir, ";", true)
	}

	if !changed {
		state := "already on"
		if !add {
			state = "already absent from"
		}
		res.Actions = append(res.Actions, fmt.Sprintf("%s is %s the user PATH", dir, state))
		return res, nil
	}

	verb, prep := "add", "to"
	if !add {
		verb, prep = "remove", "from"
	}
	res.Actions = append(res.Actions, fmt.Sprintf("%s %s %s the user PATH (registry: HKCU\\Environment\\Path)", verb, dir, prep))
	res.Changed = true
	if dryRun {
		return res, nil
	}

	if err := k.SetStringValue("Path", newVal); err != nil {
		return res, fmt.Errorf("writing user PATH: %w", err)
	}

	broadcastEnvironmentChange()
	res.Actions = append(res.Actions, "notify running processes of the PATH change (new shells pick it up immediately; already-open shells need to be restarted)")
	return res, nil
}

// broadcastEnvironmentChange sends WM_SETTINGCHANGE to all top-level
// windows so listeners (Explorer, some shells) refresh their
// environment without a logoff. Best-effort: a failure here doesn't
// undo the registry write, it just means the broadcast itself didn't
// reach anyone — the registry value, which is what actually matters
// for new processes, is already correct regardless.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	param, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	_, _, _ = proc.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(param)),
		uintptr(smtoAbortIfHung),
		uintptr(5000),
		uintptr(unsafe.Pointer(&result)),
	)
}
