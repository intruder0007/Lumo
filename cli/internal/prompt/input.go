package prompt

import (
	"io"

	"golang.org/x/term"
)

// key is a logical keypress recognized by the input handler.
type key int

const (
	keyUp key = iota
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyEnter
	keySpace
	keyCancel
	keyOther
)

// readByte reads exactly one byte from r.
func readByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// pushbackReader wraps an io.Reader with a one-byte unread buffer. The
// widgets use it so that a bare ESC doesn't swallow the byte after it:
// readKeyByte must look one byte ahead to recognize escape sequences,
// and when that lookahead turns out to be a plain key (ESC followed by
// Enter, say), it must be handed back to the next read.
type pushbackReader struct {
	r      io.Reader
	peeked []byte
}

func (p *pushbackReader) Read(b []byte) (int, error) {
	if len(p.peeked) > 0 {
		b[0] = p.peeked[0]
		p.peeked = p.peeked[1:]
		return 1, nil
	}
	return p.r.Read(b)
}

func (p *pushbackReader) unread(b byte) {
	p.peeked = append(p.peeked, b)
}

// readKeyByte reads one logical keypress from r, returning the key and
// the raw byte that produced it (the widgets need the byte to
// distinguish filter characters from backspace, and a bare ESC from
// Ctrl+C/'q'). Recognized:
//
//   - ANSI arrows: ESC [ A / B / C / D
//   - ANSI Home/End: ESC [ H / F, ESC [ 1~ / 4~
//   - Plain bytes: Enter, Space, Ctrl+C (3), Esc (27), Backspace
//     (8/127), printable ASCII (32-126)
//   - Vim keys in menus: j/k (down/up), h (left), g/G (home/end), q
//     (cancel)
//
// A bare ESC (not followed by '[') pushes any lookahead byte back and
// returns keyCancel.
func readKeyByte(r io.Reader) (key, byte, error) {
	b, err := readByte(r)
	if err != nil {
		return keyOther, 0, err
	}
	switch b {
	case 3: // Ctrl+C — raw mode delivers it as a plain byte, not a SIGINT
		return keyCancel, b, nil
	case 27: // ESC
		b2, err := readByte(r)
		if err != nil || b2 != '[' {
			if err == nil {
				if p, ok := r.(*pushbackReader); ok {
					p.unread(b2)
				}
			}
			return keyCancel, b, nil
		}
		b3, err := readByte(r)
		if err != nil {
			return keyOther, b, err
		}
		switch b3 {
		case 'A':
			return keyUp, b, nil
		case 'B':
			return keyDown, b, nil
		case 'C':
			return keyRight, b, nil
		case 'D':
			return keyLeft, b, nil
		case 'H':
			return keyHome, b, nil
		case 'F':
			return keyEnd, b, nil
		case '1', '4': // ESC [ 1~ (Home) / 4~ (End)
			b4, err := readByte(r)
			if err != nil {
				return keyOther, b, err
			}
			if b4 == '~' {
				if b3 == '1' {
					return keyHome, b, nil
				}
				return keyEnd, b, nil
			}
			if p, ok := r.(*pushbackReader); ok {
				p.unread(b4)
			}
			return keyOther, b, nil
		}
		return keyOther, b, nil
	case '\r', '\n':
		return keyEnter, b, nil
	case ' ':
		return keySpace, b, nil
	case 'k':
		return keyUp, b, nil
	case 'j':
		return keyDown, b, nil
	case 'g':
		return keyHome, b, nil
	case 'G':
		return keyEnd, b, nil
	case 'q':
		return keyCancel, b, nil
	default:
		return keyOther, b, nil
	}
}

// readKey reads one logical keypress from r, discarding the raw byte.
func readKey(r io.Reader) (key, error) {
	k, _, err := readKeyByte(r)
	return k, err
}

// RawKey, its constants, and ReadKeyByte let a caller outside this
// package (e.g. `lumo tui`'s panel-navigation loop) reuse the same
// escape-sequence-aware key classification as the wizard's widgets,
// instead of duplicating readKeyByte's arrow-key/vim-key parsing. Named
// RawKey, not Key, because Key (components.go) already names the
// unrelated key-hint display struct used by KeyHints.
type RawKey = key

const (
	RawKeyUp     = keyUp
	RawKeyDown   = keyDown
	RawKeyLeft   = keyLeft
	RawKeyRight  = keyRight
	RawKeyHome   = keyHome
	RawKeyEnd    = keyEnd
	RawKeyEnter  = keyEnter
	RawKeySpace  = keySpace
	RawKeyCancel = keyCancel
	RawKeyOther  = keyOther
)

// ReadKeyByte is the exported form of readKeyByte.
func ReadKeyByte(r io.Reader) (RawKey, byte, error) { return readKeyByte(r) }

// RawMode manages terminal raw mode for the duration of an interactive
// session, restoring the original state on Close (idempotent, safe on
// panic).
type RawMode struct {
	fd       int
	old      *term.State
	restored bool
}

// EnterRaw puts the terminal into raw mode. Call Close to restore.
func EnterRaw(fd int) (*RawMode, error) {
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return &RawMode{fd: fd, old: old}, nil
}

// Close restores the terminal to its original state.
func (r *RawMode) Close() {
	if !r.restored && r.old != nil {
		_ = term.Restore(r.fd, r.old)
		r.restored = true
	}
}
