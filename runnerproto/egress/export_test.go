package egress

import (
	"net/http"
	"time"
)

// DNSTimeout bounds resolving a host.
const DNSTimeout = dnsTimeout

// Deadline returns the time c gives a whole request.
func Deadline(c *Client) time.Duration { return c.deadline }

// SetDeadline sets the time c gives a whole request.
func SetDeadline(c *Client, d time.Duration) { c.deadline = d }

// Control runs the check c's dialer makes on address just before it connects.
func Control(c *Client, address string) error { return c.control("tcp", address, nil) }

// Transport returns the transport c sends requests with.
func Transport(c *Client) *http.Transport { return c.transport }
