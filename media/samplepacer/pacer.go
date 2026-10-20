// Package samplepacer releases media samples (access units) in real time.
//
// The producer side pulls samples from a source faster than real time, in bursts. The pacer holds them for a
// configurable latency and hands them to a single consumer at the wall-clock time that corresponds to their decode
// timestamp. Two tracks (video and audio) are interleaved by decode time on the way out.
//
// It differs from media/pacer in what it solves: media/pacer recovers the sender's clock from the arrival times of
// opaque network packets. Here arrival is under our control and every sample carries its own timestamp, so the only
// job is holding samples back to a target latency and keeping the latency bounded when the source falls behind.
package samplepacer

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/eluv-io/errors-go"
)

// Track identifies the media track of an Item. The pacer interleaves the tracks by decode time.
type Track int

const (
	Video Track = iota
	Audio
	numTracks
)

// Item is one sample to pace. DTS and PTS are on one continuous time axis shared by both tracks.
type Item struct {
	Track   Track
	DTS     time.Duration
	PTS     time.Duration
	Sync    bool // a random access point (video keyframe); audio items are always sync
	Size    int  // payload size in bytes, for statistics
	Payload any
}

// Config configures a Pacer. Zero values select the defaults.
type Config struct {
	// Latency is the target delay between a sample's decode time and its release, i.e. the jitter buffer.
	Latency time.Duration
	// MaxBuffered bounds the decode-time span held in the pacer. Push blocks beyond it, which throttles a producer
	// that runs ahead. Default: Latency + 2 s.
	MaxBuffered time.Duration
	// MaxSkew bounds how far ahead of the other track a track is released. A video sample whose DTS is more than
	// MaxSkew beyond the last pushed audio sample waits for audio to catch up, unless audio has been silent for
	// StallTimeout. Default: 500 ms.
	MaxSkew time.Duration
	// StallTimeout is how long a track may push nothing before it stops gating the other track. Default: 3 s.
	StallTimeout time.Duration
	// LateTolerance is the lateness up to which a sample is still released immediately (a burst). Beyond it the
	// pacer skips forward to the next video keyframe and re-anchors, so the latency stays bounded. Default: Latency.
	LateTolerance time.Duration
	// Now is the clock. Default: time.Now.
	Now func() time.Time
	// Sleep waits until t or until wake is signalled or ctx is done. Default: a timer. Tests inject a fake clock.
	Sleep func(ctx context.Context, t time.Time, wake <-chan struct{})
}

func (c *Config) applyDefaults() {
	if c.Latency <= 0 {
		c.Latency = 500 * time.Millisecond
	}
	if c.MaxBuffered <= 0 {
		c.MaxBuffered = c.Latency + 2*time.Second
	}
	if c.MaxSkew <= 0 {
		c.MaxSkew = 500 * time.Millisecond
	}
	if c.StallTimeout <= 0 {
		c.StallTimeout = 3 * time.Second
	}
	if c.LateTolerance <= 0 {
		c.LateTolerance = c.Latency
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Sleep == nil {
		c.Sleep = timerSleep
	}
}

func timerSleep(ctx context.Context, t time.Time, wake <-chan struct{}) {
	d := time.Until(t)
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-wake:
	case <-ctx.Done():
	}
}

// Stats is a snapshot of the pacer's counters.
type Stats struct {
	Queued            int           `json:"queued"`              // items currently held
	QueuedDuration    time.Duration `json:"queued_duration"`     // decode-time span currently held
	MaxQueuedDuration time.Duration `json:"max_queued_duration"` // largest span ever held
	Delivered         uint64        `json:"delivered"`           // items handed to the consumer
	Late              uint64        `json:"late"`                // items delivered after their due time, within tolerance
	MaxLateness       time.Duration `json:"max_lateness"`        // largest lateness of a delivered item
	Dropped           uint64        `json:"dropped"`             // items skipped to catch up to a keyframe
	Skips             uint64        `json:"skips"`               // catch-up events (each drops one or more items)
	Underruns         uint64        `json:"underruns"`           // times the consumer found the queue empty at a due time
	Anchored          bool          `json:"anchored"`
	AnchorWall        time.Time     `json:"anchor_wall,omitempty"`
	AnchorDTS         time.Duration `json:"anchor_dts,omitempty"`
}

