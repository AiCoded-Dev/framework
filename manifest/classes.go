package manifest

import (
	"slices"
	"strings"
)

// Classes are the classes of data a source may hold, from the least sensitive to the most.
var Classes = []string{"public", "internal", "confidential", "personal"}

// ReservedClasses are the classes of data no app may hold: health, HR and card data. A
// permission list that names one is not allowed (E-MAN-024).
var ReservedClasses = []string{"health", "hr", "card"}

// ClassRank returns the place of class in Classes, and false for any other name, a reserved
// class included.
func ClassRank(class string) (int, bool) {
	i := slices.Index(Classes, class)
	return i, i >= 0
}

// Admits reports whether the audience's internal entries admit a member of the groups, given
// by name without "group:": everyone does, and so does group:<n> for any n in groups. External
// entries admit no one here.
func (a Audience) Admits(groups []string) bool {
	if slices.Contains(a.Internal, "everyone") {
		return true
	}
	return slices.ContainsFunc(groups, func(g string) bool {
		return g != "" && slices.Contains(a.Internal, "group:"+g)
	})
}

// classList is Classes written for people: "public, internal, confidential or personal".
var classList = strings.Join(Classes[:len(Classes)-1], ", ") + " or " + Classes[len(Classes)-1]
