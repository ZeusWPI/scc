// Package lyrics provides a way to work with both synced and plain lyrics
package lyrics

import (
	"time"

	"github.com/zeusWPI/scc/internal/pkg/db/dto"
)

// Lyrics is the common interface for different lyric types
type Lyrics interface {
	GetSong() dto.Song
	Previous(int) []Lyric
	Current() (Lyric, bool)
	Next() (Lyric, bool)
	Upcoming(int) []Lyric
}

// Lyric represents a single lyric line.
type Lyric struct {
	Text     string
	Duration time.Duration
}

// New returns a new object that implements the Lyrics interface
func New(song *dto.Song) Lyrics {
	if song.LyricsType == "synced" {
		return newLRC(song)
	}

	return newPlain(song)
}
