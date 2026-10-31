// Package registry holds the reviewed facts about third-party actions that
// trust zones, permissions and the verifier rely on (design §6). It is
// advisory: it can cause errors but never grants anything.
package registry

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

//go:embed registry.yml
var raw []byte

// Entry describes one reviewed action.
type Entry struct {
	Majors                []string   `yaml:"majors"`
	PrivilegedOK          bool       `yaml:"privileged_ok"`
	FetchesRefs           bool       `yaml:"fetches_refs"`
	RestoresCache         string     `yaml:"restores_cache"`
	SecretInputs          []string   `yaml:"secret_inputs"`
	ForbiddenInPrivileged []string   `yaml:"forbidden_in_privileged"`
	Requires              []string   `yaml:"requires"`
	Container             *Container `yaml:"container"`
	Reviewed              Review     `yaml:"reviewed"`
}

// Container is a digest-pinned image Pinfold runs instead of the action (D-035).
type Container struct {
	Image  string                    `yaml:"image"`
	Tag    string                    `yaml:"tag"`
	Digest string                    `yaml:"digest"`
	Inputs map[string]ContainerInput `yaml:"inputs"`
}

// ContainerInput maps a with: input to the environment variable the image reads.
type ContainerInput struct {
	Env     string `yaml:"env"`
	Default string `yaml:"default"`
}

// Ref returns the pinned image reference, e.g. ghcr.io/owner/image@sha256:....
func (c *Container) Ref() string { return c.Image + "@" + c.Digest }

// Review records where an entry's facts come from.
type Review struct {
	Source string `yaml:"source"`
	Commit string `yaml:"commit"`
	Date   string `yaml:"date"`
	Notes  string `yaml:"notes"`
}

type file struct {
	Version int               `yaml:"version"`
	Actions map[string]*Entry `yaml:"actions"`
}

var (
	once    sync.Once
	loaded  file
	loadErr error
)

func load() (file, error) {
	once.Do(func() {
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		loadErr = dec.Decode(&loaded)
	})
	return loaded, loadErr
}

// Digest identifies this registry; it is recorded in pinfold.lock (M4).
func Digest() string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Ref is a parsed uses: reference.
type Ref struct {
	Name  string // owner/repo or owner/repo/path
	Tag   string
	Major string // "v7" for "v7", "v7.0.1" or "v7.0"
}

// ParseRef splits owner/repo[/path]@tag. The spec decoder has already checked the form.
func ParseRef(uses string) (Ref, error) {
	name, tag, ok := strings.Cut(uses, "@")
	if !ok || name == "" || tag == "" {
		return Ref{}, fmt.Errorf("invalid action reference %q", uses)
	}
	major, _, _ := strings.Cut(tag, ".")
	return Ref{Name: name, Tag: tag, Major: major}, nil
}

// Lookup returns the entry for a uses: reference, or nil if the action isn't in
// the registry. reviewed reports whether the reference's major version was reviewed.
func Lookup(uses string) (e *Entry, reviewed bool, err error) {
	f, err := load()
	if err != nil {
		return nil, false, fmt.Errorf("registry: %w", err)
	}
	r, err := ParseRef(uses)
	if err != nil {
		return nil, false, err
	}
	e = f.Actions[r.Name]
	if e == nil {
		return nil, false, nil
	}
	for _, m := range e.Majors {
		if m == r.Major {
			return e, true, nil
		}
	}
	return e, false, nil
}

// All returns every entry by name.
func All() (map[string]*Entry, error) {
	f, err := load()
	return f.Actions, err
}
