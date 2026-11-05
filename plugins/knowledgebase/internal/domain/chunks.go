package knowledge

import "strings"

// packBlocks retains paragraph and fenced-code boundaries when the whole block
// fits. Only a block larger than the embedding chunk envelope is split by runes.
// Every rune, including blank lines and fence delimiters, belongs to one chunk.
func packBlocks(text []rune) []int {
	ends := make([]int, 0, (len(text)+chunkRunes-1)/chunkRunes)
	start, packed := 0, 0
	for _, end := range blockEnds(text) {
		if end-start <= chunkRunes {
			packed = end
			continue
		}
		if packed > start {
			ends = append(ends, packed)
			start = packed
		}
		for end-start > chunkRunes {
			start += chunkRunes
			ends = append(ends, start)
		}
		packed = end
	}
	if packed > start {
		ends = append(ends, packed)
	}
	return ends
}

func blockEnds(text []rune) []int {
	ends := make([]int, 0)
	appendEnd := func(end int) {
		if end > 0 && (len(ends) == 0 || ends[len(ends)-1] != end) {
			ends = append(ends, end)
		}
	}
	var fence rune
	fenceLength := 0
	for start := 0; start < len(text); {
		end := start
		for end < len(text) && text[end] != '\n' {
			end++
		}
		if end < len(text) {
			end++
		}
		line := strings.TrimSpace(string(text[start:end]))
		marker, length, rest := fenceMarker(line)
		if fence != 0 {
			if marker == fence && length >= fenceLength && strings.TrimSpace(rest) == "" {
				fence, fenceLength = 0, 0
				appendEnd(end)
			}
		} else if marker != 0 {
			appendEnd(start)
			fence, fenceLength = marker, length
		} else if line == "" {
			appendEnd(end)
		} else if strings.HasPrefix(line, "#") {
			appendEnd(start)
		}
		start = end
	}
	appendEnd(len(text))
	return ends
}

func fenceMarker(line string) (rune, int, string) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0, ""
	}
	length := 0
	for length < len(line) && line[length] == line[0] {
		length++
	}
	if length < 3 {
		return 0, 0, ""
	}
	return rune(line[0]), length, line[length:]
}
