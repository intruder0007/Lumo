package prompt

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrCancelled is returned by the interactive widgets when the user
// cancels (Ctrl+C or 'q' with an empty filter).
var ErrCancelled = errors.New("cancelled")

// ErrBack is returned by the interactive widgets when the user presses a
// back key (← or 'h' with an empty filter, or Esc with an empty filter)
// to return to the previous screen (design-spec §3.3).
var ErrBack = errors.New("back")

// fuzzyMatches reports whether needle is a case-insensitive subsequence
// of hay — "rbapi" matches "REST API (node:http)" — the match rule for
// widget type-ahead filtering. An empty needle matches everything.
func fuzzyMatches(hay, needle string) bool {
	h := strings.ToLower(hay)
	for _, r := range strings.ToLower(needle) {
		idx := strings.IndexRune(h, r)
		if idx < 0 {
			return false
		}
		h = h[idx+1:]
	}
	return true
}

func widgetTitle(t Theme, text string) string {
	return t.Header(text + ":")
}

// filterRow formats the live filter/count indicator under a menu.
func filterRow(t Theme, filter string, shown, total int) string {
	if filter == "" {
		return ""
	}
	return t.Dim(fmt.Sprintf("filter: %s (%d/%d)", filter, shown, total))
}

// optionLine renders one menu row. box is non-empty for multi-select.
func optionLine(t Theme, o option, highlighted bool, box string) string {
	prefix := "  "
	if highlighted {
		prefix = t.Glyph(GlyphCursor) + " "
	}
	name := o.Name
	if !o.Available {
		name = t.Dim(name + " (coming soon)")
	}
	if box != "" {
		return prefix + box + " " + name
	}
	return prefix + name
}

// firstVisiblePos returns the index into visible of the first
// available option matching defaultID, else the first available option,
// else 0.
func firstVisiblePos(opts []option, visible []int, defaultID string) int {
	for pos, idx := range visible {
		if opts[idx].ID == defaultID && opts[idx].Available {
			return pos
		}
	}
	for pos, idx := range visible {
		if opts[idx].Available {
			return pos
		}
	}
	return 0
}

// nextVisiblePos/prevVisiblePos move one position through visible,
// skipping unavailable options, wrapping around.
func nextVisiblePos(opts []option, visible []int, from int) int {
	for i := 1; i <= len(visible); i++ {
		pos := (from + i) % len(visible)
		if opts[visible[pos]].Available {
			return pos
		}
	}
	return from
}

func prevVisiblePos(opts []option, visible []int, from int) int {
	for i := 1; i <= len(visible); i++ {
		pos := (from - i + len(visible)) % len(visible)
		if opts[visible[pos]].Available {
			return pos
		}
	}
	return from
}

func firstAvailPos(opts []option, visible []int) int {
	return firstVisiblePos(opts, visible, "")
}

func lastAvailPos(opts []option, visible []int) int {
	for i := len(visible) - 1; i >= 0; i-- {
		if opts[visible[i]].Available {
			return i
		}
	}
	return 0
}

// applyFilter rebuilds visible from opts for the current filter and
// resets the cursor to the first match.
func applyFilter(opts []option, visible []int, filter string) ([]int, int) {
	visible = visible[:0]
	for i, o := range opts {
		if fuzzyMatches(o.Name, filter) {
			visible = append(visible, i)
		}
	}
	return visible, firstVisiblePos(opts, visible, "")
}

// SelectMenu renders opts and lets the user pick one available option
// with arrow keys (or j/k), Home/End (or g/G), and Enter, redrawing in
// place and atomically. Typing narrows the list by fuzzy subsequence
// match; backspace edits the filter; a bare Esc clears the filter first
// and acts as "back" when it is already empty; Ctrl+C and 'q' cancel
// (never conflicting with a non-empty filter). It returns the selected
// option's ID, ErrBack, or ErrCancelled.
//
// When 'w' is a real terminal the widget clears the screen and renders
// as a single screen; off a terminal it renders statically and
// deterministically (no escape codes).
func SelectMenu(w io.Writer, r io.Reader, t Theme, label string, opts []option, defaultID string) (string, error) {
	return selectMenu(NewOutput(w), r, t, label, opts, defaultID)
}

