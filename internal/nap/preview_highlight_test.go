package nap

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHighlightPreviewMatchesPreservesVisibleText(t *testing.T) {
	cfg := newConfig()
	rendered := "alpha beta gamma"

	highlighted := highlightPreviewMatches(rendered, "beta", cfg)

	if visible := strings.Join(strings.Fields(stripANSIEscapes(highlighted)), " "); visible != rendered {
		t.Fatalf("visible text mismatch: got %q want %q", visible, rendered)
	}
	if !strings.Contains(highlighted, ansiResetBackground) {
		t.Fatalf("expected highlighted preview to include a background reset, got %q", highlighted)
	}
}

func TestHighlightPreviewMatchesReappliesBackgroundAcrossANSI(t *testing.T) {
	cfg := newConfig()
	start := searchHighlightStart(cfg)
	rendered := "\x1b[31mal\x1b[0m\x1b[32mpha\x1b[0m beta"

	highlighted := highlightPreviewMatches(rendered, "alpha", cfg)

	if visible := stripANSIEscapes(highlighted); visible != "alpha beta" {
		t.Fatalf("visible text mismatch: got %q want %q", visible, "alpha beta")
	}
	if count := strings.Count(highlighted, start); count < 3 {
		t.Fatalf("expected background reapplication across ansi segments, got %d in %q", count, highlighted)
	}
}

func TestSearchHighlightUsesDedicatedOrangeColor(t *testing.T) {
	cfg := newConfig()

	if cfg.SearchHighlightColor != "#FF8700" {
		t.Fatalf("expected orange search highlight color, got %q", cfg.SearchHighlightColor)
	}
}

func TestHighlightPreviewMatchesDoesNotBleedPastMatch(t *testing.T) {
	cfg := newConfig()
	start := searchHighlightStart(cfg)
	rendered := "\x1b[31mEvery\x1b[0m other words"

	highlighted := highlightPreviewMatches(rendered, "Every", cfg)

	highlightedIndexes := highlightedVisibleIndexes(highlighted, start)
	want := []int{0, 1, 2, 3, 4}
	if !slices.Equal(highlightedIndexes, want) {
		t.Fatalf("highlighted indexes mismatch: got %v want %v in %q", highlightedIndexes, want, highlighted)
	}
}

func stripANSIEscapes(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if seqLen := ansiSequenceLength(s[i:]); seqLen > 0 {
			i += seqLen
			continue
		}

		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func highlightedVisibleIndexes(rendered, start string) []int {
	var indexes []int
	visibleIndex := 0
	active := false
	for i := 0; i < len(rendered); {
		switch {
		case start != "" && strings.HasPrefix(rendered[i:], start):
			active = true
			i += len(start)
		case strings.HasPrefix(rendered[i:], ansiResetBackground):
			active = false
			i += len(ansiResetBackground)
		case ansiSequenceLength(rendered[i:]) > 0:
			i += ansiSequenceLength(rendered[i:])
		default:
			if active {
				indexes = append(indexes, visibleIndex)
			}
			_, size := utf8.DecodeRuneInString(rendered[i:])
			i += size
			visibleIndex++
		}
	}
	return indexes
}
