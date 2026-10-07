# E-GATE-009: a symbolic link or special file in the commit

The commit holds a symbolic link, or another entry that is not a plain file or folder, at the path the message names. The delivery pipeline takes plain files and folders only, so that nothing in an app points outside it.

**Fix:** replace the link with a copy of the file it points to, or remove it, and commit.
