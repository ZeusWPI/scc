// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0

package sqlc

import (
	"time"
)

type Message struct {
	ID        int64
	Name      string
	Ip        string
	Message   string
	CreatedAt time.Time
}

type Spotify struct {
	ID         int64
	Title      string
	Artists    string
	SpotifyID  string
	DurationMs int64
	CreatedAt  time.Time
}