func selectMenu(o *Output, r io.Reader, t Theme, label string, opts []option, defaultID string) (string, error) {
	o.ClearScreen()

	visible := make([]int, len(opts))
	for i := range opts {
		visible[i] = i
	}
	cursor := firstVisiblePos(opts, visible, defaultID)
	filter := ""

	region := NewRegion(o)
	draw := func() {
		lines := []string{optionTitle(t, label)}
		for pos, idx := range visible {
			lines = append(lines, optionLine(t, opts[idx], pos == cursor, ""))
		}
		if len(visible) == 0 && filter != "" {
			lines = append(lines, t.Dim(fmt.Sprintf("no matches for %q", filter)))
		}
		lines = append(lines, "")
		if hint := menuHint(t, filter, "select", "enter", o); hint != "" {
			lines = append(lines, hint)
		}
		if filter != "" {
			lines = append(lines, filterRow(t, filter, len(visible), len(opts)))
		}
		region.Draw(lines)
	}
	draw()

	r = &pushbackReader{r: r}
	for {
		k, b, err := readKeyByte(r)
		if err != nil {
			return "", err
		}
		switch k {
		case keyCancel:
			if b == 27 && filter != "" {
				// Esc first clears the filter; back is the second Esc.
				filter = ""
				visible, cursor = applyFilter(opts, visible, filter)
				draw()
				continue
			}
			if b == 'q' && filter != "" {
				// 'q' never conflicts with filtering (design §3.2 rule 1).
				filter += "q"
				visible, cursor = applyFilter(opts, visible, filter)
				draw()
				continue
			}
			return "", ErrCancelled
		case keyUp:
			if len(visible) > 0 {
				cursor = prevVisiblePos(opts, visible, cursor)
				draw()
			}
		case keyDown:
			if len(visible) > 0 {
				cursor = nextVisiblePos(opts, visible, cursor)
				draw()
			}
		case keyHome:
			if len(visible) > 0 {
				cursor = firstAvailPos(opts, visible)
				draw()
			}
		case keyEnd:
			if len(visible) > 0 {
				cursor = lastAvailPos(opts, visible)
				draw()
			}
		case keyLeft:
			// explicit back key
			return "", ErrBack
		case keyEnter:
			if len(visible) > 0 && opts[visible[cursor]].Available {
				return opts[visible[cursor]].ID, nil
			}
		case keySpace:
			// single-select menus ignore space
		case keyOther:
			switch {
			case b == 127 || b == 8: // backspace/delete
				if filter != "" {
					filter = filter[:len(filter)-1]
					visible, cursor = applyFilter(opts, visible, filter)
					draw()
				}
			case b == 'h' && filter == "":
				return "", ErrBack
			case b >= 32 && b <= 126:
				filter += string(b)
				visible, cursor = applyFilter(opts, visible, filter)
				draw()
			}
		}
	}
}

// MultiSelectMenu renders opts with checkboxes; arrow keys move the
// cursor, space toggles the highlighted option, Enter confirms. Typing
// narrows the list by fuzzy subsequence match, exactly like SelectMenu.
// It returns the selected IDs, ErrBack, or ErrCancelled.
func MultiSelectMenu(w io.Writer, r io.Reader, t Theme, label string, opts []option) ([]string, error) {
	return multiSelectMenu(NewOutput(w), r, t, label, opts)
}

