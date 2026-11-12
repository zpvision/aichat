package knowledge

import (
	"strings"
	"unicode"
)

// Character approximation: 2200 Unicode characters is roughly 400–700 tokens for these languages.
func Split(pages []Page, size, overlap int) []Chunk {
	out := []Chunk{}
	for _, p := range pages {
		r := []rune(p.Text)
		for start := 0; start < len(r); {
			end := start + size
			if end >= len(r) {
				end = len(r)
			} else {
				floor := start + size*2/3
				cut := end
				for cut > floor && !unicode.IsSpace(r[cut]) {
					cut--
				}
				if cut > floor {
					end = cut
				}
				for k := end - 1; k > floor; k-- {
					if r[k] == '\n' && r[k-1] == '\n' {
						end = k + 1
						break
					}
				}
				if end == start+size && !unicode.IsSpace(r[end]) {
					for end < len(r) && !unicode.IsSpace(r[end]) {
						end++
					}
				}
			}
			s := strings.TrimSpace(string(r[start:end]))
			if s != "" {
				out = append(out, Chunk{Text: s, Page: p.Number, Position: len(out)})
			}
			if end == len(r) {
				break
			}
			next := end - overlap
			if next <= start {
				next = end
			}
			for next < end && !unicode.IsSpace(r[next]) {
				next++
			}
			start = next
		}
	}
	return out
}
