# E-MIG-005: a step of the migrations failed

A step failed while it was applied to the database: MySQL refused its statement, the statement waited more than 30 seconds for a table that another session used, it ran for more than 5 minutes, or another applier held the database's migrations for more than 60 seconds. The message names the step as `<publish>#<index>` and MySQL's error number and message. The steps before it stay applied and in the journal; the step and those after it are not, and the next try starts at it. On the platform the release stops and the version that runs keeps serving.

**Fix:** after a lock wait or a timeout, release again when the table is less busy; when a draft's statement fails, change the draft
