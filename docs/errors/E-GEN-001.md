# E-GEN-001: cannot parse the template

The generator could not read the HTML of the template at this line: a tag is never closed,
usually because of a missing quote or a stray `<`, or an end tag closes an element that is not
open. The generator refuses to guess the structure, because a misread page could put text and
values in the wrong place. For the same reason it refuses a tag with the same attribute twice,
markup inside `<iframe>`, `<noscript>`, `<noembed>`, `<noframes>`, `<xmp>` or `<plaintext>`,
and a self-closing `<script/>`, `<title/>`, `<textarea/>` or other element whose text is read
raw: browsers ignore the `/` there and read the rest of the page as the element's text.

**Fix:** fix the HTML at this line: close every quote and tag, write a stray `<` as `&lt;`, give
each attribute once, write `<title></title>` rather than `<title/>`, and put only text in
`<noscript>` and `<iframe>`
