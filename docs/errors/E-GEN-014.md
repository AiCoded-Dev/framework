# E-GEN-014: invalid or repeated field name

A form field has a name that is not made of letters, digits and `_` starting with a letter, or
it shares its name with another field of the form. Only radio or checkbox inputs of one type may
share a name, because they send one value or a set of values; any other repeat would make two
fields read the same value.

**Fix:** name fields with letters and digits; only checkbox and radio inputs may share a name
