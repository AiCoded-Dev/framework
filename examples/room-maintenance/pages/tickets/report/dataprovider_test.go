package report

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/web/form"

	"room-maintenance/deps"
)

func TestRoomOptions(t *testing.T) {
	room := func(n int) form.SelectOption[int] { return form.SelectOption[int]{Value: n, Label: strconv.Itoa(n)} }
	assert.Equal(t, []form.SelectOptionElement[int]{
		form.SelectOptionGroup[int]{Label: "Floor 1", Options: []form.SelectOptionElement[int]{room(101), room(102)}},
		form.SelectOptionGroup[int]{Label: "Floor 2", Options: []form.SelectOptionElement[int]{room(201)}},
	}, roomOptions([]deps.Room{{Number: 101, Floor: 1}, {Number: 102, Floor: 1}, {Number: 201, Floor: 2}}))
	assert.Empty(t, roomOptions(nil))
}
