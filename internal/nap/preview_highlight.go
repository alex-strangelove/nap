package nap

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ansiResetBackground = "\x1b[49m"

type visibleRange struct {
	start int
	end   int
}

func highlightPreviewMatches(rendered, query string, config Config) string {
	query = strings.TrimSpace(query)
	if query == "" || rendered == "" {
		return rendered
	}

	ranges := visibleMatchRanges(rendered, query)
	if len(ranges) == 0 {
		return rendered
	}

	return applyVisibleHighlights(rendered, ranges, searchHighlightStart(config))
}

func visibleMatchRanges(rendered, query string) []visibleRange {
	visible := visibleRunes(rendered)
	queryRunes := []rune(query)
	if len(visible) == 0 || len(queryRunes) == 0 || len(queryRunes) > len(visible) {
		return nil
	}

	lowerVisible := lowerRunes(visible)
	lowerQuery := lowerRunes(queryRunes)

	var ranges []visibleRange
	for i := 0; i <= len(lowerVisible)-len(lowerQuery); {
		if runesEqual(lowerVisible[i:i+len(lowerQuery)], lowerQuery) {
			ranges = append(ranges, visibleRange{start: i, end: i + len(lowerQuery)})
			i += len(lowerQuery)
			continue
		}
		i++
	}

	return ranges
}

func visibleRunes(rendered string) []rune {
	var visible []rune
	for i := 0; i < len(rendered); {
		if seqLen := ansiSequenceLength(rendered[i:]); seqLen > 0 {
			i += seqLen
			continue
		}

		r, size := utf8.DecodeRuneInString(rendered[i:])
		visible = append(visible, r)
		i += size
	}
	return visible
}

func lowerRunes(src []rune) []rune {
	lowered := make([]rune, len(src))
	for i, r := range src {
		lowered[i] = unicode.ToLower(r)
	}
	return lowered
}

func runesEqual(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func applyVisibleHighlights(rendered string, ranges []visibleRange, start string) string {
	starts := make(map[int]int, len(ranges))
	ends := make(map[int]int, len(ranges))
	for _, r := range ranges {
		starts[r.start]++
		ends[r.end]++
	}

	var out strings.Builder
	out.Grow(len(rendered) + len(ranges)*(len(start)+len(ansiResetBackground)))

	visibleIndex := 0
	activeMatches := 0

	for i := 0; i < len(rendered); {
		if starts[visibleIndex] > 0 {
			for j := 0; j < starts[visibleIndex]; j++ {
				out.WriteString(start)
				activeMatches++
			}
			delete(starts, visibleIndex)
		}

		if seqLen := ansiSequenceLength(rendered[i:]); seqLen > 0 {
			seq := rendered[i : i+seqLen]
			out.WriteString(seq)
			if activeMatches > 0 && seq[len(seq)-1] == 'm' {
				out.WriteString(start)
			}
			i += seqLen
			continue
		}

		_, size := utf8.DecodeRuneInString(rendered[i:])
		out.WriteString(rendered[i : i+size])
		i += size
		visibleIndex++

		if ends[visibleIndex] > 0 {
			for j := 0; j < ends[visibleIndex]; j++ {
				activeMatches--
				out.WriteString(ansiResetBackground)
			}
			delete(ends, visibleIndex)
		}
	}

	for activeMatches > 0 {
		out.WriteString(ansiResetBackground)
		activeMatches--
	}

	return out.String()
}

func ansiSequenceLength(s string) int {
	if len(s) < 2 || s[0] != '\x1b' || s[1] != '[' {
		return 0
	}

	for i := 2; i < len(s); i++ {
		if s[i] >= 0x40 && s[i] <= 0x7e {
			return i + 1
		}
	}

	return 0
}

func searchHighlightStart(config Config) string {
	color := strings.TrimSpace(config.SearchHighlightColor)
	if color == "" {
		return ""
	}
	if strings.HasPrefix(color, "#") && len(color) == 7 {
		r, errR := strconv.ParseUint(color[1:3], 16, 8)
		g, errG := strconv.ParseUint(color[3:5], 16, 8)
		b, errB := strconv.ParseUint(color[5:7], 16, 8)
		if errR == nil && errG == nil && errB == nil {
			return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
		}
	}
	if index, err := strconv.Atoi(color); err == nil {
		return fmt.Sprintf("\x1b[48;5;%dm", index)
	}
	return ""
}
