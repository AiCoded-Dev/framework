package deps

import "context"

// The hotel's rooms: 101 to 108 on the first floor, up to 401 to 408 on the fourth.
const (
	floors        = 4
	roomsPerFloor = 8
)

// Room is a room of the hotel.
type Room struct {
	Number int
	Floor  int
}

// Rooms returns every room, by number.
func (d *Deps) Rooms(ctx context.Context) ([]Room, error) {
	rows, err := d.DB.QueryContext(ctx, "SELECT number, floor FROM rooms ORDER BY number")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rooms []Room
	for rows.Next() {
		var r Room
		if err := rows.Scan(&r.Number, &r.Floor); err != nil {
			return nil, err
		}
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}
