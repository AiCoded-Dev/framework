package node

// Esc names the escaper generated code uses for an expression's value.
type Esc uint8

// Escapers, one per HTML context.
const (
	EscText Esc = iota
	EscAttr
	EscURL
	EscURLPart
	EscURLPathStart
	EscURLQuery
)

// Func returns the render function for e.
func (e Esc) Func() string {
	switch e {
	case EscAttr:
		return "render.Attr"
	case EscURL:
		return "render.URL"
	case EscURLPart:
		return "render.URLPart"
	case EscURLPathStart:
		return "render.URLPathStart"
	case EscURLQuery:
		return "render.URLQuery"
	}
	return "render.Text"
}
