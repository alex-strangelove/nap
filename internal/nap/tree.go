package nap

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

type folderTree struct {
	roots      []Folder
	children   map[Folder][]Folder
	parents    map[Folder]Folder
	depths     map[Folder]int
	snippets   map[Folder][]Snippet
	flashcards map[Folder][]Snippet
}

func ensureAncestorLists(lists map[Folder]*list.Model, height int, styles SnippetsBaseStyle) {
	if len(lists) == 0 {
		lists[Folder(defaultSnippetFolder)] = newList([]list.Item{}, height, styles)
		return
	}

	queue := make([]Folder, 0, len(lists))
	for folder := range lists {
		queue = append(queue, folder)
	}

	for len(queue) > 0 {
		folder := queue[0]
		queue = queue[1:]

		parent, ok := parentFolder(folder)
		if !ok {
			continue
		}

		if _, exists := lists[parent]; exists {
			continue
		}

		lists[parent] = newList([]list.Item{}, height, styles)
		queue = append(queue, parent)
	}
}

func buildFolderTree(lists map[Folder]*list.Model) folderTree {
	tree := folderTree{
		children:   make(map[Folder][]Folder),
		parents:    make(map[Folder]Folder),
		depths:     make(map[Folder]int),
		snippets:   make(map[Folder][]Snippet),
		flashcards: make(map[Folder][]Snippet),
	}

	folders := make([]Folder, 0, len(lists))
	for folder := range lists {
		folders = append(folders, folder)
	}

	slices.SortFunc(folders, func(left, right Folder) int {
		return compareIndexedLabels(filepath.Base(string(left)), filepath.Base(string(right)))
	})

	for _, folder := range folders {
		tree.depths[folder] = strings.Count(string(folder), "/")
		for _, item := range lists[folder].Items() {
			snippet, ok := item.(Snippet)
			if !ok {
				continue
			}
			if isFlashcardDeck(snippet) {
				tree.flashcards[folder] = append(tree.flashcards[folder], snippet)
			}
			if isHiddenFlashcardDeck(snippet) {
				continue
			}

			tree.snippets[folder] = append(tree.snippets[folder], snippet)
		}
		sortSnippets(tree.snippets[folder])
		sortSnippets(tree.flashcards[folder])

		parent, ok := parentFolder(folder)
		if !ok {
			tree.roots = append(tree.roots, folder)
			continue
		}

		tree.parents[folder] = parent
		tree.children[parent] = append(tree.children[parent], folder)
	}

	for parent := range tree.children {
		slices.SortFunc(tree.children[parent], func(left, right Folder) int {
			return compareIndexedLabels(filepath.Base(string(left)), filepath.Base(string(right)))
		})
	}

	return tree
}

func parentFolder(folder Folder) (Folder, bool) {
	parent := filepath.ToSlash(filepath.Dir(string(folder)))
	if parent == "." || parent == "" {
		return "", false
	}

	return Folder(parent), true
}

func ancestorFolders(folder Folder) []Folder {
	var ancestors []Folder

	for current, ok := parentFolder(folder); ok; current, ok = parentFolder(current) {
		ancestors = append(ancestors, current)
	}

	slices.Reverse(ancestors)
	return ancestors
}

func (t folderTree) visibleItems(expanded map[Folder]bool) []list.Item {
	items := make([]list.Item, 0, len(t.depths))

	var walk func(folder Folder)
	walk = func(folder Folder) {
		items = append(items, folder)
		if !expanded[folder] {
			return
		}

		for _, child := range t.orderedChildren(folder) {
			switch v := child.(type) {
			case Snippet:
				items = append(items, v)
			case Folder:
				walk(v)
			}
		}
	}

	for _, root := range t.roots {
		walk(root)
	}

	return items
}

func (t folderTree) firstItem(folder Folder) (list.Item, bool) {
	children := t.orderedChildren(folder)
	if len(children) == 0 {
		return nil, false
	}

	return children[0], true
}

func (t folderTree) parent(folder Folder) (Folder, bool) {
	parent, ok := t.parents[folder]
	return parent, ok
}

func (t folderTree) hasChildren(folder Folder) bool {
	return len(t.children[folder]) > 0 || len(t.snippets[folder]) > 0
}

func (t folderTree) orderedChildren(folder Folder) []list.Item {
	items := make([]list.Item, 0, len(t.snippets[folder])+len(t.children[folder]))
	for _, snippet := range t.snippets[folder] {
		items = append(items, snippet)
	}
	for _, child := range t.children[folder] {
		items = append(items, child)
	}
	slices.SortFunc(items, func(left, right list.Item) int {
		return compareIndexedLabels(treeItemLabel(left), treeItemLabel(right))
	})
	return items
}

func treeItemLabel(item list.Item) string {
	switch v := item.(type) {
	case Snippet:
		return v.Name
	case Folder:
		return filepath.Base(string(v))
	default:
		return ""
	}
}

func visibleFolderIndex(items []list.Item, target list.Item, parents map[Folder]Folder) int {
	if len(items) == 0 {
		return 0
	}

	if target != nil {
		for idx, item := range items {
			switch candidate := item.(type) {
			case Folder:
				if folder, ok := target.(Folder); ok && candidate == folder {
					return idx
				}
			case Snippet:
				if snippet, ok := target.(Snippet); ok && candidate.Path() == snippet.Path() {
					return idx
				}
			}
		}
	}

	for current := treeItemFolder(target); current != ""; current = parents[current] {
		for idx, item := range items {
			folder, ok := item.(Folder)
			if ok && folder == current {
				return idx
			}
		}

		if _, ok := parents[current]; !ok {
			break
		}
	}

	return 0
}

func treeItemFolder(item list.Item) Folder {
	switch v := item.(type) {
	case Folder:
		return v
	case Snippet:
		return Folder(v.Folder)
	default:
		return ""
	}
}

func isSameOrDescendantFolder(folder, candidate Folder) bool {
	if folder == "" || candidate == "" {
		return false
	}

	if folder == candidate {
		return true
	}

	return strings.HasPrefix(string(candidate), string(folder)+"/")
}

func isIndexSnippet(snippet Snippet) bool {
	switch snippet.Name {
	case "index", "01-index":
		return true
	default:
		return false
	}
}
