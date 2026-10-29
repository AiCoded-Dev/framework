# E-REL-023: the operator cannot release what needs approval

The platform's operator asked for a release, or a rollback, whose decision is *Needs approval*. Such
a release goes live only when an approver of the app's app type approves its approval card in the
console, and the operator is never that approver: the operator releases only what is
*Pre-approved*. A rollback is decided against the app type as it is now, so one to an earlier
release that now *Needs approval* is refused the same way. Nothing was released, and the app keeps
running the release it had.

```
admin release <publish>            # refused: the publish needs approval
```

**Fix:** an R2 release goes live through an approval card in the console; to roll back, publish the earlier commit again
