// Package htmlutils holds element sets of the HTML syntax.
package htmlutils

// VoidElements have no end tag.
var VoidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"keygen": true,
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
}

// LiteralElements have text that is not parsed as markup.
var LiteralElements = map[string]bool{
	"iframe":    true,
	"noembed":   true,
	"noframes":  true,
	"noscript":  true,
	"plaintext": true,
	"script":    true,
	"style":     true,
	"xmp":       true,
}

// PreserveWhitespaceElements keep their whitespace.
var PreserveWhitespaceElements = map[string]bool{
	"pre":      true,
	"textarea": true,
}
