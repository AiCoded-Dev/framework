# E-GEN-026: value inside a raw-text element

A `{{ }}` or `{{$ }}` is inside `<iframe>`, `<noscript>`, `<noembed>`, `<noframes>`, `<xmp>` or
`<plaintext>`. The browser reads the text of these elements without decoding character
references, so no escaping can stop a value from ending the element and adding markup.

**Fix:** move the `{{ }}` out of `<iframe>`, `<noscript>`, `<noembed>`, `<noframes>`, `<xmp>` or `<plaintext>`