// ErrClosed is returned by Push after Close.
var ErrClosed = errors.Str("sample pacer closed")

// Pacer holds samples and releases them in real time. Push may be called from several goroutines; Run must be called
// exactly once and is the single consumer.
type Pacer struct {
	cfg Config

	mu       sync.Mutex
	notFull  *sync.Cond // signalled when the held span drops below MaxBuffered
	wake     chan struct{}
	queue    itemHeap
	closed   bool
	maxDTS   time.Duration // largest DTS pushed
	haveMax  bool
	lastPush [numTracks]trackPush
	anchored bool
	anchorAt time.Time
	anchorTs time.Duration
	pending  bool // an explicit Discontinuity is pending: the next release re-anchors
	stats    Stats
}

type trackPush struct {
	seen bool
	dts  time.Duration
	at   time.Time
}

// New creates a Pacer.
func New(cfg Config) *Pacer {
	cfg.applyDefaults()
	p := &Pacer{cfg: cfg, wake: make(chan struct{}, 1)}
	p.notFull = sync.NewCond(&p.mu)
	return p
}

// Push queues an item. It blocks while the held decode-time span exceeds MaxBuffered, until ctx is done or the pacer
// is closed. Items may arrive out of decode order across tracks; within a track they must be in decode order.
func (p *Pacer) Push(ctx context.Context, it Item) error {
	stop := context.AfterFunc(ctx, func() {
		p.mu.Lock()
		p.notFull.Broadcast()
		p.mu.Unlock()
	})
	defer stop()

	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.closed {
			return ErrClosed
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(p.queue) == 0 || p.spanLocked() < p.cfg.MaxBuffered {
			break
		}
		p.notFull.Wait()
	}
	heap.Push(&p.queue, it)
	if !p.haveMax || it.DTS > p.maxDTS {
		p.maxDTS = it.DTS
		p.haveMax = true
	}
	if it.Track >= 0 && it.Track < numTracks {
		p.lastPush[it.Track] = trackPush{seen: true, dts: it.DTS, at: p.cfg.Now()}
	}
	if span := p.spanLocked(); span > p.stats.MaxQueuedDuration {
		p.stats.MaxQueuedDuration = span
	}
	p.signalLocked()
	return nil
}

// spanLocked returns the decode-time span currently held: the largest pushed DTS minus the head's DTS.
func (p *Pacer) spanLocked() time.Duration {
	if len(p.queue) == 0 {
		return 0
	}
	return p.maxDTS - p.queue[0].DTS
}

func (p *Pacer) signalLocked() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Discontinuity announces that the next items belong to a new timeline segment: the next released item re-anchors
// the schedule instead of being judged against the previous anchor.
func (p *Pacer) Discontinuity() {
	p.mu.Lock()
	p.pending = true
	p.signalLocked()
	p.mu.Unlock()
}

// Close stops the pacer: Push returns ErrClosed and Run returns once the queue has drained.
func (p *Pacer) Close() {
	p.mu.Lock()
	p.closed = true
	p.notFull.Broadcast()
	p.signalLocked()
	p.mu.Unlock()
}

// Stats returns a snapshot of the counters.
func (p *Pacer) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.stats
	s.Queued = len(p.queue)
	s.QueuedDuration = p.spanLocked()
	s.Anchored = p.anchored
	s.AnchorWall = p.anchorAt
	s.AnchorDTS = p.anchorTs
	return s
}

