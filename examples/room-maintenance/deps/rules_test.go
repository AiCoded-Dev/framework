package deps

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/auth"
)

var (
	alice = auth.Identity{Subject: "alice", Roles: []string{"staff"}}
	bob   = auth.Identity{Subject: "bob", Roles: []string{"staff", "hr"}}
	dana  = auth.Identity{Subject: "dana", Roles: []string{"staff", HotelOps}}
)

func TestCanOpen(t *testing.T) {
	ticket := Ticket{Author: "alice", Status: Done}
	assert.True(t, CanOpen(alice, ticket), "the author opens their ticket")
	assert.False(t, CanOpen(bob, ticket), "staff never open someone else's")
	assert.False(t, CanOpen(auth.Identity{Subject: "Alice"}, ticket), "a login matches exactly")
	assert.False(t, CanOpen(auth.Identity{Subject: "alice "}, ticket), "a trailing space is part of the login")
	assert.True(t, CanOpen(dana, ticket), "hotel-ops open every ticket")
	assert.False(t, CanOpen(auth.Identity{}, Ticket{}), "no viewer opens nothing, not even a ticket with no author")
}

func TestCanEdit(t *testing.T) {
	ticket := Ticket{Author: "alice", Status: Open}
	assert.True(t, CanEdit(alice, ticket), "the author edits their open ticket")
	assert.False(t, CanEdit(alice, Ticket{Author: "alice", Status: InProgress}), "only while it is open")
	assert.False(t, CanEdit(alice, Ticket{Author: "alice", Status: Done}), "only while it is open")
	assert.False(t, CanEdit(bob, ticket), "staff never edit someone else's")
	assert.False(t, CanEdit(dana, ticket), "hotel-ops change the status, not the text")
	assert.False(t, CanEdit(auth.Identity{}, Ticket{Status: Open}), "no viewer edits nothing")
}

func TestFollowTickets(t *testing.T) {
	d := New()
	mine, theirs, all := d.FollowTickets(alice), d.FollowTickets(bob), d.FollowTickets(dana)
	defer mine.Close()
	defer theirs.Close()
	defer all.Close()
	d.TicketChanged("bob")
	assert.False(t, woken(mine), "a list never learns that another's ticket changed")
	assert.True(t, woken(theirs), "the author's list follows their tickets")
	assert.True(t, woken(all), "hotel-ops follow every ticket")
	d.TicketChanged("alice")
	assert.True(t, woken(mine))
	assert.False(t, woken(theirs))
	assert.True(t, woken(all))
}

// woken reports whether changes has a change waiting.
func woken(changes Changes) bool {
	select {
	case <-changes.Updates():
		return true
	default:
		return false
	}
}

func TestTextProblems(t *testing.T) {
	for _, c := range []struct{ title, details, titleProblem, detailsProblem string }{
		{"Leaking tap", "The bathroom tap drips.", "", ""},
		{"", "", "Say what is wrong.", "Describe the problem."},
		{strings.Repeat("é", 100), strings.Repeat("é", 2000), "", ""},
		{strings.Repeat("é", 101), strings.Repeat("é", 2001), "Use at most 100 characters.", "Use at most 2000 characters."},
	} {
		titleProblem, detailsProblem := TextProblems(c.title, c.details)
		assert.Equal(t, c.titleProblem, titleProblem, c.title)
		assert.Equal(t, c.detailsProblem, detailsProblem, c.details)
	}
}
