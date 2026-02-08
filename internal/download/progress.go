package download

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Progress tracks download progress with atomic operations for thread safety.
type Progress struct {
	status       atomic.Value // string
	bytesTotal   atomic.Int64
	bytesDone    atomic.Int64
	tracksTotal  atomic.Int32
	tracksDone   atomic.Int32
	tracksFailed atomic.Int32
	trackTitle   atomic.Value // string
	trackArtist  atomic.Value // string
	startedAt    atomic.Int64 // unix nano
}

// Snapshot is a point-in-time copy of progress state, safe for UI reads.
type Snapshot struct {
	Status      string
	BytesTotal  int64
	BytesDone   int64
	TracksTotal int
	TracksDone  int
	TracksFailed int
	TrackTitle  string
	TrackArtist string
	SpeedStr    string
	ETAStr      string
	Percent     float64
}

// NewProgress creates a new progress tracker.
func NewProgress() *Progress {
	p := &Progress{}
	p.status.Store("idle")
	p.trackTitle.Store("")
	p.trackArtist.Store("")
	p.startedAt.Store(time.Now().UnixNano())
	return p
}

// SetStatus updates the current status.
func (p *Progress) SetStatus(s string) {
	p.status.Store(s)
}

// AddBytes adds downloaded bytes.
func (p *Progress) AddBytes(n int64) {
	p.bytesDone.Add(n)
}

// SetBytesTotal sets the expected total bytes.
func (p *Progress) SetBytesTotal(n int64) {
	p.bytesTotal.Store(n)
}

// SetTrack sets the current track info.
func (p *Progress) SetTrack(title, artist string) {
	p.trackTitle.Store(title)
	p.trackArtist.Store(artist)
}

// SetTracksTotal sets the total number of tracks.
func (p *Progress) SetTracksTotal(n int) {
	p.tracksTotal.Store(int32(n))
}

// IncTracksDone increments completed tracks.
func (p *Progress) IncTracksDone() {
	p.tracksDone.Add(1)
}

// IncTracksFailed increments failed tracks.
func (p *Progress) IncTracksFailed() {
	p.tracksFailed.Add(1)
}

// ResetTimer resets the download speed timer.
func (p *Progress) ResetTimer() {
	p.startedAt.Store(time.Now().UnixNano())
	p.bytesDone.Store(0)
}

// Snap returns a thread-safe snapshot of current progress.
func (p *Progress) Snap() Snapshot {
	done := p.bytesDone.Load()
	total := p.bytesTotal.Load()
	started := p.startedAt.Load()

	elapsed := time.Since(time.Unix(0, started)).Seconds()
	speed := float64(0)
	if elapsed > 0.01 {
		speed = float64(done) / elapsed
	}

	pct := float64(0)
	if total > 0 {
		pct = float64(done) / float64(total) * 100
	}

	etaStr := ""
	if speed > 0 && total > done {
		remaining := float64(total-done) / speed
		m := int(remaining) / 60
		s := int(remaining) % 60
		etaStr = fmt.Sprintf("%d:%02d", m, s)
	}

	speedStr := ""
	if speed > 0 {
		speedStr = fmt.Sprintf("%.1f MB/s", speed/1048576)
	}

	title, _ := p.trackTitle.Load().(string)
	artist, _ := p.trackArtist.Load().(string)
	status, _ := p.status.Load().(string)

	return Snapshot{
		Status:       status,
		BytesTotal:   total,
		BytesDone:    done,
		TracksTotal:  int(p.tracksTotal.Load()),
		TracksDone:   int(p.tracksDone.Load()),
		TracksFailed: int(p.tracksFailed.Load()),
		TrackTitle:   title,
		TrackArtist:  artist,
		SpeedStr:     speedStr,
		ETAStr:       etaStr,
		Percent:      pct,
	}
}
