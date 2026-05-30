package nap

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ansiResetBackground          = "\x1b[49m"
	ansiResetForeground          = "\x1b[39m"
	selectedSearchHighlightColor = "#FF00FF"
)

type visibleRange struct {
	start int
	end   int
}

type visibleHighlight struct {
	start int
	end   int
	style highlightStyle
}

type highlightStyle struct {
	start string
	end   string
}

func highlightPreviewMatches(rendered, query string, config Config, selectedIndex int) string {
	query = strings.TrimSpace(query)
	if query == "" || rendered == "" {
		return rendered
	}

	ranges := visibleMatchRanges(rendered, query)
	if len(ranges) == 0 {
		return rendered
	}

	defaultStyle := highlightStyle{
		start: searchHighlightStart(config),
		end:   ansiResetBackground,
	}
	selectedStyle := selectedSearchHighlightStyle(config)
	highlights := make([]visibleHighlight, 0, len(ranges))
	for i, r := range ranges {
		style := defaultStyle
		if i == selectedIndex {
			style = selectedStyle
		}
		highlights = append(highlights, visibleHighlight{
			start: r.start,
			end:   r.end,
			style: style,
		})
	}

	return applyVisibleHighlights(rendered, highlights)
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

func applyVisibleHighlights(rendered string, highlights []visibleHighlight) string {
	starts := make(map[int][]highlightStyle, len(highlights))
	ends := make(map[int][]highlightStyle, len(highlights))
	for _, highlight := range highlights {
		starts[highlight.start] = append(starts[highlight.start], highlight.style)
		ends[highlight.end] = append(ends[highlight.end], highlight.style)
	}

	var out strings.Builder
	out.Grow(len(rendered) + len(highlights)*16)

	visibleIndex := 0
	activeHighlights := make([]highlightStyle, 0, 1)
	activeSGR := make([]string, 0, 4)

	for i := 0; i < len(rendered); {
		if styles := starts[visibleIndex]; len(styles) > 0 {
			for _, style := range styles {
				out.WriteString(style.start)
				activeHighlights = append(activeHighlights, style)
			}
			delete(starts, visibleIndex)
		}

		if seqLen := ansiSequenceLength(rendered[i:]); seqLen > 0 {
			seq := rendered[i : i+seqLen]
			out.WriteString(seq)
			if seq[len(seq)-1] == 'm' {
				activeSGR = updateActiveSGR(activeSGR, seq)
				for _, style := range activeHighlights {
					out.WriteString(style.start)
				}
			}
			i += seqLen
			continue
		}

		_, size := utf8.DecodeRuneInString(rendered[i:])
		out.WriteString(rendered[i : i+size])
		i += size
		visibleIndex++

		if styles := ends[visibleIndex]; len(styles) > 0 {
			for _, style := range styles {
				out.WriteString(style.end)
			}
			if len(activeHighlights) >= len(styles) {
				activeHighlights = activeHighlights[:len(activeHighlights)-len(styles)]
			} else {
				activeHighlights = activeHighlights[:0]
			}
			for _, seq := range activeSGR {
				out.WriteString(seq)
			}
			for _, style := range activeHighlights {
				out.WriteString(style.start)
			}
			delete(ends, visibleIndex)
		}
	}

	for i := len(activeHighlights) - 1; i >= 0; i-- {
		out.WriteString(activeHighlights[i].styleEnd())
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
	return ansiColor(color, 48)
}

func selectedSearchHighlightStyle(config Config) highlightStyle {
	return highlightStyle{
		start: ansiColor(config.WhiteColor, 38) + ansiColor(selectedSearchHighlightColor, 48),
		end:   ansiResetForeground + ansiResetBackground,
	}
}

func (h highlightStyle) styleEnd() string {
	return h.end
}

func updateActiveSGR(active []string, seq string) []string {
	if seq == "\x1b[m" || seq == "\x1b[0m" {
		return active[:0]
	}
	return append(active, seq)
}

func ansiColor(color string, prefix int) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return ""
	}
	if strings.HasPrefix(color, "#") && len(color) == 7 {
		r, errR := strconv.ParseUint(color[1:3], 16, 8)
		g, errG := strconv.ParseUint(color[3:5], 16, 8)
		b, errB := strconv.ParseUint(color[5:7], 16, 8)
		if errR == nil && errG == nil && errB == nil {
			return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", prefix, r, g, b)
		}
	}
	if index, err := strconv.Atoi(color); err == nil {
		return fmt.Sprintf("\x1b[%d;5;%dm", prefix, index)
	}
	return ""
}
