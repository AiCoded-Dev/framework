// Package server serves the runner protocol to one app: runner.sock, and mysql.sock through
// mysqlproxy when the app declares sqldb. It checks everything the app sends against the
// protocol and the app's permission list: the protocol version, secret names, file names, mail
// and its recipients, calls to other apps and the size of spans. Only what passes reaches the
// providers, which keep the files, send the mail, hold the secrets and take the spans on the
// target the app runs on. A provider gives the app an error of the error catalogue with
// CodedError, which the app reads as the framework's own coded errors.
//
// aicoded dev, aicoded l7-runner and the platform's runner serve apps through it. Apps never
// import it; they reach their runner through the building blocks.
package server