func multiSelectMenu(o *Output, r io.Reader, t Theme, label string, opts []option) ([]string, error) {
	o.ClearScreen()

	visible := make([]int, len(opts))
	for i := range opts {
		visible[i] = i
	}
	cursor := firstVisiblePos(opts, visible, "")
	selected := make(map[int]bool, len(opts))
	filter := ""

	region := NewRegion(o)
	draw := func() {
		lines := []string{optionTitle(t, label)}
		for pos, idx := range visible {
			box := t.Glyph(GlyphUnchecked)
			if selected[idx] {
				box = t.Glyph(GlyphChecked)
			}
			lines = append(lines, optionLine(t, opts[idx], pos == cursor, box))
		}
		if len(visible) == 0 && filter != "" {
			lines = append(lines, t.Dim(fmt.Sprintf("no matches for %q", filter)))
		}
		lines = append(lines, "")
		if hint := menuHint(t, filter, "toggle", "space", o); hint != "" {
			lines = append(lines, hint)
		}
		if filter != "" {
			lines = append(lines, filterRow(t, filter, len(visible), len(opts)))
		}
		region.Draw(lines)
	}
	draw()
	r = &pushbackReader{r: r}
	for {
		k, b, err := readKeyByte(r)
		if err != nil {
			return nil, err
		}
		switch k {
		case keyCancel:
			if b == 27 && filter != "" {
				filter = ""
				visible, cursor = applyFilter(opts, visible, filter)
				draw()
				continue
			}
			if b == 'q' && filter != "" {
				filter += "q"
				visible, cursor = applyFilter(opts, visible, filter)
				draw()
				continue
			}
			return nil, ErrCancelled
		case keyUp:
			if len(visible) > 0 {
				cursor = prevVisiblePos(opts, visible, cursor)
				draw()
			}
		case keyDown:
			if len(visible) > 0 {
				cursor = nextVisiblePos(opts, visible, cursor)
				draw()
			}
		case keyHome:
			if len(visible) > 0 {
				cursor = firstAvailPos(opts, visible)
				draw()
			}
		case keyEnd:
			if len(visible) > 0 {
				cursor = lastAvailPos(opts, visible)
				draw()
			}
		case keyLeft:
			return nil, ErrBack
		case keySpace:
			if len(visible) > 0 && opts[visible[cursor]].Available {
				selected[visible[cursor]] = !selected[visible[cursor]]
				draw()
			}
		case keyEnter:
			var out []string
			for i, o := range opts {
				if selected[i] {
					out = append(out, o.ID)
				}
			}
			return out, nil
		case keyOther:
			switch {
			case b == 127 || b == 8: // backspace/delete
				if filter != "" {
					filter = filter[:len(filter)-1]
					visible, cursor = applyFilter(opts, visible, filter)
					draw()
				}
			case b == 'h' && filter == "":
				return nil, ErrBack
			case b >= 32 && b <= 126:
				filter += string(b)
				visible, cursor = applyFilter(opts, visible, filter)
				draw()
			}
		}
	}
}

// TextInput collects a single line of typed input (design-spec §4.2).
// Backspace edits, Enter confirms, a bare Esc clears the buffer first
// and acts as back when it is empty, Ctrl+C cancels, and 'q' is a
// normal character (vim keys never apply to text input — §3.2 rule 3).
// Returns the line, ErrBack, or ErrCancelled. An inline advisory note
// appears while the text contains characters a project name can't.
// initial seeds the buffer with real, editable text (e.g. a remembered
// default) — unlike placeholder, which is only a dimmed hint shown
// while the buffer is empty and is never part of the returned value.
func TextInput(w io.Writer, r io.Reader, t Theme, label, placeholder, initial string) (string, error) {
	return textInput(NewOutput(w), r, t, label, placeholder, initial)
}

