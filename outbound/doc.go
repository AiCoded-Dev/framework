// Package outbound calls outside services. [Client] returns an *http.Client whose requests go to
// the runner, which sends each one only to a host that egress in the permission list names:
//
//	egress:
//	  - api.weather.example
//	  - {host: api.partner.example, secret: partner_api_key, header: Authorization, prefix: "Bearer "}
//
// A host bound to a secret gets the secret's value, after the prefix, in the header of every
// request; the app never reads the key, and secrets.Get of it is E-RUN-015.
//
// Rules:
//
//   - Build each request with http.NewRequestWithContext and the context of the request or job it
//     serves; a request without the app's context is E-RUN-004.
//   - Only https to port 443 (E-OUT-002) of a host that egress lists (E-OUT-001). The platform's
//     own hosts and private networks are never reachable (E-OUT-003).
//   - A request body and a response body are at most 10 MiB each (E-OUT-004, E-OUT-005), and a
//     whole request takes at most 30 s (E-OUT-006). An instance runs at most 16 requests at once
//     (E-OUT-007).
//   - Redirects are not followed: a 3xx response comes back with its Location.
//   - Any other HTTP client, such as http.Get or an http.Client of the app's own, is E-LINT-009.
//
// A refusal comes back as a coded error, which the response's error wraps. A new outside service
// is a change of the permission list, and a new key binding always needs approval.
package outbound
