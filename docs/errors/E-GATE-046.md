# E-GATE-046: a page open to more people than before

This is a note: it does not stop the publish. It is a reason the release *Needs approval*: the publish passed, but it goes live only once an approver approved it, and `aicoded publish` and `aicoded status` list it under `Needs approval:`. Until the console exists, the platform's operator releases such a publish when your organisation asks. A page of the app is open to more people than in the version that runs now: one of its access rules admits a role that the rule did not admit before, it lost a rule, it no longer has a `Guard`, or it became `shared`. Whatever the app type allows, such a change always needs an approver, since people who could not see the page's data now can. A new page is not more accessible: it had no one to compare with. See [app types](../guides/app-types.md).

```html
<!-- pages/rooms/index.html: staff can now open a page that only hotel-ops could -->
<ssr:access role="hotel-ops,staff"/>
```

**Fix:** an approver sees the page and who can now open it on the approval card; to stay Pre-approved, keep the page's access rules, Guard and shared as they were
