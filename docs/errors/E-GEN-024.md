# E-GEN-024: value in an unquoted attribute

An attribute value with `{{ }}` is not in quotes. Without quotes, a space in the value starts a
new attribute, so a value could add attributes, such as event handlers, to the element.

**Fix:** put the attribute value in double quotes
