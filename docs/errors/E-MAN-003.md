# E-MAN-003: aicoded.yaml cannot be read

`aicoded.yaml` is not valid YAML, or a section has the wrong shape (for example a single
value where a list is expected), or it holds more than one YAML document: a `---` line below
the first document starts a second one. It may not use an anchor, an alias or a merge key
(`&name`, `*name` or `<<`): the message `aicoded.yaml uses an anchor, alias or merge key` points
at each, since a few such lines can stand for a very large file. `aicoded generate` also reports
it when it cannot rewrite the `access` or `services` section without changing the rest of the
file: the file must be one YAML document in block style, with lines that end in a line feed,
every top-level key at the start of a line, and one `access` and one `services` section.

```yaml
secrets: &names [api_key]
settings: *names            # wrong: an alias
settings: [api_key]         # right: written out in full
```

**Fix:** correct the file at the reported line. Lists are written as `[a, b]` or one `- item` per line.
For a second document, keep one YAML document: remove the `---` line and everything below it.
For an anchor, alias or merge key, write each value out in full: `aicoded.yaml` may not use `&name`, `*name` or `<<`.