// Run releases items to deliver in real time until ctx is done or the pacer is closed and drained. An error from
// deliver ends the run and is returned.
func (p *Pacer) Run(ctx context.Context, deliver func(Item) error) error {
	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			if p.closed {
				p.mu.Unlock()
				return nil
			}
			if p.anchored {
				p.stats.Underruns++
			}
			p.mu.Unlock()
			select {
			case <-p.wake:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		now := p.cfg.Now()
		head := p.queue[0]

		if wait, until := p.skewWaitLocked(head, now); wait {
			p.mu.Unlock()
			p.cfg.Sleep(ctx, until, p.wake)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		if !p.anchored || p.pending {
			p.anchorLocked(now, head.DTS)
		}
		due := p.anchorAt.Add(head.DTS - p.anchorTs + p.cfg.Latency)
		if now.Before(due) {
			p.mu.Unlock()
			p.cfg.Sleep(ctx, due, p.wake)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		late := now.Sub(due)
		if late > p.cfg.LateTolerance {
			if p.skipToKeyframeLocked(now) {
				p.mu.Unlock()
				continue
			}
		}

		item := heap.Pop(&p.queue).(Item)
		p.stats.Delivered++
		if late > 0 {
			p.stats.Late++
			if late > p.stats.MaxLateness {
				p.stats.MaxLateness = late
			}
		}
		p.notFull.Broadcast()
		p.mu.Unlock()

		if err := deliver(item); err != nil {
			return err
		}
	}
}

// anchorLocked fixes the schedule: the item with DTS ts is due at now + Latency.
func (p *Pacer) anchorLocked(now time.Time, ts time.Duration) {
	p.anchored = true
	p.pending = false
	p.anchorAt = now
	p.anchorTs = ts
}

// skewWaitLocked reports whether head must wait for the other track to catch up, and until when at the latest. A
// track that has never pushed does not gate the other for longer than StallTimeout from the first push of anything;
// a track that stopped pushing stops gating after StallTimeout.
func (p *Pacer) skewWaitLocked(head Item, now time.Time) (bool, time.Time) {
	if head.Track < 0 || head.Track >= numTracks {
		return false, time.Time{}
	}
	if p.spanLocked() >= p.cfg.MaxBuffered {
		// The queue holds its bound, so Push is blocking both producers and the track we would wait for cannot send
		// anything until something leaves. Waiting would stall every track for StallTimeout, release the backlog in a
		// burst and stall again, so the release rate collapses to one burst per StallTimeout. Releasing now is what
		// lets the track we are waiting for catch up.
		return false, time.Time{}
	}
	other := p.lastPush[1-head.Track]
	self := p.lastPush[head.Track]
	if !other.seen {
		// Nothing from the other track yet. Give it StallTimeout from our own first push before going ahead alone.
		if !self.seen || now.Sub(self.at) >= p.cfg.StallTimeout {
			return false, time.Time{}
		}
		return true, self.at.Add(p.cfg.StallTimeout)
	}
	if head.DTS-other.dts <= p.cfg.MaxSkew {
		return false, time.Time{}
	}
	stalledAt := other.at.Add(p.cfg.StallTimeout)
	if !now.Before(stalledAt) {
		return false, time.Time{}
	}
	return true, stalledAt
}

// skipToKeyframeLocked drops queued items up to, but excluding, the next video keyframe and re-anchors the schedule
// on it, so a source that fell behind resumes at the target latency instead of dragging the backlog along. Returns
// false when there is no later keyframe to skip to, in which case the caller releases the head as a late item.
func (p *Pacer) skipToKeyframeLocked(now time.Time) bool {
	// Look for a keyframe other than the head itself.
	found := false
	for i := range p.queue {
		it := p.queue[i]
		if it.Track == Video && it.Sync && it.DTS > p.queue[0].DTS {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	dropped := 0
	for len(p.queue) > 0 {
		head := p.queue[0]
		if head.Track == Video && head.Sync && dropped > 0 {
			break
		}
		heap.Pop(&p.queue)
		dropped++
	}
	p.stats.Dropped += uint64(dropped)
	p.stats.Skips++
	p.anchorLocked(now, p.queue[0].DTS)
	log.Warn("sample pacer skipped to keyframe to bound latency", "dropped", dropped, "latency", p.cfg.Latency)
	p.notFull.Broadcast()
	return true
}

// itemHeap orders items by DTS, then video before audio at equal times.
type itemHeap []Item

func (h itemHeap) Len() int { return len(h) }
func (h itemHeap) Less(i, j int) bool {
	if h[i].DTS != h[j].DTS {
		return h[i].DTS < h[j].DTS
	}
	return h[i].Track < h[j].Track
}
func (h itemHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *itemHeap) Push(x any)   { *h = append(*h, x.(Item)) }
func (h *itemHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = Item{}
	*h = old[:n-1]
	return it
}
