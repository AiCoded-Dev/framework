package reactive_test

import (
	"fmt"

	"aicoded.dev/framework/web/reactive"
)

// A Topic wakes the Subscribe hooks that watch one key, such as the open pages of one user. A
// subscription holds only the newest value, and Publish never blocks.
func ExampleTopic() {
	lastSeen := reactive.NewTopic[string, string]()

	sub := lastSeen.Subscribe("alice") // in Subscribe of the page /users/alice/info
	defer sub.Close()

	lastSeen.Publish("bob", "10:41") // a page of bob's: nothing for alice's pages
	lastSeen.Publish("alice", "10:42")
	lastSeen.Publish("alice", "10:43") // replaces 10:42, which no one read

	fmt.Println(<-sub.Updates()) // Subscribe passes it to state.SetLastSeen
	// Output: 10:43
}

// A Broadcast reaches every subscription, such as one for each open home page that counts the
// visitors online.
func ExampleBroadcast() {
	online := reactive.NewBroadcast[int]()
	a, b := online.Subscribe(), online.Subscribe() // two open home pages
	defer a.Close()

	online.Publish(online.TotalSubs())
	fmt.Println(<-a.Updates(), <-b.Updates())

	b.Close() // the second page closed
	online.Publish(online.TotalSubs())
	fmt.Println(<-a.Updates())
	// Output:
	// 2 2
	// 1
}
