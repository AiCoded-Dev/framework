# E-ORG-005: deletion stopped part way

The platform's operator asked `admin orgs delete`, and a step after the organisation was closed
failed: removing its hosts or secrets Hosted by us, its built apps and uploads, its telemetry, or
its rows. The message names the step. What the steps before it removed stays removed; the
organisation's rows go last, all at once, so none of them was deleted.

The organisation stays closed until the command is run again: no one can sign in to it or to its
apps, nothing of it is published, released or previewed, and its apps are stopped. Running the
same command again prints the plan, with what is left to remove by hand, and resumes the deletion
where it stopped.

**Fix:** run the same `admin orgs delete <slug>` again
