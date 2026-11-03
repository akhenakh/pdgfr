package tts

import (
	"regexp"
	"strings"
	"unicode"
)

// maxSentenceRunes bounds one synthesis call. A longer sentence is cut at a
// clause boundary, which keeps the wait before the first sound short; the
// model segments long input on its own, but only returns when it is done.
const maxSentenceRunes = 500

// abbrevEnd reports whether a fragment ends with a title or an abbreviation
// whose full stop does not end the sentence.
var abbrevEnd = regexp.MustCompile(`(?:^|[\s"«“(])(?:M|MM|Mmes?|Mlles?|Dr|Pr|Me|St|Ste|etc|cf|ex|av|bd|vol|no|n°|p|pp|fig|réf|éd|trad|tél|[A-Z])\.$`)

// SplitSentences splits text the way Brindille reads it: after . ! ? or …,
// followed by a capital, a digit, a quote or a bracket. Titles and initials
// ("M. Dupont", "J. R. R. Tolkien") stay with the following sentence. A very
// long sentence is cut at a comma or a semicolon so audio can start early.
func SplitSentences(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	rs := []rune(text)
	var parts []string
	start := 0
	for i := 0; i < len(rs); i++ {
		if !strings.ContainsRune(".!?…", rs[i]) {
			continue
		}
		j := i + 1
		for j < len(rs) && strings.ContainsRune("\"”’'»)›]", rs[j]) {
			j++
		}
		if j >= len(rs) || !unicode.IsSpace(rs[j]) {
			continue
		}
		k := j
		for k < len(rs) && unicode.IsSpace(rs[k]) {
			k++
		}
		if k >= len(rs) || !startsSentence(rs[k]) {
			continue
		}
		parts = append(parts, string(rs[start:j]))
		start = k
		i = k - 1
	}
	parts = append(parts, string(rs[start:]))

	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n := len(out); n > 0 && abbrevEnd.MatchString(out[n-1]) {
			out[n-1] += " " + p
		} else {
			out = append(out, splitLong(p)...)
		}
	}
	return out
}

// splitLong cuts a sentence longer than maxSentenceRunes at its last clause
// boundary, falling back to a word break and then to a hard cut.
func splitLong(s string) []string {
	rs := []rune(s)
	if len(rs) <= maxSentenceRunes {
		return []string{s}
	}
	var out []string
	for len(rs) > maxSentenceRunes {
		head := rs[:maxSentenceRunes]
		k := -1
		for i := len(head) - 1; i >= 0; i-- {
			if strings.ContainsRune(",;:—…", head[i]) && i+1 < len(head) && head[i+1] == ' ' {
				k = i + 1
				break
			}
		}
		if k <= 0 {
			k = lastIndexRuneSet(head, ' ')
		}
		if k <= 0 {
			k = len(head)
		}
		out = append(out, strings.TrimSpace(string(rs[:k])))
		rs = []rune(strings.TrimSpace(string(rs[k:])))
	}
	if len(rs) > 0 {
		out = append(out, string(rs))
	}
	return out
}

func startsSentence(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsDigit(r) || r == '"' || r == '“' || r == '«' || r == '('
}

func lastIndexRuneSet(rs []rune, r rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] == r {
			return i
		}
	}
	return -1
}
