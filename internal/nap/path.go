package nap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errInvalidSnippetPath = errors.New("snippet path must stay within NAP_HOME")

func resolveHomePath(home, relative string) (string, error) {
	root, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}

	target, err := filepath.Abs(filepath.Join(home, relative))
	if err != nil {
		return "", err
	}

	if target == root {
		return "", fmt.Errorf("%w: %q", errInvalidSnippetPath, relative)
	}

	prefix := root + string(os.PathSeparator)
	if !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("%w: %q", errInvalidSnippetPath, relative)
	}

	return target, nil
}

func snippetStoragePath(home string, snippet Snippet) (string, error) {
	return resolveHomePath(home, snippet.Path())
}

func validateSnippetRelativePath(relative string) error {
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) {
		return fmt.Errorf("%w: %q", errInvalidSnippetPath, relative)
	}
	parent := ".." + string(os.PathSeparator)
	if clean == ".." || strings.HasPrefix(clean, parent) {
		return fmt.Errorf("%w: %q", errInvalidSnippetPath, relative)
	}
	return nil
}
