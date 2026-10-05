# E-MAN-003: aicoded.yaml cannot be read

`aicoded.yaml` is not valid YAML, or a section has the wrong shape (for example a single
value where a list is expected), or it holds more than one YAML document: a `---` line below
the first document starts a second one. `aicoded generate` also reports it when it cannot
rewrite the `access` or `services` section without changing the rest of the file: the file must
be one YAML document in block style, with lines that end in a line feed, every top-level key at
the start of a line, and one `access` and one `services` section.

**Fix:** correct the file at the reported line. Lists are written as `[a, b]` or one `- item` per line.
For a second document, keep one YAML document: remove the `---` line and everything below it.
