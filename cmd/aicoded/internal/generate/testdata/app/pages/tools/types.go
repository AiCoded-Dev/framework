package tools

// AddIn is what the page sends to add.
type AddIn struct {
	A      int    `json:"a"`
	B      int    `json:"b"`
	Note   string `json:"note,omitempty"`
	Secret string `json:"-"`
}

// AddOut is what add returns.
type AddOut struct {
	Sum int `json:"sum"`
}
