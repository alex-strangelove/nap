package nap

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/list"
)

func TestParseNameSupportsNestedFolders(t *testing.T) {
	folder, name, language, err := parseName("foo/bar/baz.go")
	if err != nil {
		t.Fatalf("parseName returned error: %v", err)
	}
	if folder != "foo/bar" || name != "baz" || language != "go" {
		t.Fatalf("unexpected parseName result: got (%q, %q, %q)", folder, name, language)
	}
}

func TestCLI(t *testing.T) {
	tmp := tmpHome(t)

	t.Run("stdin", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Logf("could not open pipe: %v", err)
			t.FailNow()
		}
		os.Stdin = r

		w.WriteString("foo bar baz")
		w.Close()
		runCLI([]string{"foo/bar/baz.go"})

		cfg := readConfig()
		snippets := readSnippets(cfg)

		if len(snippets) != 1 {
			t.Logf("snippet count is incorrect: got %d but want 1", len(snippets))
			t.FailNow()
		}

		if snippets[0].Folder != "foo/bar" || snippets[0].File != "baz.go" {
			t.Fatalf("nested snippet metadata mismatch: got folder=%q file=%q", snippets[0].Folder, snippets[0].File)
		}

		fn := filepath.Join(tmp, "foo", "bar", "baz.go")
		fi, err := os.Open(fn)
		if err != nil {
			t.Logf("could not open test file: %v", err)
			t.FailNow()
		}
		defer fi.Close()

		content, err := io.ReadAll(fi)
		if err != nil {
			t.Logf("could not read test file: %v", err)
			t.FailNow()
		}

		if string(content) != "foo bar baz" {
			t.Logf(`snippet is incorrect: got %q but want "foo bar baz"`, string(content))
			t.FailNow()
		}
	})

	t.Run("stdout", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Logf("could not open pipe: %v", err)
			t.FailNow()
		}
		os.Stdout = w
		runCLI([]string{"foo/bar/baz.go"})
		w.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			t.Log("could not read stdout")
			t.FailNow()
		}

		if string(out) != "foo bar baz" {
			t.Logf(`snippet is incorrect: got %q but want "foo bar baz"`, string(out))
			t.FailNow()
		}
	})

	t.Run("list", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Logf("could not open pipe: %v", err)
			t.FailNow()
		}
		os.Stdout = w
		runCLI([]string{"list"})
		w.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			t.Log("could not read stdout")
			t.FailNow()
		}

		if string(out) != "foo/bar/baz.go\n" {
			t.Logf(`snippet is incorrect: got %q but want "foo/bar/baz.go\n"`, string(out))
			t.FailNow()
		}
	})
}

