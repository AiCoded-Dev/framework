# E-GEN-015: expression syntax error

A value inside `{{ }}` or `{{$ }}`, or the value of `ssr:if` or `ssr:else-if`, is not a valid
expression: an operator lacks an operand, a bracket or `}}` is missing, a string has an invalid
escape, or a condition is written as a loop.

**Fix:** fix the expression inside `{{ }}`
