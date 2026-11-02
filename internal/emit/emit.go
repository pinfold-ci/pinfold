// Package emit writes YAML with full control over key order, quoting and
// line endings, so generated workflows are byte-for-byte deterministic and no
// scalar can be misread by a YAML 1.1 or 1.2 parser.
package emit

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Node is a YAML value: Map, Seq, Str, Bool, Int or Empty.
type Node interface{ node() }

// Map is a mapping with keys in the order given.
type Map []Pair

// Pair is one key/value. Comment, if set, is written after the key's line.
type Pair struct {
	Key     string
	Value   Node
	Comment string
}

// Seq is a sequence.
type Seq []Node

// Str is a string scalar, quoted whenever it would otherwise be misread.
type Str string

// Bool is a boolean scalar.
type Bool bool

// Int is an integer scalar.
type Int int

// Empty is an empty mapping, written as {}.
type Empty struct{}

func (Map) node()   {}
func (Seq) node()   {}
func (Str) node()   {}
func (Bool) node()  {}
func (Int) node()   {}
func (Empty) node() {}

// Document renders header comment lines followed by the root mapping.
// The output always uses LF line endings and ends with exactly one newline.
func Document(header []string, root Map) string {
	var b strings.Builder
	for _, h := range header {
		writeComment(&b, "", h)
	}
	writeMap(&b, root, 0)
	return b.String()
}

func writeComment(b *strings.Builder, indent, text string) {
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(strings.TrimRight(indent+"# "+sanitizeComment(line), " ") + "\n")
	}
}

// sanitizeComment keeps a comment on one line and printable. YAML parsers
// also break lines at NEL, LS and PS, so those become spaces too.
func sanitizeComment(s string) string {
	s = strings.ToValidUTF8(s, " ")
	return strings.Map(func(r rune) rune {
		if !printable(string(r)) || r == '\n' {
			return ' '
		}
		return r
	}, s)
}

// MaxKey is the longest key the emitter writes, measured as written (after
// quoting and escaping). YAML caps implicit keys at 1024 characters; the spec
// keeps every key well below this.
const MaxKey = 1000

func key(k string) string {
	out := Scalar(k)
	if len(out) > MaxKey {
		panic(fmt.Sprintf("emit: key longer than %d bytes as written (compiler bug: the spec should have rejected it)", MaxKey))
	}
	return out
}

func writeMap(b *strings.Builder, m Map, depth int) {
	ind := strings.Repeat("  ", depth)
	for _, p := range m {
		b.WriteString(ind + key(p.Key) + ":")
		writeValue(b, p.Value, depth, p.Comment)
	}
}

// writeValue writes what follows "key:" or "-".
func writeValue(b *strings.Builder, v Node, depth int, comment string) {
	tail := ""
	if comment != "" {
		tail = " # " + sanitizeComment(comment)
	}
	switch v := v.(type) {
	case Map:
		if len(v) == 0 {
			b.WriteString(" {}" + tail + "\n")
			return
		}
		b.WriteString(tail + "\n")
		writeMap(b, v, depth+1)
	case Seq:
		if len(v) == 0 {
			b.WriteString(" []" + tail + "\n")
			return
		}
		b.WriteString(tail + "\n")
		writeSeq(b, v, depth+1)
	case Empty:
		b.WriteString(" {}" + tail + "\n")
	case Bool:
		fmt.Fprintf(b, " %t%s\n", bool(v), tail)
	case Int:
		fmt.Fprintf(b, " %d%s\n", int(v), tail)
	case Str:
		if block, ok := literalBlock(string(v), depth+1); ok {
			b.WriteString(" " + block.indicator + tail + "\n" + block.body)
			return
		}
		b.WriteString(" " + Scalar(string(v)) + tail + "\n")
	default:
		panic(fmt.Sprintf("emit: unknown node %T", v))
	}
}

func writeSeq(b *strings.Builder, s Seq, depth int) {
	ind := strings.Repeat("  ", depth)
	for _, item := range s {
		if m, ok := item.(Map); ok && len(m) > 0 {
			// "- key: value" with the rest of the mapping aligned under the first key.
			b.WriteString(ind + "- " + key(m[0].Key) + ":")
			writeValue(b, m[0].Value, depth+1, m[0].Comment)
			writeMap(b, m[1:], depth+1)
			continue
		}
		b.WriteString(ind + "-")
		writeValue(b, item, depth, "")
	}
}

type block struct{ indicator, body string }

// literalBlock renders multi-line text as a "|" block when that round-trips
// exactly; otherwise the caller falls back to a quoted scalar.
func literalBlock(s string, depth int) (block, bool) {
	if !strings.Contains(s, "\n") || !printable(s) || strings.ContainsAny(s, "\r\t") {
		return block{}, false
	}
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		if l != strings.TrimRight(l, " ") {
			return block{}, false // trailing spaces are fragile in blocks
		}
	}
	chomp := "-"
	if strings.HasSuffix(s, "\n") {
		chomp = ""
		lines = lines[:len(lines)-1]
		if strings.HasSuffix(s, "\n\n") {
			return block{}, false // needs "+" chomping; quote instead
		}
	}
	if lines[0] == "" || strings.HasPrefix(lines[0], " ") {
		return block{}, false // would need an indentation indicator; quote instead
	}
	ind := strings.Repeat("  ", depth)
	indicator := "|" + chomp
	var b strings.Builder
	for _, l := range lines {
		if l == "" {
			b.WriteString("\n")
		} else {
			b.WriteString(ind + l + "\n")
		}
	}
	return block{indicator, b.String()}, true
}

var (
	plainRE = regexp.MustCompile(`^[A-Za-z_./][A-Za-z0-9_./@+=-]*(?: [A-Za-z0-9_./@+=()-]+)*$`)
	// Words a YAML 1.1 or 1.2 parser reads as booleans or null.
	specialWords = map[string]bool{
		"y": true, "yes": true, "n": true, "no": true, "true": true, "false": true,
		"on": true, "off": true, "null": true,
	}
)

// Scalar renders s as a plain scalar when that is unambiguous, otherwise
// single-quoted, or double-quoted with escapes when it contains control
// characters or invalid UTF-8.
func Scalar(s string) string {
	if plainRE.MatchString(s) && !specialWords[strings.ToLower(s)] && !strings.HasPrefix(s, ".") {
		return s
	}
	if printable(s) && !strings.Contains(s, "\n") {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return doubleQuoted(s)
}

func printable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (unicode.IsControl(r) && r != '\n') || r == '\ufeff' || r == '\u2028' || r == '\u2029' || r == '\u0085' || r == '\ufffe' || r == '\uffff' {
			return false
		}
	}
	return true
}

func doubleQuoted(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, "\\x%02x", s[i]) // invalid UTF-8 can't round-trip; make it visible
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f || r == '\ufeff' || r == '\u2028' || r == '\u2029' || r == '\u0085' || r == '\ufffe' || r == '\uffff' || (unicode.IsControl(r) && r > 0x7f):
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}
