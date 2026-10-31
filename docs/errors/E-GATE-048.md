# E-GATE-048: a new or changed key binding

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:` as `the key <secret> goes to <host> in <header>`, then `after "<prefix>"` when the binding has a prefix. An entry of `egress` binds a key to an outside service, and the version that runs now does not bind the same key to that host in the same header after the same prefix: the binding is new, the app has never been live, a host that was listed alone now has a key, or the binding's secret, header or prefix changed. Whatever the app type allows, such a change always needs an approver, since one of the company's keys now goes to a service in a new way. A binding removed, or a host that keeps no key, is no reason. See [outside services](../guides/outbound.md) and [app types](../guides/app-types.md).

```yaml
secrets: [partner_api_key]
egress:
  - {host: api.partner.example, secret: partner_api_key, header: Authorization, prefix: "Bearer "}
```

**Fix:** an approver sees which key goes to which service on the approval card; to stay Pre-approved, keep the bindings the live release has
