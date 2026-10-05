package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &SsrAssets{}

// SsrAssets is <ssr:assets/>, where the page's script and style tags are written.
type SsrAssets struct {
	BaseNode
}

func (n *SsrAssets) CollectVarRefs(_ map[string]bool) []string { return []string{} }

func (n *SsrAssets) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn("if err := s.WriteAssets(w); err != nil {\nreturn err\n}")
}
