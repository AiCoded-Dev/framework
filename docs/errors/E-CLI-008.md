# E-CLI-008: sign-in timed out

`aicoded login` waits 5 minutes for you to finish the sign-in in the browser that it opens. With `--device`, or when no browser opens, it shows a code to enter in a browser on any device, and the code expires after the time the platform gives it, 10 minutes. The sign-in was not finished in that time, so `aicoded login` stopped and kept nothing. A browser page that answers for another sign-in, such as an old tab, is refused and does not end the wait. When the browser said that no organisation has that name, `--org` named none on the platform.

**Fix:** run `aicoded login` again, with the organisation's name in `--org`, and finish the sign-in in time, in the browser it opens or with the new code it shows.
