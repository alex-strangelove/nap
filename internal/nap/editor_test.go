package nap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetEditor(t *testing.T) {
	tt := []struct {
		Name      string
		EditorEnv string
		Cmd       string
		Args      []string
	}{
		{
			Name: "default",
			Cmd:  "nano",
		},
		{
			Name:      "vim",
			EditorEnv: "vim",
			Cmd:       "vim",
		},
		{
			Name:      "vim with flag",
			EditorEnv: "vim --foo",
			Cmd:       "vim",
			Args:      []string{"--foo"},
		},
		{
			Name:      "code",
			EditorEnv: "code -w",
			Cmd:       "code",
			Args:      []string{"-w"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			var err error
			switch tc.EditorEnv {
			case "":
				err = os.Unsetenv("EDITOR")
			default:
				err = os.Setenv("EDITOR", tc.EditorEnv)
			}
			if err != nil {
				t.Logf("could not (un)set env: %v", err)
				t.FailNow()
			}

			cmd, args := getEditor()

			if cmd != tc.Cmd {
				t.Logf("cmd is incorrect: want %q but got %q", tc.Cmd, cmd)
				t.FailNow()
			}

			if argStr, tcArgStr := fmt.Sprint(args), fmt.Sprint(tc.Args); argStr != tcArgStr {
				t.Logf("args are incorrect: want %q but got %q", tcArgStr, argStr)
				t.FailNow()
			}
		})
	}
}

func TestSearchEditorCmdUsesHelixLocationTarget(t *testing.T) {
	cmd := searchEditorCmd("plans/roadmap.md", 3, 7)

	if filepath.Base(cmd.Path) != searchEditor {
		t.Fatalf("search editor is incorrect: want %q but got %q", searchEditor, cmd.Path)
	}

	wantArgs := fmt.Sprint([]string{searchEditor, "plans/roadmap.md:3:7"})
	if argStr := fmt.Sprint(cmd.Args); argStr != wantArgs {
		t.Fatalf("search editor args are incorrect: want %q but got %q", wantArgs, argStr)
	}
}

func TestRenderMarkdownUsesConfiguredStyle(t *testing.T) {
	rendered, err := renderMarkdown("# Title\n\nHello **world**.", 72, "auto")
	if err != nil {
		t.Fatalf("renderMarkdown returned error: %v", err)
	}
	if !strings.Contains(rendered, "Title") || !strings.Contains(rendered, "world") {
		t.Fatalf("rendered markdown is missing content: %q", rendered)
	}
}

func TestRenderMarkdownRejectsUnknownStyle(t *testing.T) {
	_, err := renderMarkdown("hello", 72, "missing-style")
	if err == nil {
		t.Fatal("expected renderMarkdown to fail for unknown style")
	}
	if !strings.Contains(err.Error(), "style not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEffectiveMarkdownStyleFollowsDarkTheme(t *testing.T) {
	cfg := newConfig()
	cfg.Theme = "dracula"
	cfg.MarkdownStyle = "auto"

	if got := cfg.effectiveMarkdownStyle(); got != "dark" {
		t.Fatalf("effective markdown style mismatch: got %q want %q", got, "dark")
	}
}

func TestEffectiveMarkdownStyleFollowsLightTheme(t *testing.T) {
	cfg := newConfig()
	cfg.Theme = "github"
	cfg.MarkdownStyle = "auto"

	if got := cfg.effectiveMarkdownStyle(); got != "light" {
		t.Fatalf("effective markdown style mismatch: got %q want %q", got, "light")
	}
}

func TestEffectiveMarkdownStylePreservesExplicitOverride(t *testing.T) {
	cfg := newConfig()
	cfg.Theme = "dracula"
	cfg.MarkdownStyle = "tokyo-night"

	if got := cfg.effectiveMarkdownStyle(); got != "tokyo-night" {
		t.Fatalf("effective markdown style mismatch: got %q want %q", got, "tokyo-night")
	}
}

func TestIsMarkdownLanguage(t *testing.T) {
	tests := map[string]bool{
		"md":       true,
		"markdown": true,
		"MD":       true,
		"go":       false,
		"txt":      false,
	}

	for language, want := range tests {
		if got := isMarkdownLanguage(language); got != want {
			t.Fatalf("markdown detection for %q is incorrect: want %t but got %t", language, want, got)
		}
	}
}

func TestSearchQueryLocation(t *testing.T) {
	loc, ok := searchQueryLocation("first line\nsecond marker here\nthird", "marker")
	if !ok {
		t.Fatal("expected to find marker")
	}
	if loc.line != 2 || loc.column != 8 {
		t.Fatalf("location mismatch: got %d:%d want 2:8", loc.line, loc.column)
	}
}
