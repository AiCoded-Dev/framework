# E-REL-035: the database provisioner In your own cloud refused

The organisation's apps run In your own cloud. The platform never connects to the guest zone's
database itself: a small function in the customer's account, the database provisioner, makes each
app's database and its two users. It refused to make or remove the app's database, or answered
names that are not the app's, so the app gets no database. The message says which step failed.
Nothing was released.

**Fix:** the customer looks at the database provisioner's log in their account, or updates the
guest zone's stack to the platform's template; then release again
