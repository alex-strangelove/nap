package nap

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

type folderTree struct {
	roots         []Folder
	children      map[Folder][]Folder
	parents       map[Folder]Folder
	depths        map[Folder]int
	boundSnippets map[Folder]Snippet
}

type boundSnippetItem struct {
	parent  Folder
	snippet Snippet
}

func (b boundSnippetItem) FilterValue() string {
	return string(b.parent)
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
		children:      make(map[Folder][]Folder),
		parents:       make(map[Folder]Folder),
		depths:        make(map[Folder]int),
		boundSnippets: make(map[Folder]Snippet),
	}

	folders := make([]Folder, 0, len(lists))
	for folder := range lists {
		folders = append(folders, folder)
	}

	slices.Sort(folders)

	for _, folder := range folders {
		tree.depths[folder] = strings.Count(string(folder), "/")
		if snippet, ok := boundIndexSnippet(lists[folder]); ok {
			tree.boundSnippets[folder] = snippet
		}

		parent, ok := parentFolder(folder)
		if !ok {
			tree.roots = append(tree.roots, folder)
			continue
		}

		tree.parents[folder] = parent
		tree.children[parent] = append(tree.children[parent], folder)
	}

	for parent := range tree.children {
		slices.Sort(tree.children[parent])
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

		if snippet, ok := t.boundSnippet(folder); ok {
			items = append(items, boundSnippetItem{
				parent:  folder,
				snippet: snippet,
			})
		}

		for _, child := range t.children[folder] {
			walk(child)
		}
	}

	for _, root := range t.roots {
		walk(root)
	}

	return items
}

func (t folderTree) firstChild(folder Folder) (Folder, bool) {
	children := t.children[folder]
	if len(children) == 0 {
		return "", false
	}

	return children[0], true
}

func (t folderTree) boundSnippet(folder Folder) (Snippet, bool) {
	snippet, ok := t.boundSnippets[folder]
	return snippet, ok
}

func (t folderTree) parent(folder Folder) (Folder, bool) {
	parent, ok := t.parents[folder]
	return parent, ok
}

func (t folderTree) hasChildren(folder Folder) bool {
	return len(t.children[folder]) > 0 || t.hasBoundSnippet(folder)
}

func (t folderTree) hasBoundSnippet(folder Folder) bool {
	_, ok := t.boundSnippets[folder]
	return ok
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
			case boundSnippetItem:
				if snippet, ok := target.(boundSnippetItem); ok && candidate.parent == snippet.parent && candidate.snippet.Path() == snippet.snippet.Path() {
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
	case boundSnippetItem:
		return v.parent
	default:
		return ""
	}
}

func boundIndexSnippet(li *list.Model) (Snippet, bool) {
	if li == nil {
		return Snippet{}, false
	}

	for _, item := range li.Items() {
		snippet, ok := item.(Snippet)
		if !ok {
			continue
		}

		if isIndexSnippet(snippet) {
			return snippet, true
		}
	}

	return Snippet{}, false
}

func isIndexSnippet(snippet Snippet) bool {
	switch snippet.Name {
	case "index", "01-index":
		return true
	default:
		return false
	}
}
