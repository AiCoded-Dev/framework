# E-GEN-004: unknown `ssr:` tag

The template uses a tag with the `ssr:` prefix that the generator does not know. Tags with
this prefix are instructions to the generator, so an unknown one is a mistake, not markup to
pass through.

**Fix:** use access, var, call, content, assets, form, input, select, textarea or json
