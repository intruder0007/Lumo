package prompt

import (
	"bytes"
	"strings"
	"testing"
)

// TestTextInput_LiteralCharactersThatAlsoActAsMenuKeys reproduces a real
// bug report: typing a path like "C:\Users\Jlaj jain\OneDrive\Desktop"
// into the wizard's Location field silently dropped the letters j, k,
// g, G, q and the space character, corrupting "Jlaj jain" into "Jlaain"
// and "Desktop" into "Destop". readKeyByte (input.go) classifies those
// bytes as menu-navigation keys (keyDown/keyUp/keyHome/keyEnd/keyCancel/
// keySpace) for selectMenu's benefit, but textInput's switch had no case
// for them, so they were silently discarded instead of typed literally.
func TestTextInput_LiteralCharactersThatAlsoActAsMenuKeys(t *testing.T) {
	cases := []struct {
		name  string
		typed string // keystrokes, terminated with Enter
		want  string
	}{
		{"lowercase j (menu: down)", "Jlaj jain\r", "Jlaj jain"},
		{"lowercase k (menu: up)", "Desktop\r", "Desktop"},
		{"lowercase g (menu: home)", "garage\r", "garage"},
		{"uppercase G (menu: end)", "Garage\r", "Garage"},
		{"lowercase q (menu: cancel)", "quiet\r", "quiet"},
		{"space (menu: keySpace)", "my projects\r", "my projects"},
		{"mixed, matches the real bug report", `C:\Users\Jlaj jain\OneDrive\Desktop` + "\r", `C:\Users\Jlaj jain\OneDrive\Desktop`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := TextInput(&out, strings.NewReader(c.typed), GetTheme("minimal", true), "Location", "", "")
			if err != nil {
				t.Fatalf("TextInput(%q): %v", c.typed, err)
			}
			if got != c.want {
				t.Errorf("TextInput(%q) = %q, want %q", c.typed, got, c.want)
			}
		})
	}
}

// TestTextInput_ArrowKeysStayNoOps guards against a naive fix that makes
// b==27 always mean "clear buffer": real arrow-key/Home/End escape
// sequences must still be harmless no-ops in a free-text field, not
// wipe what the user already typed.
func TestTextInput_ArrowKeysStayNoOps(t *testing.T) {
	var out bytes.Buffer
	typed := "abc" + "\x1b[A" + "\x1b[D" + "\x1b[H" + "def\r" // "abc", up, left, home, "def", enter
	got, err := TextInput(&out, strings.NewReader(typed), GetTheme("minimal", true), "Location", "", "")
	if err != nil {
		t.Fatalf("TextInput: %v", err)
	}
	if got != "abcdef" {
		t.Errorf("TextInput with arrow/home keys interleaved = %q, want %q (arrows/home should be no-ops, not clear the buffer)", got, "abcdef")
	}
}

// TestTextInput_BareEscClearsBuffer preserves existing behavior: a real
// bare Esc (not part of an arrow-key sequence) clears the buffer first.
func TestTextInput_BareEscClearsBuffer(t *testing.T) {
	var out bytes.Buffer
	typed := "abc" + "\x1b" + "def\r" // "abc", bare Esc (clears), "def", enter
	got, err := TextInput(&out, strings.NewReader(typed), GetTheme("minimal", true), "Location", "", "")
	if err != nil {
		t.Fatalf("TextInput: %v", err)
	}
	if got != "def" {
		t.Errorf("TextInput with bare Esc = %q, want %q (Esc should clear the prior buffer)", got, "def")
	}
}
