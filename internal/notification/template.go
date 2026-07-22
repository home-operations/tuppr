package notification

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/registry/checksum"
	"github.com/go-sprout/sprout/registry/conversion"
	"github.com/go-sprout/sprout/registry/encoding"
	"github.com/go-sprout/sprout/registry/env"
	"github.com/go-sprout/sprout/registry/maps"
	"github.com/go-sprout/sprout/registry/numeric"
	"github.com/go-sprout/sprout/registry/random"
	"github.com/go-sprout/sprout/registry/reflect"
	"github.com/go-sprout/sprout/registry/regexp"
	"github.com/go-sprout/sprout/registry/semver"
	"github.com/go-sprout/sprout/registry/slices"
	"github.com/go-sprout/sprout/registry/std"
	"github.com/go-sprout/sprout/registry/strings"
	"github.com/go-sprout/sprout/registry/time"
	"github.com/go-sprout/sprout/registry/uniqueid"
)

// Built-in templates. The message renders the version-less form when the
// current version could not be determined (CurrentVersion empty).
const (
	defaultTitleTemplate   = "Tuppr Upgrade Started"
	defaultMessageTemplate = "{{ if .CurrentVersion }}Node {{ .Node }} is upgrading Talos from {{ .CurrentVersion }} -> {{ .TargetVersion }}{{ else }}Starting upgrade for node {{ .Node }}{{ end }}"
)

// EventData is the context exposed to notification templates.
type EventData struct {
	Node           string // node being upgraded
	CurrentVersion string // current Talos version; empty if undetermined
	TargetVersion  string // target Talos version
	Plan           string // TalosUpgrade resource name
}

// Renderer renders notification titles and messages from Go templates, with the
// go-sprout/sprout safe helpers plus a strict env available (matching chaski).
type Renderer struct {
	title   *template.Template
	message *template.Template
}

// NewRenderer parses the title and message templates; an empty string selects
// the built-in default for that field. It errors on an invalid template.
func NewRenderer(titleTmpl, messageTmpl string) (*Renderer, error) {
	if titleTmpl == "" {
		titleTmpl = defaultTitleTemplate
	}
	if messageTmpl == "" {
		messageTmpl = defaultMessageTemplate
	}

	fm := funcMap()

	title, err := template.New("title").Funcs(fm).Parse(titleTmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing title template: %w", err)
	}
	message, err := template.New("message").Funcs(fm).Parse(messageTmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing message template: %w", err)
	}

	return &Renderer{title: title, message: message}, nil
}

// DefaultRenderer returns a Renderer using the built-in default templates.
func DefaultRenderer() *Renderer {
	r, err := NewRenderer("", "")
	if err != nil {
		panic("notification: default templates must parse: " + err.Error())
	}
	return r
}

// Render produces the notification title and message for the given event.
func (r *Renderer) Render(data EventData) (title, message string, err error) {
	title, err = execute(r.title, data)
	if err != nil {
		return "", "", fmt.Errorf("rendering title: %w", err)
	}
	message, err = execute(r.message, data)
	if err != nil {
		return "", "", fmt.Errorf("rendering message: %w", err)
	}
	return title, message, nil
}

func execute(t *template.Template, data EventData) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// funcMap builds the sprout safe function map plus a strict env, mirroring
// chaski: no filesystem, network, or sprig-backward aliases, so a template
// cannot read a file or make a network call.
func funcMap() template.FuncMap {
	h := sprout.New()
	// AddRegistries only errors on a duplicate UID; the set below is static.
	if err := h.AddRegistries(
		std.NewRegistry(),
		strings.NewRegistry(),
		conversion.NewRegistry(),
		encoding.NewRegistry(),
		numeric.NewRegistry(),
		slices.NewRegistry(),
		maps.NewRegistry(),
		regexp.NewRegistry(),
		time.NewRegistry(),
		semver.NewRegistry(),
		random.NewRegistry(),
		checksum.NewRegistry(),
		uniqueid.NewRegistry(),
		reflect.NewRegistry(),
		env.NewRegistry(),
	); err != nil {
		panic("notification: building sprout handler: " + err.Error())
	}
	fm := h.Build()
	// Strict env: error on an unset variable rather than rendering empty.
	fm["env"] = strictEnv
	return fm
}

func strictEnv(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", key)
	}
	return v, nil
}
