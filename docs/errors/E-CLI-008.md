# E-CLI-008: sign-in timed out

`aicoded login` prints the address of the sign-in and waits 5 minutes for you to open it in a browser on this computer and finish the sign-in. With `--device` it shows a code to enter in a browser on any device instead, and the code expires after the time the platform gives it, 10 minutes. The sign-in was not finished in that time, so `aicoded login` stopped and kept nothing. A browser page that answers for another sign-in, such as an old tab, is refused and does not end the wait. When the browser said that no organisation has that name, `--org` named none on the platform.

**Fix:** run `aicoded login` again, with the organisation's name in `--org`, and finish the sign-in in time, at the address it prints or with the new code it shows; on a computer without a browser, use `--device`.
