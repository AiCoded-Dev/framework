package s_id

// Trace is one trace as the page shows it.
type Trace struct {
	ID       string
	Duration string
	Apps     string
	Total    float64 // the duration in milliseconds, for the spans' meters
	// Spans are in the order of their tree: each follows its parent, and siblings go by start.
	Spans []Span
}

// Span is one span as the page shows it.
type Span struct {
	Name     string
	App      string
	Depth    int    // how deep it lies in the tree, at most maxDepth
	Offset   string // when it started after the trace
	Duration string
	Length   float64 // the duration in milliseconds
	Error    string
	Attrs    []Attr
}

// Attr is an attribute of a span.
type Attr struct{ Key, Value string }
