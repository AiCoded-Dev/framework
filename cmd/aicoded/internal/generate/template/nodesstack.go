package template

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/htmlutils"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
)

// NodesStack holds the open elements, the root first.
type NodesStack []node.WithChildren

// Push opens n.
func (s *NodesStack) Push(n node.WithChildren) {
	*s = append(*s, n)
}

// Top returns the innermost open element.
func (s *NodesStack) Top() node.WithChildren {
	if len(*s) == 0 {
		return nil
	}
	return (*s)[len(*s)-1]
}

// topTag returns the tag of the innermost open element, or "" when it is not an HTML element.
func (s NodesStack) topTag() string {
	if el, ok := s.Top().(*node.HtmlElement); ok {
		return el.TagName
	}
	return ""
}

func (s NodesStack) isWhitespacePreserved() bool {
	for _, n := range s {
		if el, ok := n.(*node.HtmlElement); ok {
			if htmlutils.LiteralElements[el.TagName] || htmlutils.PreserveWhitespaceElements[el.TagName] {
				return true
			}
		}
	}
	return false
}
