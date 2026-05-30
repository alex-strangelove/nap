package nap

import (
	"os"
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
)

type snippetSearchDoc struct {
	snippet       Snippet
	metadata      string
	metadataLower string
	contentLower  string
}

type snippetSearchSource struct {
	docs []snippetSearchDoc
}

func (s snippetSearchSource) String(i int) string {
	return s.docs[i].metadata
}

func (s snippetSearchSource) Len() int {
	return len(s.docs)
}

func buildSnippetSearchDocs(home string, snippets []Snippet) []snippetSearchDoc {
	sorted := append([]Snippet(nil), snippets...)
	sortSnippets(sorted)

	docs := make([]snippetSearchDoc, 0, len(sorted))
	for _, snippet := range sorted {
		content := readSnippetSearchContent(home, snippet)
		metadata := strings.TrimSpace(strings.Join([]string{
			snippet.Path(),
			snippet.Folder,
			snippet.Name,
			snippet.File,
			snippet.Language,
			strings.Join(snippet.Tags, " "),
		}, " "))
		docs = append(docs, snippetSearchDoc{
			snippet:       snippet,
			metadata:      metadata,
			metadataLower: strings.ToLower(metadata),
			contentLower:  strings.ToLower(content),
		})
	}

	return docs
}

func searchSnippetDocs(docs []snippetSearchDoc, query string) []Snippet {
	query = strings.TrimSpace(query)
	if query == "" {
		results := make([]Snippet, 0, len(docs))
		for _, doc := range docs {
			results = append(results, doc.snippet)
		}
		return results
	}

	queryLower := strings.ToLower(query)
	scores := map[int]int{}

	for idx, doc := range docs {
		if pos := strings.Index(doc.metadataLower, queryLower); pos >= 0 {
			if pos > 400 {
				pos = 400
			}
			scores[idx] += 2000 - pos + strings.Count(doc.metadataLower, queryLower)*25
		}
		if pos := strings.Index(doc.contentLower, queryLower); pos >= 0 {
			contentPenalty := pos / 4
			if contentPenalty > 250 {
				contentPenalty = 250
			}
			scores[idx] += 1200 - contentPenalty + strings.Count(doc.contentLower, queryLower)*20
		}
	}

	for _, match := range fuzzy.FindFrom(query, snippetSearchSource{docs: docs}) {
		scores[match.Index] += 700 + match.Score
	}

	if len(scores) == 0 {
		return nil
	}

	indexes := make([]int, 0, len(scores))
	for idx := range scores {
		indexes = append(indexes, idx)
	}

	sort.SliceStable(indexes, func(i, j int) bool {
		left, right := indexes[i], indexes[j]
		if scores[left] != scores[right] {
			return scores[left] > scores[right]
		}
		return docs[left].snippet.Path() < docs[right].snippet.Path()
	})

	results := make([]Snippet, 0, len(indexes))
	for _, idx := range indexes {
		results = append(results, docs[idx].snippet)
	}

	return results
}

func readSnippetSearchContent(home string, snippet Snippet) string {
	path, err := snippetStoragePath(home, snippet)
	if err != nil {
		return ""
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(content)
}
