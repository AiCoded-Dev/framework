# E-GEN-013: inputs sharing a name disagree on gotype

Radio or checkbox inputs that share a name form one field, which has one Go type. These
inputs declare different `gotype`s, so the field's type is ambiguous.

**Fix:** give every input with this name the same `gotype`
