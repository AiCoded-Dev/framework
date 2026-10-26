# E-MIG-001: the draft cannot be used

The next schema change of an app is its draft, `.aicoded/migration.sql`: SQL statements, each ended by `;`. A draft holds at most 64 KiB of UTF-8 text and at most 100 statements, and at least one statement besides comments. The draft travels with a publish, not in git: `aicoded publish` refuses a commit that holds `.aicoded/migration.sql`.

**Fix:** keep the draft to 64 KiB of UTF-8 and 100 statements, split a larger change over two publishes, delete a draft with no statement, and untrack a committed draft with `git rm --cached .aicoded/migration.sql`
