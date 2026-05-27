package nap

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestPreviewCmdUsesGlow(t *testing.T) {
	if err := os.Unsetenv("PREVIEWER"); err != nil {
		t.Fatalf("could not unset PREVIEWER: %v", err)
	}

	cmd := previewCmd(72)

	if filepath.Base(cmd.Path) != defaultPreviewer {
		t.Fatalf("previewer is incorrect: want %q but got %q", defaultPreviewer, cmd.Path)
	}

	wantArgs := fmt.Sprint([]string{defaultPreviewer, "-s", defaultGlowStyle, "-w", "72"})
	if argStr := fmt.Sprint(cmd.Args); argStr != wantArgs {
		t.Fatalf("previewer args are incorrect: want %q but got %q", wantArgs, argStr)
	}
}

func TestPreviewCmdPreservesExplicitStyle(t *testing.T) {
	if err := os.Setenv("PREVIEWER", "/snap/bin/glow -s light"); err != nil {
		t.Fatalf("could not set PREVIEWER: %v", err)
	}

	cmd := previewCmd(72)

	wantArgs := fmt.Sprint([]string{"/snap/bin/glow", "-s", "light", "-w", "72"})
	if argStr := fmt.Sprint(cmd.Args); argStr != wantArgs {
		t.Fatalf("previewer args are incorrect: want %q but got %q", wantArgs, argStr)
	}
}

func TestGetPreviewer(t *testing.T) {
	tt := []struct {
		Name         string
		PreviewerEnv string
		Cmd          string
		Args         []string
	}{
		{
			Name: "default",
			Cmd:  "glow",
		},
		{
			Name:         "custom path",
			PreviewerEnv: "/snap/bin/glow",
			Cmd:          "/snap/bin/glow",
		},
		{
			Name:         "custom path with flag",
			PreviewerEnv: "/snap/bin/glow -s dark",
			Cmd:          "/snap/bin/glow",
			Args:         []string{"-s", "dark"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			var err error
			switch tc.PreviewerEnv {
			case "":
				err = os.Unsetenv("PREVIEWER")
			default:
				err = os.Setenv("PREVIEWER", tc.PreviewerEnv)
			}
			if err != nil {
				t.Fatalf("could not (un)set PREVIEWER: %v", err)
			}

			cmd, args := getPreviewer()

			if cmd != tc.Cmd {
				t.Fatalf("previewer is incorrect: want %q but got %q", tc.Cmd, cmd)
			}

			if argStr, tcArgStr := fmt.Sprint(args), fmt.Sprint(tc.Args); argStr != tcArgStr {
				t.Fatalf("previewer args are incorrect: want %q but got %q", tcArgStr, argStr)
			}
		})
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
