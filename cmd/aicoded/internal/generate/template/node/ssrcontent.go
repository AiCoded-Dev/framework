package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &SsrContent{}

// SsrContent is <ssr:content/>, where the child page is written.
type SsrContent struct {
	BaseNode
	Default string
}

func (n *SsrContent) CollectVarRefs(_ map[string]bool) []string { return []string{} }

func (n *SsrContent) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn("if err := s.WriteChild(w); err != nil {\nreturn err\n}")
}
