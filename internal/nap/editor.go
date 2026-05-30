package nap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
)

const (
	defaultEditor        = "nano"
	searchEditor         = "hx"
	defaultMarkdownStyle = "auto"
)

// Cmd returns a *exec.Cmd editing the given path with $EDITOR or nano if no
// $EDITOR is set.
func editorCmd(path string) *exec.Cmd {
	editor, args := getEditor()
	return exec.Command(editor, append(args, path)...)
}

func searchEditorCmd(path string, line, column int) *exec.Cmd {
	target := path
	if line > 0 && column > 0 {
		target = fmt.Sprintf("%s:%d:%d", path, line, column)
	}
	return exec.Command(searchEditor, target)
}

func getEditor() (string, []string) {
	editor := strings.Fields(os.Getenv("EDITOR"))
	if len(editor) > 0 {
		return editor[0], editor[1:]
	}
	return defaultEditor, nil
}

func renderMarkdown(content string, width int, style string) (string, error) {
	options := []glamour.TermRendererOption{
		markdownStyleOption(style),
	}
	if width > 0 {
		options = append(options, glamour.WithWordWrap(width))
	}

	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return "", err
	}

	return renderer.Render(content)
}

func markdownStyleOption(style string) glamour.TermRendererOption {
	if _, err := os.Stat(style); err == nil {
		return glamour.WithStylePath(style)
	}
	return glamour.WithStandardStyle(style)
}

func isMarkdownLanguage(language string) bool {
	switch strings.ToLower(language) {
	case "md", "markdown", "mdown", "mkd":
		return true
	default:
		return false
	}
}

func filepathBase(path string) string {
	return filepath.Base(path)
}
