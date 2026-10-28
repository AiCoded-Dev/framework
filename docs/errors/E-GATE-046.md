# E-GATE-046: a page open to more people than before

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:`. A page of the app is open to more people than in the version that runs now: one of its access rules admits a role that the rule did not admit before, it lost a rule, it no longer has a `Guard`, or it became `shared`. Whatever the app type allows, such a change always needs an approver, since people who could not see the page's data now can. A new page is not more accessible: it had no one to compare with. See [app types](../guides/app-types.md).

```html
<!-- pages/rooms/index.html: staff can now open a page that only hotel-ops could -->
<ssr:access role="hotel-ops,staff"/>
```

**Fix:** an approver sees the page and who can now open it on the approval card; to stay Pre-approved, keep the page's access rules, Guard and shared as they were
