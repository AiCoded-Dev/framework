package node

import (
	"fmt"
	"html"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

// UnionRefs merges CollectVarRefs results in order, without duplicates. It never returns nil.
func UnionRefs(sets ...[]string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range sets {
		for _, name := range s {
			if !seen[name] {
				seen[name] = true
				result = append(result, name)
			}
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

// Node is a part of a parsed template that writes Go code.
type Node interface {
	WriteGoCode(buf *gobuf.GoBuf)
	// FilePos returns the //line directive of the node's template position.
	FilePos() string
	// CollectVarRefs returns the variables of reactive that the node's subtree refers to,
	// never nil.
	CollectVarRefs(reactive map[string]bool) []string
}

// WithChildren is a node that holds child nodes.
type WithChildren interface {
	Node
	AddChildren(children ...Node)
	LastChild() Node
	PopChild()
}

// BaseNode is a node's position: the template's base name and the line.
type BaseNode struct {
	File string
	Line int
}

func (n *BaseNode) FilePos() string {
	return fmt.Sprintf("//line %s:%d", n.File, n.Line)
}

// openMarker returns the start tag that marks a live site with its key for the browser runtime.
func openMarker(tag, key string) string {
	return "<" + tag + ` data-ssr-bind="` + html.EscapeString(key) + `">`
}
