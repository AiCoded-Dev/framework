# E-GEN-025: value in a tag or attribute name

A `{{ }}` is part of a tag name or an attribute name. A value there could turn the element into
another one or give it any attribute, and no escaper can make that safe.

**Fix:** use a fixed name; values go only in quoted attribute values and text
