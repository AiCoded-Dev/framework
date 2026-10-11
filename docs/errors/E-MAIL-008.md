# E-MAIL-008: no mailbox on this target

The app read its mailbox: it listed, read, moved or deleted a message. Where the app runs now,
the platform sends its mail but keeps no mailbox for it yet; `aicoded dev` keeps one in memory.
Sending works as before.

**Fix:** only send mail on this target: keep `mailer.List`, `mailer.Get`, `mailer.Move` and `mailer.Delete` out of the paths the app takes there
