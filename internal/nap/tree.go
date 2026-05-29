package nap

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

type folderTree struct {
	roots    []Folder
	children map[Folder][]Folder
	parents  map[Folder]Folder
	depths   map[Folder]int
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
		children: make(map[Folder][]Folder),
		parents:  make(map[Folder]Folder),
		depths:   make(map[Folder]int),
	}

	folders := make([]Folder, 0, len(lists))
	for folder := range lists {
		folders = append(folders, folder)
	}

	slices.Sort(folders)

	for _, folder := range folders {
		tree.depths[folder] = strings.Count(string(folder), "/")

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

func (t folderTree) parent(folder Folder) (Folder, bool) {
	parent, ok := t.parents[folder]
	return parent, ok
}

func (t folderTree) hasChildren(folder Folder) bool {
	return len(t.children[folder]) > 0
}

func visibleFolderIndex(items []list.Item, target Folder, parents map[Folder]Folder) int {
	if len(items) == 0 {
		return 0
	}

	for current := target; current != ""; current = parents[current] {
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
