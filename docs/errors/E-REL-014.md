# E-REL-014: sending address cannot change yet

Each app sends from a mailbox of its own, whose address is the app's `email.from` when it was first
released with mail. For now that address cannot change: a release whose `email.from` names another
address is refused, and the app keeps running the release it had, with its mailbox. Changing an
app's sending address comes later. Nothing was released.

**Fix:** keep the app's `email.from` at the address the message names, commit, publish and release again
