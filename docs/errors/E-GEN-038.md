# E-GEN-038: URL scheme depends on a value

A URL attribute has a `{{ }}` that can decide the URL's scheme: the value is not the whole
attribute, and the fixed text before it has no `/`, `?`, `#` or scheme followed by `:`, as in
`href="java{{ x }}"` or `href="{{ proto }}://{{ host }}"`. A value that is the whole attribute
has its scheme checked when the page is written; a value after a fixed path or scheme is escaped
as part of the URL.

**Fix:** make the value the whole attribute, or start it with a fixed path or scheme such as `/`, `?` or `https://`
