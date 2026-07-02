package main

import (
	"encoding/json"
	"testing"
)

func TestSearchToolAnnotations(t *testing.T) {
	t.Parallel()

	tool := newSearchTool(json.RawMessage(`{}`))

	if tool.Annotations == nil {
		t.Fatal("search tool should have ToolAnnotations set")
	}

	if !tool.Annotations.ReadOnlyHint {
		t.Error("search tool ReadOnlyHint should be true")
	}

	if tool.Annotations.OpenWorldHint == nil {
		t.Fatal("search tool OpenWorldHint should be set (non-nil)")
	}

	if !*tool.Annotations.OpenWorldHint {
		t.Error("search tool OpenWorldHint should be true")
	}
}

func TestSearchToolName(t *testing.T) {
	t.Parallel()

	tool := newSearchTool(json.RawMessage(`{}`))

	if tool.Name != "search" {
		t.Errorf("expected tool name 'search', got %q", tool.Name)
	}
}
