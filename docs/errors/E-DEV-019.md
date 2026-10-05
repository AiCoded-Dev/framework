# E-DEV-019: not logged in to the dev UI

The dev UI of `aicoded dev`, at `http://localhost:<port>`, shows your apps, their logs, traces and mail, and stops and starts them, so it lets in only your browser. When `aicoded dev` starts, it prints a login link that holds a secret token. Opening that link gives your browser a cookie that lets it in. The page was opened without that cookie, or the login link did not hold the token. The browser sends the cookie only with requests that start at the dev UI itself or outside the browser, so a login link clicked on another web page ends here too. `aicoded mcp` prints only the address of the dev UI; a second `aicoded dev` in the same folder prints the login link of the one that runs.

**Fix:** open the login link that `aicoded dev` printed when it started, from the terminal or by pasting it into the address bar.
