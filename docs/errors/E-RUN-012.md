# E-RUN-012: secrets read too often

On the platform, each read of a secret goes to the platform's secrets store, which records it. The
runner bounds the reads of each app in each environment, all its instances and secrets together:
100 at once, then 20 a second, and 8 under way at once. Past either bound, the runner refuses the
read without asking the store: nothing was read. An app that reads a secret when it starts, or when
it first needs it, and keeps the value in memory never meets the bound.

**Fix:** read each secret once, when the app starts or first needs it, and keep the value in memory rather than reading it at every request; then call again
