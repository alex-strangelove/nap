package nap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStateMissingFileReturnsZeroState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "missing-state.json")
	t.Setenv("NAP_STATE", statePath)

	state := readState()
	if state.CurrentFolder != "" || state.CurrentSnippet != "" || len(state.ExpandedFolders) != 0 {
		t.Fatalf("unexpected state: got %#v want zero value", state)
	}
}

func TestReadStateReadsSavedState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	t.Setenv("NAP_STATE", statePath)

	want := State{
		CurrentFolder:   "work",
		CurrentSnippet:  "handler.go",
		ExpandedFolders: []string{"work"},
	}
	if err := want.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	got := readState()
	if got.CurrentFolder != want.CurrentFolder || got.CurrentSnippet != want.CurrentSnippet {
		t.Fatalf("state mismatch: got %#v want %#v", got, want)
	}
	if len(got.ExpandedFolders) != len(want.ExpandedFolders) || got.ExpandedFolders[0] != want.ExpandedFolders[0] {
		t.Fatalf("expanded folders mismatch: got %#v want %#v", got.ExpandedFolders, want.ExpandedFolders)
	}

	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
}
