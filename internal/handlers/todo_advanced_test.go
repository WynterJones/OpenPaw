package handlers

import (
	"strings"
	"testing"
)

// An attachment with no resolved path is dropped rather than stored: a row
// pointing nowhere would be handed to an agent as a path it cannot open.
func TestEncodeAttachments_DropsPathless(t *testing.T) {
	got := encodeAttachments([]TodoAttachment{
		{Kind: "file", Path: "/real/file.md", Name: "file"},
		{Kind: "file", Path: "  ", Name: "unresolved"},
		{Kind: "media", Name: "no path at all"},
	})

	list := decodeAttachments(got)
	if len(list) != 1 {
		t.Fatalf("kept %d attachments, want 1: %s", len(list), got)
	}
	if list[0].Path != "/real/file.md" {
		t.Errorf("kept the wrong one: %+v", list[0])
	}
}

// Ref is a write-only input. Persisting it would leave a stale id next to the
// path it already resolved to, which is the kind of thing that silently rots.
func TestEncodeAttachments_NeverStoresRef(t *testing.T) {
	got := encodeAttachments([]TodoAttachment{
		{Kind: "file", Path: "/real/file.md", Name: "file", Ref: "ctx-123"},
	})
	if strings.Contains(got, "ctx-123") {
		t.Errorf("stored the ref: %s", got)
	}
	if decodeAttachments(got)[0].Ref != "" {
		t.Error("decoded attachment still carries a ref")
	}
}

func TestEncodeAttachments_DefaultsAndCap(t *testing.T) {
	one := decodeAttachments(encodeAttachments([]TodoAttachment{{Path: "/a/b.txt"}}))[0]
	if one.Kind != "file" {
		t.Errorf("kind = %q, want the file default", one.Kind)
	}
	if one.Name != "/a/b.txt" {
		t.Errorf("name = %q, want it to fall back to the path", one.Name)
	}

	many := make([]TodoAttachment, maxTodoAttachments+10)
	for i := range many {
		many[i] = TodoAttachment{Kind: "file", Path: "/f/" + string(rune('a'+i%26)) + string(rune('0'+i/26))}
	}
	if n := len(decodeAttachments(encodeAttachments(many))); n != maxTodoAttachments {
		t.Errorf("kept %d attachments, want the cap of %d", n, maxTodoAttachments)
	}
}

// A malformed column must not stop an agent reading the task itself.
func TestDecodeAttachments_MalformedIsEmptyNotNil(t *testing.T) {
	for _, raw := range []string{"", "   ", "not json", "{}", "null"} {
		got := decodeAttachments(raw)
		if got == nil {
			t.Errorf("decodeAttachments(%q) returned nil — the UI maps over this", raw)
		}
		if len(got) != 0 {
			t.Errorf("decodeAttachments(%q) = %v, want empty", raw, got)
		}
	}
}

func TestFormatTodoAttachments(t *testing.T) {
	raw := encodeAttachments([]TodoAttachment{
		{Kind: "image", Path: "/data/shot.png", Name: "screenshot"},
		{Kind: "directory", Path: "/repo", Name: "repo"},
	})
	out := FormatTodoAttachments(raw)

	for _, want := range []string{"/data/shot.png", "screenshot", "image", "/repo", "directory"} {
		if !strings.Contains(out, want) {
			t.Errorf("agent-facing text missing %q:\n%s", want, out)
		}
	}
	if FormatTodoAttachments("[]") != "" {
		t.Error("no attachments should produce no text, not an empty header")
	}
}
