# E-REL-011: building block not offered where the app runs

The platform makes each app's database, file buckets, mailbox and route where the organisation's
apps run: at the target of the server the platform's operator placed the organisation on. That
target does not offer one of the building blocks the app's permission list declares, or the
platform cannot reach the target at all, and then it offers none. Rather than release an app that
would miss a building block, the platform refuses the release. Nothing was released.

A read of an app's spans or traces In your own cloud is refused with this code too: there, the
app's logs can be read, and its traces come later. So is an app that sends mail In your own cloud
when the customer's guest zone is set to receive the apps' mail (`Inbox`) while another of the
account's SES rule sets than `aicoded` is active: the platform does not change the customer's own
rule set. So is an app In your own cloud whose file stores, or whose domains in
`email.to_domains`, need more rights than one app's role in the cloud can hold: the platform
gives each app one role there, which holds a rule for each store and for each domain.

**Fix:** the one for the case the message names:

- a building block the target does not offer: remove it from aicoded.yaml and the code that uses it, run aicoded check, commit and publish again; or ask the platform's operator to place the organisation on a server whose target offers it, then release again;
- a target the platform cannot reach: ask the platform's operator to make it reachable, then release again;
- a read of traces In your own cloud: read the app's logs with aicoded logs instead, as its traces come later;
- the inbox In your own cloud while another SES rule set is active: the customer sets the stack's parameter `Inbox` to false, or makes `aicoded` the active rule set in SES, then release again;
- more file stores or mail domains than one role holds In your own cloud: remove file stores from aicoded.yaml and the code that uses them, or keep their files in fewer stores, or list fewer domains in `email.to_domains`; run aicoded check, commit and publish again.
