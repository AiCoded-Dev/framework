# E-CLI-001: no page for this topic or error code

`aicoded explain` and the `howto` tool of `aicoded mcp` print a page of the docs built into `aicoded`: a guide, a task recipe, the changelog for AI assistants or the page of an error code. The topic they were given names none. An error code is written `E-<AREA>-<NNN>`, such as `E-DEV-001`, and a guide or a task recipe is named by its path, such as `guides/overview` or `tasks/add-form`. This happens when a topic or a code was copied wrongly, or comes from another version of `aicoded`.

**Fix:** copy the topic or the code from the list that `aicoded explain` or `howto` gives with no topic, such as `guides/overview` or `E-DEV-001`.
