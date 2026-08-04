package notification

import "testing"

const (
	testNode   = "node-a"
	testTarget = "v1.11.0"
)

func TestDefaultRenderer_WithCurrentVersion(t *testing.T) {
	title, message, err := DefaultRenderer().Render(EventData{
		Node:           testNode,
		CurrentVersion: "v1.10.0",
		TargetVersion:  testTarget,
		Plan:           "cluster",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if title != "Tuppr Upgrade Started" {
		t.Fatalf("title = %q", title)
	}
	if want := "Node node-a is upgrading Talos from v1.10.0 -> v1.11.0"; message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}

func TestDefaultRenderer_WithoutCurrentVersion(t *testing.T) {
	_, message, err := DefaultRenderer().Render(EventData{Node: testNode, TargetVersion: testTarget})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := "Starting upgrade for node node-a"; message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}

func TestNewRenderer_CustomTemplatesWithSproutFunc(t *testing.T) {
	// toUpper is sprout's name; the sprig-backward alias "upper" is intentionally absent.
	r, err := NewRenderer("[{{ .Plan | toUpper }}]", "{{ .Node }} -> {{ .TargetVersion }}")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	title, message, err := r.Render(EventData{Node: testNode, TargetVersion: testTarget, Plan: "prod"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if title != "[PROD]" {
		t.Fatalf("title = %q", title)
	}
	if message != testNode+" -> "+testTarget {
		t.Fatalf("message = %q", message)
	}
}

func TestNewRenderer_InvalidTemplate(t *testing.T) {
	if _, err := NewRenderer("{{ .Node", ""); err == nil {
		t.Fatal("expected error for invalid title template")
	}
	if _, err := NewRenderer("", "{{ .Node | nosuchfunc }}"); err == nil {
		t.Fatal("expected error for invalid message template")
	}
}

func TestFuncMap_ExcludesIOAndHasStrictEnv(t *testing.T) {
	fm := funcMap()

	for _, banned := range []string{"readFile", "readDir", "getHostByName", "upper"} {
		if _, ok := fm[banned]; ok {
			t.Errorf("funcMap must not expose %q", banned)
		}
	}
	if _, ok := fm["trim"]; !ok {
		t.Error("funcMap should include safe helpers (e.g. trim)")
	}

	fn, ok := fm["env"].(func(string) (string, error))
	if !ok {
		t.Fatalf("env = %T, want func(string) (string, error)", fm["env"])
	}
	t.Setenv("TUPPR_TEST_VAR", "present")
	if v, err := fn("TUPPR_TEST_VAR"); err != nil || v != "present" {
		t.Errorf("env(set) = %q, %v; want present, nil", v, err)
	}
	if _, err := fn("TUPPR_TEST_DEFINITELY_UNSET"); err == nil {
		t.Error("env(unset) should error (strict), got nil")
	}
}
