package template

import "strings"

type rawAttr struct {
	name   string
	quoted bool
}

// scanAttrs lists the attributes of a start tag in source order, with whether each value was
// quoted. It follows the HTML tokenizer's rules, so it sees the attributes x/net/html sees.
func scanAttrs(tag []byte) []rawAttr {
	i := 1
	for i < len(tag) && !isHTMLSpace(tag[i]) && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	var attrs []rawAttr
	for {
		for i < len(tag) && (isHTMLSpace(tag[i]) || tag[i] == '/') {
			i++
		}
		if i >= len(tag) || tag[i] == '>' {
			return attrs
		}
		start := i
		i++
		for i < len(tag) && !isHTMLSpace(tag[i]) && tag[i] != '/' && tag[i] != '>' && tag[i] != '=' {
			i++
		}
		a := rawAttr{name: strings.ToLower(string(tag[start:i]))}
		j := i
		for j < len(tag) && isHTMLSpace(tag[j]) {
			j++
		}
		if j < len(tag) && tag[j] == '=' {
			j++
			for j < len(tag) && isHTMLSpace(tag[j]) {
				j++
			}
			if j < len(tag) && (tag[j] == '"' || tag[j] == '\'') {
				q := tag[j]
				j++
				for j < len(tag) && tag[j] != q {
					j++
				}
				a.quoted = true
				j++
			} else {
				for j < len(tag) && !isHTMLSpace(tag[j]) && tag[j] != '>' {
					j++
				}
			}
			i = j
		}
		attrs = append(attrs, a)
	}
}

func isHTMLSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r' }
