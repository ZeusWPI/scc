// Package song provides the functions to draw an overview of the song integration
package song

import (
	"context"
	"database/sql"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/zeusWPI/scc/internal/pkg/db"
	"github.com/zeusWPI/scc/internal/pkg/db/dto"
	"github.com/zeusWPI/scc/internal/pkg/lyrics"
	"github.com/zeusWPI/scc/pkg/config"
	"github.com/zeusWPI/scc/ui/components/stopwatch"
	"github.com/zeusWPI/scc/ui/view"
)

var (
	previousAmount = 5  // Amount of passed lyrics to show
	upcomingAmount = 10 // Amount of upcoming lyrics to show
)

type playing struct {
	song      *dto.Song
	progress  progress.Model
	stopwatch stopwatch.Model
	lyrics    lyrics.Lyrics
	previous  []string // Lyrics already sang
	current   string   // Current lyric
	upcoming  []string // Lyrics that are coming up
}

// Model represents the view model for song
type Model struct {
	db         *db.DB
	current    playing
	history    []string
	topSongs   []topStat
	topGenres  []topStat
	topArtists []topStat
	width      int
	height     int
}

// Msg triggers a song data update
// Required for the View interface
type Msg struct{}

type msgPlaying struct {
	current playing
}

type msgTop struct {
	topSongs   []topStat
	topGenres  []topStat
	topArtists []topStat
}

type msgLyrics struct {
	song      dto.Song
	previous  []string
	current   string
	upcoming  []string
	startNext time.Time
	done      bool
}

type topStat struct {
	name   string
	amount int
}

// NewModel initializes a new song model
func NewModel(db *db.DB) view.View {
	// Get history, afterwards it gets updated when a new currentSong is detected
	history, _ := db.Queries.GetSongHistory(context.Background())

	return &Model{
		db:         db,
		current:    playing{stopwatch: stopwatch.New(), progress: progress.New()},
		history:    history,
		topSongs:   make([]topStat, 0, 5),
		topGenres:  make([]topStat, 0, 5),
		topArtists: make([]topStat, 0, 5),
		width:      0,
		height:     0,
	}
}

// Init starts the song view
func (m *Model) Init() tea.Cmd {
	return m.current.stopwatch.Init()
}

// Name returns the name of the view
func (m *Model) Name() string {
	return "Songs"
}

// Update updates the song view
func (m *Model) Update(msg tea.Msg) (view.View, tea.Cmd) {
	switch msg := msg.(type) {
	case view.MsgSize:
		entry, ok := msg.Sizes[m.Name()]
		if ok {
			m.width = entry.Width
			m.height = entry.Height
		}

		return m, nil
	case msgPlaying:
		m.history = append(m.history, msg.current.song.Title)
		if len(m.history) > 5 {
			m.history = m.history[1:]
		}

		m.current = msg.current
		// New song, start the commands to update the lyrics
		lyric, ok := m.current.lyrics.Next()
		if !ok {
			// Song already done
			m.current = playing{song: nil}
			return m, m.current.stopwatch.Reset()
		}

		// Go through the lyrics until we get to the current one
		startTime := m.current.song.CreatedAt.Add(lyric.Duration)
		for startTime.Before(time.Now()) {
			lyric, ok := m.current.lyrics.Next()
			if !ok {
				// We're too late to display lyrics
				m.current = playing{song: nil}
				return m, m.current.stopwatch.Reset()
			}
			startTime = startTime.Add(lyric.Duration)
		}

		m.current.upcoming = lyricsToString(m.current.lyrics.Upcoming(upcomingAmount))
		return m, tea.Batch(updateLyrics(m.current, startTime), m.current.stopwatch.Start(time.Since(m.current.song.CreatedAt)))

	case msgTop:
		if msg.topSongs != nil {
			m.topSongs = msg.topSongs
		}
		if msg.topGenres != nil {
			m.topGenres = msg.topGenres
		}
		if msg.topArtists != nil {
			m.topArtists = msg.topArtists
		}

		return m, nil

	case msgLyrics:
		// Check if it's still relevant
		if msg.song.ID != m.current.song.ID {
			// We already switched to a new song
			return m, nil
		}

		if msg.done {
			// Song has finished. Reset variables
			m.current = playing{song: nil}
			return m, m.current.stopwatch.Reset()
		}

		// Msg is relevant, update values
		m.current.previous = msg.previous
		m.current.current = msg.current
		m.current.upcoming = msg.upcoming

		// Start the cmd to update the lyrics
		return m, updateLyrics(m.current, msg.startNext)

	case progress.FrameMsg:
		progressModel, cmd := m.current.progress.Update(msg)
		m.current.progress = progressModel.(progress.Model)

		return m, cmd
	}

	var cmd tea.Cmd
	m.current.stopwatch, cmd = m.current.stopwatch.Update(msg)

	return m, cmd
}

