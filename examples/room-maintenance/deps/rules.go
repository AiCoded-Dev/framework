package deps

import (
	"unicode/utf8"

	"aicoded.dev/framework/auth"
)

// HotelOps is the role of the people who look after the rooms: they open every ticket and change
// its status.
const HotelOps = "hotel-ops"

// CanOpen reports whether viewer may open t: they reported it, or they have the role hotel-ops.
func CanOpen(viewer auth.Identity, t Ticket) bool {
	return viewer.Authenticated() && (viewer.Subject == t.Author || viewer.HasRole(HotelOps))
}

// CanEdit reports whether viewer may change the title and details of t: they reported it, and it
// is still open.
func CanEdit(viewer auth.Identity, t Ticket) bool {
	return viewer.Authenticated() && viewer.Subject == t.Author && t.Status == Open
}

// TextProblems returns what is wrong with a ticket's title and details, each without the spaces
// around it, or "" for a part with no problem.
func TextProblems(title, details string) (titleProblem, detailsProblem string) {
	switch {
	case title == "":
		titleProblem = "Say what is wrong."
	case utf8.RuneCountInString(title) > 100:
		titleProblem = "Use at most 100 characters."
	}
	switch {
	case details == "":
		detailsProblem = "Describe the problem."
	case utf8.RuneCountInString(details) > 2000:
		detailsProblem = "Use at most 2000 characters."
	}
	return titleProblem, detailsProblem
}
