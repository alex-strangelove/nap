package nap

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	defaultEditor    = "nano"
	searchEditor     = "hx"
	defaultPreviewer = "glow"
	defaultGlowStyle = "light"
)

var errPreviewerNotFound = errors.New("previewer not found")

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

func previewCmd(width int) *exec.Cmd {
	previewer, args := getPreviewer()
	if filepathBase(previewer) == defaultPreviewer && !hasPreviewStyle(args) {
		args = append(args, "-s", defaultGlowStyle)
	}
	if width > 0 {
		args = append(args, "-w", strconv.Itoa(width))
	}
	return exec.Command(previewer, args...)
}

func getEditor() (string, []string) {
	editor := strings.Fields(os.Getenv("EDITOR"))
	if len(editor) > 0 {
		return editor[0], editor[1:]
	}
	return defaultEditor, nil
}

func getPreviewer() (string, []string) {
	previewer := strings.Fields(os.Getenv("PREVIEWER"))
	if len(previewer) > 0 {
		return previewer[0], previewer[1:]
	}
	return defaultPreviewer, nil
}

func previewContent(content string, width int) (string, error) {
	cmd := previewCmd(width)
	cmd.Stdin = strings.NewReader(content)
	cmd.Env = previewEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", errPreviewerNotFound
	}
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return string(out), nil
}

func isMarkdownLanguage(language string) bool {
	switch strings.ToLower(language) {
	case "md", "markdown", "mdown", "mkd":
		return true
	default:
		return false
	}
}

func hasPreviewStyle(args []string) bool {
	for i, arg := range args {
		if arg == "-s" || arg == "--style" {
			return true
		}
		if strings.HasPrefix(arg, "--style=") {
			return true
		}
		if strings.HasPrefix(arg, "-s=") {
			return true
		}
		if arg == "-s" && i+1 < len(args) {
			return true
		}
	}
	return false
}

func filepathBase(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func previewEnv() []string {
	env := os.Environ()
	env = append(env, "CLICOLOR_FORCE=1")
	if os.Getenv("TERM") == "" {
		env = append(env, "TERM=xterm-256color")
	}
	if os.Getenv("COLORTERM") == "" {
		env = append(env, "COLORTERM=truecolor")
	}
	return env
}
