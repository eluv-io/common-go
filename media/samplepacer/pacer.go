// Package samplepacer releases media samples (access units) in real time.
//
// The producer side pulls samples from a source faster than real time, in bursts. The pacer holds them for a
// configurable latency and hands them to a single consumer at the wall-clock time that corresponds to their decode
// timestamp. Two tracks (video and audio) are interleaved by decode time on the way out.
//
// It differs from media/pacer in what it solves: media/pacer recovers the sender's clock from the arrival times of
// opaque network packets. Here arrival is under our control and every sample carries its own timestamp, so the only
// job is holding samples back to a target latency and keeping the latency bounded when the source falls behind.
//
// The payload type is a type parameter rather than an interface, so a pooled payload travels through the queue
// without being boxed. Ownership follows the payload: an item the pacer accepted is owned by the pacer until it is
// delivered, and every item it accepted but will not deliver goes to Config.OnDrop instead. Push returning an error
// did not accept the item, so the caller keeps it.
package samplepacer

import (
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
type Item[P any] struct {
	Track   Track
	DTS     time.Duration
	PTS     time.Duration
	Sync    bool // a random access point (video keyframe); audio items are always sync
	Size    int  // payload size in bytes, for statistics
	Payload P
}

// Config configures a Pacer. Zero values select the defaults.
type Config[P any] struct {
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
	// OnDrop receives the payload of every item the pacer accepted but will not deliver: items skipped to catch up
	// to a keyframe, and items still queued when Run returns early or when a pacer that was never run is closed. It
	// is where a pooled payload is released. It runs under the pacer's lock and must not call back into the pacer.
	OnDrop func(P)
	// Now is the clock. Default: time.Now.
	Now func() time.Time
	// Sleep waits until t or until wake is signalled or ctx is done. Default: a timer reused across waits. Tests
	// inject a fake clock. It is only ever called from Run, so an implementation may keep state.
	Sleep func(ctx context.Context, t time.Time, wake <-chan struct{})
}

func (c *Config[P]) applyDefaults() {
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
}

// timerSleeper waits with one timer reused across waits, rather than a new one per wait: Run waits once per released
// sample. Reset and Stop are safe to use without draining the channel since Go 1.23.
type timerSleeper struct {
	t *time.Timer
}

func (s *timerSleeper) sleep(ctx context.Context, until time.Time, wake <-chan struct{}) {
	d := time.Until(until)
	if d <= 0 {
		return
	}
	if s.t == nil {
		s.t = time.NewTimer(d)
	} else {
		s.t.Reset(d)
	}
	select {
	case <-s.t.C:
	case <-wake:
		s.t.Stop()
	case <-ctx.Done():
		s.t.Stop()
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
	Dropped           uint64        `json:"dropped"`             // items accepted but not delivered (skipped or discarded)
	Skips             uint64        `json:"skips"`               // catch-up events (each drops one or more items)
	Underruns         uint64        `json:"underruns"`           // times the consumer found the queue empty at a due time
	Anchored          bool          `json:"anchored"`
	AnchorWall        time.Time     `json:"anchor_wall,omitempty"`
	AnchorDTS         time.Duration `json:"anchor_dts,omitempty"`
}

// ErrClosed is returned by Push after Close, and after Run returned: in both cases nothing will consume the queue
// any more, so the caller keeps the item it tried to push.
var ErrClosed = errors.Str("sample pacer closed")

// Pacer holds samples and releases them in real time. Push may be called from several goroutines; Run must be called
// at most once and is the single consumer.
type Pacer[P any] struct {
	cfg Config[P]

	mu       sync.Mutex
	notFull  chan struct{} // holds a token when an item left the queue, so a blocked Push can retry
	closedCh chan struct{} // closed once, to wake every blocked Push at once
	wake     chan struct{}
	queue    itemHeap[P]
	closed   bool
	running  bool
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
func New[P any](cfg Config[P]) *Pacer[P] {
	cfg.applyDefaults()
	if cfg.Sleep == nil {
		cfg.Sleep = (&timerSleeper{}).sleep
	}
	return &Pacer[P]{
		cfg:      cfg,
		wake:     make(chan struct{}, 1),
		notFull:  make(chan struct{}, 1),
		closedCh: make(chan struct{}),
	}
}

// Push queues an item. It blocks while the held decode-time span exceeds MaxBuffered, until ctx is done or the pacer
// is closed. Items may arrive out of decode order across tracks; within a track they must be in decode order.
//
// On success the pacer owns the item's payload. On error it does not: the caller still owns it.
func (p *Pacer[P]) Push(ctx context.Context, it Item[P]) error {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return ErrClosed
		}
		if err := ctx.Err(); err != nil {
			p.mu.Unlock()
			return err
		}
		if len(p.queue) == 0 || p.spanLocked() < p.cfg.MaxBuffered {
			p.pushLocked(it)
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()

		// The token is buffered, so a release between the unlock and the select is not missed.
		select {
		case <-p.notFull:
		case <-p.closedCh:
		case <-ctx.Done():
		}
	}
}

func (p *Pacer[P]) pushLocked(it Item[P]) {
	p.queue.push(it)
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
}

// spanLocked returns the decode-time span currently held: the largest pushed DTS minus the head's DTS.
func (p *Pacer[P]) spanLocked() time.Duration {
	if len(p.queue) == 0 {
		return 0
	}
	return p.maxDTS - p.queue[0].DTS
}

func (p *Pacer[P]) signalLocked() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// releasedLocked reports that an item left the queue, so one blocked Push may retry.
func (p *Pacer[P]) releasedLocked() {
	select {
	case p.notFull <- struct{}{}:
	default:
	}
}

// Discontinuity announces that the next items belong to a new timeline segment: the next released item re-anchors
// the schedule instead of being judged against the previous anchor.
func (p *Pacer[P]) Discontinuity() {
	p.mu.Lock()
	p.pending = true
	p.signalLocked()
	p.mu.Unlock()
}

// Close stops the pacer: Push returns ErrClosed, and Run returns once the queue has drained to the consumer. What is
// queued stays queued, so a Close before Run still delivers it.
//
// An owner that will not run the pacer at all must call Discard instead, or the payloads of the queued items are
// never handed back.
func (p *Pacer[P]) Close() {
	p.mu.Lock()
	p.closeLocked()
	p.mu.Unlock()
}

// Discard closes the pacer and hands every queued payload to OnDrop, for an owner that will not run it or whose Run
// has already returned.
func (p *Pacer[P]) Discard() {
	p.mu.Lock()
	p.closeLocked()
	p.dropQueuedLocked()
	p.mu.Unlock()
}

func (p *Pacer[P]) closeLocked() {
	if !p.closed {
		p.closed = true
		close(p.closedCh)
	}
	p.signalLocked()
}

// dropQueuedLocked hands every queued payload to OnDrop and empties the queue.
func (p *Pacer[P]) dropQueuedLocked() {
	if len(p.queue) == 0 {
		return
	}
	n := len(p.queue)
	for len(p.queue) > 0 {
		it := p.queue.pop()
		if p.cfg.OnDrop != nil {
			p.cfg.OnDrop(it.Payload)
		}
	}
	p.stats.Dropped += uint64(n)
}

// Stats returns a snapshot of the counters.
func (p *Pacer[P]) Stats() Stats {
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
// deliver ends the run and is returned. Whichever way it ends, the pacer is closed afterwards and anything still
// queued goes to OnDrop, so no accepted item is left without an owner.
func (p *Pacer[P]) Run(ctx context.Context, deliver func(Item[P]) error) error {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	err := p.run(ctx, deliver)

	p.mu.Lock()
	p.running = false
	p.closeLocked()
	p.dropQueuedLocked()
	p.mu.Unlock()
	return err
}

func (p *Pacer[P]) run(ctx context.Context, deliver func(Item[P]) error) error {
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

		item := p.queue.pop()
		p.stats.Delivered++
		if late > 0 {
			p.stats.Late++
			if late > p.stats.MaxLateness {
				p.stats.MaxLateness = late
			}
		}
		p.releasedLocked()
		p.mu.Unlock()

		if err := deliver(item); err != nil {
			return err
		}
	}
}

// anchorLocked fixes the schedule: the item with DTS ts is due at now + Latency.
func (p *Pacer[P]) anchorLocked(now time.Time, ts time.Duration) {
	p.anchored = true
	p.pending = false
	p.anchorAt = now
	p.anchorTs = ts
}

// skewWaitLocked reports whether head must wait for the other track to catch up, and until when at the latest. A
// track that has never pushed does not gate the other for longer than StallTimeout from the first push of anything;
// a track that stopped pushing stops gating after StallTimeout.
func (p *Pacer[P]) skewWaitLocked(head Item[P], now time.Time) (bool, time.Time) {
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
func (p *Pacer[P]) skipToKeyframeLocked(now time.Time) bool {
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
		it := p.queue.pop()
		if p.cfg.OnDrop != nil {
			p.cfg.OnDrop(it.Payload)
		}
		dropped++
	}
	p.stats.Dropped += uint64(dropped)
	p.stats.Skips++
	p.anchorLocked(now, p.queue[0].DTS)
	log.Warn("sample pacer skipped to keyframe to bound latency", "dropped", dropped, "latency", p.cfg.Latency)
	p.releasedLocked()
	return true
}