// View draws the song view
func (m *Model) View() string {
	if m.current.song != nil {
		return m.viewPlaying()
	}

	return m.viewNotPlaying()

}

// GetUpdateDatas gets all update functions for the song view
func (m *Model) GetUpdateDatas() []view.UpdateData {
	return []view.UpdateData{
		{
			Name:     "update current song",
			View:     m,
			Update:   updateCurrentSong,
			Interval: config.GetDefaultInt("tui.song.interval_current_s", 5),
		},
		{
			Name:     "top stats",
			View:     m,
			Update:   updateTopStats,
			Interval: config.GetDefaultInt("tui.song.interval_top_s", 3600),
		},
	}
}

func updateCurrentSong(view view.View) (tea.Msg, error) {
	m := view.(*Model)

	songs, err := m.db.Queries.GetLastSongFull(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return nil, err
	}
	if len(songs) == 0 {
		return nil, nil
	}

	// Check if song is still playing
	if songs[0].CreatedAt.Add(time.Duration(songs[0].DurationMs) * time.Millisecond).Before(time.Now()) {
		// Song is finished
		return nil, nil
	}

	if m.current.song != nil && songs[0].ID == m.current.song.ID {
		// Song is already set to current
		return nil, nil
	}

	song := dto.SongDTOHistory(songs)

	return msgPlaying{current: playing{song: song, lyrics: lyrics.New(song)}}, nil
}

func updateTopStats(view view.View) (tea.Msg, error) {
	m := view.(*Model)
	msg := msgTop{}
	change := false

	songs, err := m.db.Queries.GetTopSongs(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if !equalTopSongs(m.topSongs, songs) {
		msg.topSongs = topStatSqlcSong(songs)
		change = true
	}

	genres, err := m.db.Queries.GetTopGenres(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if !equalTopGenres(m.topGenres, genres) {
		msg.topGenres = topStatSqlcGenre(genres)
		change = true
	}

	artists, err := m.db.Queries.GetTopArtists(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if !equalTopArtists(m.topArtists, artists) {
		msg.topArtists = topStatSqlcArtist(artists)
		change = true
	}

	if !change {
		return nil, nil
	}

	return msg, nil
}

func updateLyrics(song playing, start time.Time) tea.Cmd {
	timeout := time.Duration(0)
	now := time.Now()
	if start.After(now) {
		timeout = start.Sub(now)
	}

	return tea.Tick(timeout, func(_ time.Time) tea.Msg {
		// Next lyric
		lyric, ok := song.lyrics.Next()
		if !ok {
			// Song finished
			return msgLyrics{song: *song.song, done: true}
		}

		previous := song.lyrics.Previous(previousAmount)
		upcoming := song.lyrics.Upcoming(upcomingAmount)

		end := start.Add(lyric.Duration)

		return msgLyrics{
			song:      *song.song,
			previous:  lyricsToString(previous),
			current:   lyric.Text,
			upcoming:  lyricsToString(upcoming),
			startNext: end,
			done:      false,
		}
	})
}