func TestScan(t *testing.T) {
	tmp := tmpHome(t)

	cfg := readConfig()
	snippets := readSnippets(cfg)
	snippets = scanSnippets(cfg, snippets)
	initNum := len(snippets)

	tmpSnippetFolder := filepath.Join(tmp, "foo", "bar")
	tmpSnippet := filepath.Join(tmpSnippetFolder, "baz.go")
	if err := os.MkdirAll(tmpSnippetFolder, os.ModePerm); err != nil {
		t.Logf("could not create snippet folder: %v", err)
		t.FailNow()
	}
	if err := os.WriteFile(tmpSnippet, []byte("foo bar baz"), os.ModePerm); err != nil {
		t.Logf("could not create snippet: %v", err)
		t.FailNow()
	}

	if err := os.WriteFile(filepath.Join(tmpSnippetFolder, ".gitkeep"), []byte("hidden"), os.ModePerm); err != nil {
		t.Fatalf("could not create hidden file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, cfg.File), []byte("[]"), os.ModePerm); err != nil {
		t.Fatalf("could not rewrite snippets file: %v", err)
	}

	snippets = scanSnippets(cfg, snippets)
	if len(snippets) != initNum+1 {
		t.Logf("incorrect number of snippets after initial scanning: want %d but got %d", initNum+1, len(snippets))
		t.FailNow()
	}
	if snippets[len(snippets)-1].Folder != "foo/bar" {
		t.Fatalf("nested folder scan mismatch: got %q", snippets[len(snippets)-1].Folder)
	}

	if err := os.Remove(tmpSnippet); err != nil {
		t.Logf("could not remove snippet: %v", err)
		t.FailNow()
	}

	snippets = scanSnippets(cfg, snippets)
	if len(snippets) != initNum {
		t.Logf("incorrect number of snippets after follow-up scanning: want %d but got %d", initNum, len(snippets))
		t.FailNow()
	}
}

func TestSaveSnippetRejectsPathTraversal(t *testing.T) {
	tmpHome(t)
	cfg := readConfig()

	saveSnippet("secret", []string{"../../escape.go"}, cfg, readSnippets(cfg))

	snippets := readSnippets(cfg)
	if len(snippets) != 0 {
		t.Fatalf("expected no snippets to be saved for invalid path, got %d", len(snippets))
	}
}

func TestInitialInteractiveSelectionFallsBackToExistingRoot(t *testing.T) {
	config := newConfig()
	styles := DefaultStyles(config)
	lists := map[Folder]*list.Model{
		Folder("work/backend"): newList([]list.Item{
			Snippet{
				Name:     "handler",
				Folder:   "work/backend",
				File:     "handler.go",
				Language: "go",
			},
		}, 20, styles.Snippets.Focused),
	}

	ensureAncestorLists(lists, 20, styles.Snippets.Focused)
	tree := buildFolderTree(lists)

	currentFolder, currentItem := initialInteractiveSelection(State{}, lists, tree)
	if currentFolder != Folder("work") {
		t.Fatalf("current folder mismatch: got %q want %q", currentFolder, Folder("work"))
	}
	if folder, ok := currentItem.(Folder); !ok || folder != Folder("work") {
		t.Fatalf("current item mismatch: got %#v want folder %q", currentItem, Folder("work"))
	}
}

func TestInitialInteractiveSelectionUsesStoredSnippetWhenAvailable(t *testing.T) {
	config := newConfig()
	styles := DefaultStyles(config)
	lists := map[Folder]*list.Model{
		Folder("work"): newList([]list.Item{
			Snippet{
				Name:     "handler",
				Folder:   "work",
				File:     "handler.go",
				Language: "go",
			},
		}, 20, styles.Snippets.Focused),
	}

	tree := buildFolderTree(lists)

	currentFolder, currentItem := initialInteractiveSelection(State{
		CurrentFolder:  "work",
		CurrentSnippet: "handler.go",
	}, lists, tree)
	if currentFolder != Folder("work") {
		t.Fatalf("current folder mismatch: got %q want %q", currentFolder, Folder("work"))
	}
	snippet, ok := currentItem.(Snippet)
	if !ok || snippet.File != "handler.go" {
		t.Fatalf("current item mismatch: got %#v want snippet %q", currentItem, "handler.go")
	}
}

func TestReadConfigFlashcardsEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NAP_CONFIG", filepath.Join(tmp, "missing-config.yaml"))
	t.Setenv("NAP_FLASHCARDS_ENABLED", "false")
	t.Setenv("NAP_FLASHCARDS_COMMAND", "/usr/local/bin/hascard --daily")

	config := readConfig()
	if config.FlashcardsEnabled {
		t.Fatal("flashcards_enabled env override was not applied")
	}
	if config.FlashcardsCommand != "/usr/local/bin/hascard --daily" {
		t.Fatalf("flashcards_command env override mismatch: got %q", config.FlashcardsCommand)
	}
}

func TestReadConfigMarkdownStyleEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NAP_CONFIG", filepath.Join(tmp, "missing-config.yaml"))
	t.Setenv("NAP_MARKDOWN_STYLE", "tokyo-night")

	config := readConfig()
	if config.MarkdownStyle != "tokyo-night" {
		t.Fatalf("markdown_style env override mismatch: got %q", config.MarkdownStyle)
	}
}

func TestReadConfigEnablesFlashcardsByDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NAP_CONFIG", filepath.Join(tmp, "missing-config.yaml"))

	config := readConfig()
	if !config.FlashcardsEnabled {
		t.Fatal("flashcards should be enabled by default")
	}
}

func TestReadConfigDefaultsMarkdownStyleToAuto(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NAP_CONFIG", filepath.Join(tmp, "missing-config.yaml"))

	config := readConfig()
	if config.MarkdownStyle != defaultMarkdownStyle {
		t.Fatalf("default markdown style mismatch: got %q", config.MarkdownStyle)
	}
}

func tmpHome(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	if err := os.Setenv("NAP_HOME", tmp); err != nil {
		t.Log("could not set NAP_HOME")
		t.FailNow()
	}
	return tmp
}
