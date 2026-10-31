// Package load reads a spec file into a YAML node tree, accepting only a
// restricted subset of YAML: one document, no anchors, aliases, merge keys or
// explicit tags, no duplicate keys, UTF-8, at most MaxSize bytes.
package load

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
)

// MaxSize is the largest spec file accepted, in bytes.
const MaxSize = 1 << 20

// ReadFile reads and loads the spec at path.
func ReadFile(path string) (*yaml.Node, diag.List) {
	src, diags := Read(path)
	if diags.HasErrors() {
		return nil, diags
	}
	return Load(path, src)
}

// Read returns the raw bytes of the spec at path, reading at most MaxSize+1
// bytes so an oversized file is rejected by Load without reading all of it.
func Read(path string) ([]byte, diag.List) {
	f, err := os.Open(path) // #nosec G304 -- the user names the spec file to read
	if err != nil {
		return nil, diag.List{codes.New("PF100", diag.Span{File: path}, fmt.Sprintf("can't read spec: %v", err), "check the path, or run \"pinfold init\" to create a spec")}
	}
	defer func() { _ = f.Close() }() // read-only; a close error changes nothing
	src, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return nil, diag.List{codes.New("PF100", diag.Span{File: path}, fmt.Sprintf("can't read spec: %v", err), "")}
	}
	return src, nil
}

// Load parses src (named file in diagnostics). On success it returns the
// top-level mapping node and no errors.
func Load(file string, src []byte) (*yaml.Node, diag.List) {
	at := func(line, col int) diag.Span { return diag.Span{File: file, Line: line, Col: col} }
	if len(src) > MaxSize {
		return nil, diag.List{codes.New("PF105", at(0, 0), fmt.Sprintf("spec is larger than %d bytes", MaxSize), "")}
	}
	if !utf8.Valid(src) {
		return nil, diag.List{codes.New("PF105", at(0, 0), "spec is not valid UTF-8", "save the file as UTF-8")}
	}
	src = Normalize(src)

	dec := yaml.NewDecoder(bytes.NewReader(src))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, diag.List{syntaxError(file, err)}
		}
		docs = append(docs, &doc)
	}
	if len(docs) == 0 || len(docs[0].Content) == 0 || isEmptyNull(docs[0].Content[0]) {
		return nil, diag.List{codes.New("PF104", at(0, 0), "spec is empty", "run \"pinfold init\" to create a starter spec")}
	}
	if len(docs) > 1 {
		return nil, diag.List{codes.New("PF104", at(docs[1].Line, docs[1].Column), "spec contains more than one YAML document", "remove the \"---\" separator and keep a single document")}
	}

	root := docs[0].Content[0]
	var diags diag.List
	checkSubset(file, root, &diags)
	if diags.HasErrors() {
		diags.Sort()
		return nil, diags
	}
	if root.Kind != yaml.MappingNode {
		return nil, diag.List{codes.New("PF112", at(root.Line, root.Column), "the spec must be a mapping (key: value pairs) at the top level", "start with \"version: 0\" and \"workflows:\"")}
	}
	return root, nil
}

// isEmptyNull reports an implicit null, such as the content of a bare "---".
func isEmptyNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null" && n.Value == ""
}

// Normalize strips a leading BOM and turns CRLF and lone CR into LF, so the
// same spec gives the same tree, positions and hash on every platform.
func Normalize(src []byte) []byte {
	src = bytes.TrimPrefix(src, []byte("\ufeff"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(src, []byte("\r"), []byte("\n")) // YAML treats a lone CR as a line break too
}

var lineRE = regexp.MustCompile(`^yaml: line (\d+): (.*)$`)

// syntaxError turns a yaml.v3 parse error into PF101, keeping the line number when there is one.
func syntaxError(file string, err error) diag.Diagnostic {
	msg := err.Error()
	span := diag.Span{File: file}
	if m := lineRE.FindStringSubmatch(msg); m != nil {
		span.Line, _ = strconv.Atoi(m[1])
		msg = m[2]
	} else {
		msg = strings.TrimPrefix(msg, "yaml: ")
	}
	if strings.HasPrefix(msg, "unknown anchor ") {
		// An alias to a missing anchor: aliases aren't allowed at all.
		return codes.New("PF102", span, "aliases are not allowed ("+msg+")", "write the value out in full")
	}
	return codes.New("PF101", span, "invalid YAML: "+msg, "check indentation (spaces only, no tabs) and quoting")
}

// checkSubset rejects YAML features outside the accepted subset.
func checkSubset(file string, n *yaml.Node, diags *diag.List) {
	at := diag.Span{File: file, Line: n.Line, Col: n.Column}
	switch {
	case n.Kind == yaml.AliasNode:
		diags.Add(codes.New("PF102", at, fmt.Sprintf("alias *%s is not allowed", n.Value), "write the value out in full"))
		return
	case n.Anchor != "":
		diags.Add(codes.New("PF102", at, fmt.Sprintf("anchor &%s is not allowed", n.Anchor), "remove the anchor and write values out in full"))
	case n.Style&yaml.TaggedStyle != 0:
		diags.Add(codes.New("PF102", at, fmt.Sprintf("explicit tag %s is not allowed", n.Tag), "remove the tag; quote the value if it must be a string"))
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			kat := diag.Span{File: file, Line: k.Line, Col: k.Column}
			if k.Kind == yaml.ScalarNode && k.Tag == "!!merge" {
				diags.Add(codes.New("PF102", kat, "merge key << is not allowed", "write the keys out in full"))
				continue
			}
			if k.Kind != yaml.ScalarNode {
				diags.Add(codes.New("PF112", kat, "mapping keys must be plain strings", ""))
				continue
			}
			if seen[k.Value] {
				diags.Add(codes.New("PF103", kat, fmt.Sprintf("duplicate key %q", k.Value), "remove or rename one of them"))
			}
			seen[k.Value] = true
		}
	}
	for _, c := range n.Content {
		checkSubset(file, c, diags)
	}
}
