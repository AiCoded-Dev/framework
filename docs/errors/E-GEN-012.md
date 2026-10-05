# E-GEN-012: unknown gotype

A form field's `gotype` is not one the form fields can parse, or the fixed `value` of a radio
or checkbox input is not a value of its `gotype`, such as `value="x"` with `gotype="int"`. The
generator refuses it rather than emit code that cannot match what the browser sends.

**Fix:** use string, bool, int, int8–int64, uint, uint8–uint64, float32 or float64