func textInput(o *Output, r io.Reader, t Theme, label, placeholder, initial string) (string, error) {
	o.ClearScreen()

	buf := []byte(initial)
	region := NewRegion(o)
	draw := func() {
		lines := []string{optionTitle(t, label), ""}
		caret := t.Token(TokenCaret, t.Glyph(GlyphCaret))
		if len(buf) == 0 && placeholder != "" {
			lines = append(lines, "  "+t.Dim(placeholder)+caret)
		} else {
			lines = append(lines, "  "+string(buf)+caret)
		}
		if hint := inlineAdvice(string(buf), t); hint != "" {
			lines = append(lines, t.Dim("  "+hint))
		}
		lines = append(lines, "")
		if h := inputHint(t, o); h != "" {
			lines = append(lines, h)
		}
		region.Draw(lines)
	}
	draw()

	r = &pushbackReader{r: r}
	for {
		k, b, err := readKeyByte(r)
		if err != nil {
			return "", err
		}
		// readKeyByte classifies j/k/g/G/q/space as menu-navigation keys
		// (keyDown/keyUp/keyHome/keyEnd/keyCancel/keySpace) for
		// selectMenu's benefit. A free-text field has no such menu, so
		// every one of those bytes must still be typeable literally —
		// checking the byte value first (not the logical key) is what
		// makes that possible; b's printable range (32-126) can never
		// overlap with Enter (\r/\n), Ctrl+C (3), Esc (27), or backspace/
		// delete (8/127), so this can't misfire on real control keys.
		// The one ambiguous case is b==27: a bare Esc and an arrow/Home/
		// End escape sequence both report it, so that branch still
		// checks k==keyCancel to tell a real Esc apart from a navigation
		// sequence (which stays a harmless no-op here, same as before).
		switch {
		case k == keyEnter:
			return string(buf), nil
		case k == keyCancel && b == 3: // real Ctrl+C
			return "", ErrCancelled
		case k == keyCancel && b == 27: // bare Esc: clear first, then back
			if len(buf) > 0 {
				buf = buf[:0]
				draw()
				continue
			}
			return "", ErrBack
		default:
			switch {
			case b == 127 || b == 8: // backspace/delete
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					draw()
				}
			case b >= 32 && b <= 126:
				buf = append(buf, b)
				draw()
			}
		}
	}
}

// confirm renders the confirmation summary (M-10 ConfirmCard) and waits
// for Enter (generate) or a back/cancel key. Returns true to proceed.
func confirm(o *Output, r io.Reader, t Theme, title string, rows []string) (bool, error) {
	o.ClearScreen()
	region := NewRegion(o)
	draw := func() {
		lines := []string{optionTitle(t, title), ""}
		lines = append(lines, ConfirmCard(t, "", rows)...)
		lines = append(lines, "")
		if h := confirmHint(t, o); h != "" {
			lines = append(lines, h)
		}
		region.Draw(lines)
	}
	draw()

	r = &pushbackReader{r: r}
	for {
		k, b, err := readKeyByte(r)
		if err != nil {
			return false, err
		}
		switch k {
		case keyEnter:
			return true, nil
		case keyLeft:
			return false, nil
		case keyCancel:
			if b == 3 || b == 'q' {
				return false, ErrCancelled
			}
		case keyOther:
			if b == 'h' {
				return false, nil
			}
		}
	}
}

// optionTitle renders the widget's screen title line.
func optionTitle(t Theme, label string) string {
	return t.Header(label)
}

// menuHint builds the contextual footer for a menu.
func menuHint(t Theme, filter string, selectWord, selectKeys string, o *Output) string {
	if !o.TTY() {
		return ""
	}
	nav := "↑↓ move"
	if !t.UseIcons {
		nav = "up/down move"
	}
	hint := nav + " · " + selectKeys + " " + selectWord + " · type to filter · esc back · q quit"
	return t.Muted(hint)
}

// inputHint builds the footer for the text input.
func inputHint(t Theme, o *Output) string {
	if !o.TTY() {
		return ""
	}
	return t.Muted("enter next · esc clear/back · ctrl+c exit")
}

// confirmHint builds the footer for the confirm screen.
func confirmHint(t Theme, o *Output) string {
	if !o.TTY() {
		return ""
	}
	return t.Muted("enter generate · ← back · q quit")
}

// inlineAdvice returns an advisory hint for the project-name input,
// or "" when the text is fine (design-spec §4.2: advisory while typing).
func inlineAdvice(s string, t Theme) string {
	if strings.ContainsAny(s, " \t") {
		return "name may contain only letters, digits, - or _ (a path may contain most other characters)"
	}
	return ""
}
