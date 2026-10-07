# E-PUB-012: git is missing or too old

`aicoded publish` runs git to check the commit and to make its bundle, and needs git 2.31 or later, the first whose `git bundle create` reads the commits to send from its standard input. git is not on `PATH`, or it is older, as the message says.

**Fix:** install git 2.31 or later, and put it on `PATH`.
