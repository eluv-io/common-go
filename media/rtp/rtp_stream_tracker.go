package rtp

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// StreamTracker is a component that validates and tracks RTP stream. It checks RTP packets for errors (header
// format, sequence gaps, timestamp inconsistencies) and collects corresponding statistics. It optionally logs
// statistics at a specified interval.
type StreamTracker interface {
	// Track feeds RTP packets to the tracker. The packet bytes (bts) should consist of a single RTP packet. The method
	// will validate the packet and aggregate any errors. The method returns the payload and the adjusted wall clock
	// timestamp if the header is well-formatted, nil otherwise, and a list of errors if any.
	Track(bts []byte) (payload []byte, timestamp utc.UTC, errList error)
	// Stats returns RTP statistics
	Stats() *Stats
	// Reset resets the tracker state, clearing all statistics and errors.
	Reset()
}

// NewStreamTracker creates a tracker for an RTP stream.
func NewStreamTracker(streamId string, statsLogPeriod time.Duration, sequenceThreshold int64, timestampThreshold time.Duration) StreamTracker {
	tracker := &rtpStreamTracker{
		streamId:    streamId,
		statsLogger: NoopPeriodic{},
		logThrottle: timeutil.NewPeriodic(10 * time.Second),
		detector:    NewRtpGapDetector(sequenceThreshold, timestampThreshold),
	}
	if statsLogPeriod > 0 {
		tracker.statsLogger = timeutil.NewPeriodic(statsLogPeriod)
	}
	return tracker
}

type rtpStreamTracker struct {
	streamId    string
	statsLogger timeutil.Periodic
	stats       Stats
	logThrottle timeutil.Periodic
	panics      int
	detector    *GapDetector
	currT0      utc.UTC
	origT0      utc.UTC
}

func (t *rtpStreamTracker) Track(bts []byte) (payload []byte, timestamp utc.UTC, errList error) {

	defer func() {
		t.statsLogger.Do(func() {
			statsLog.Info("ts-stream-tracker", "stream", t.streamId, "stats", jsonutil.Stringer(t.Stats()))
		})
	}()

	appendErr := func(err error) {
		errList = errors.Append(errList, err)
		t.stats.ErrorCount++
	}

	t.stats.PacketCount++

	pkt, err := ParsePacket(bts)
	if err != nil {
		appendErr(err)
		return nil, utc.Zero, errList
	}

	seq, ts, err := t.detector.Detect(pkt.SequenceNumber, pkt.Timestamp)
	if err != nil {
		appendErr(err)
		t.stats.Gaps = append(t.stats.Gaps, Gap{
			PacketNum: t.stats.PacketCount,
			Seq:       seq,
			SeqPrev:   t.detector.Sequence.Previous(),
			SeqDiff:   seq - t.detector.Sequence.Previous(),
			Ts:        ts,
			TsPrev:    t.detector.Timestamp.Previous(),
			TsDiff:    ts - t.detector.Timestamp.Previous(),
		})
	}
	if t.stats.Start.IsZero() {
		t.stats.Start = utc.Now()
		t.stats.StartSeq = seq
		t.stats.StartTs = ts
	}
	t.stats.EndSeq = seq
	t.stats.EndTs = ts

	return pkt.Payload, t.toWallClockTS(ts), errList
}

func (t *rtpStreamTracker) Stats() *Stats {
	res := t.stats
	res.Duration = duration.Spec(utc.Since(t.stats.Start)).Round()
	res.RtpDuration = duration.Spec(TicksToDuration(t.stats.EndTs - t.stats.StartTs)).Round()
	return &res
}

func (t *rtpStreamTracker) Reset() {
	t.stats = Stats{}
}

func (t *rtpStreamTracker) toWallClockTS(rtpTS int64) utc.UTC {
	// A RTP timestamp is the number of ticks since the stream start, which is designated as RTP timestamp zero.
	// The wall clock timestamp of stream start, called T0, is calculated by subtracting a RTP timestamp (converted to
	// a time duration) from the current wall clock time. So, the calculated T0 depends on the receive time of a packet.
	// To improve the approximation of T0, calculate T0 for each new packet received and take the earliest T0 found.
	// The wall clock timestamp for a given packet is then calculated by adding the RTP timestamp (converted to a time
	// duration) to the current T0.
	rtpDur := TicksToDuration(rtpTS)
	t0 := utc.Now().Add(rtpDur * -1)
	if t.origT0.IsZero() {
		t.currT0 = t0
		t.origT0 = t0
	} else if t0.Before(t.currT0) {
		t.currT0 = t0
		t.stats.TsAdjCount++
		t.stats.TsAdjDuration = duration.Spec(t.origT0.Sub(t.currT0))
	}
	return t.currT0.Add(rtpDur)
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopTracker struct{}

func (n NoopTracker) Track(bts []byte) ([]byte, utc.UTC, error) {
	return nil, utc.Zero, nil
}

func (n NoopTracker) Stats() *Stats {
	return &Stats{}
}

func (n NoopTracker) Reset() {}

// ---------------------------------------------------------------------------------------------------------------------

type Stats struct {
	Start         utc.UTC       `json:"start"`
	Duration      duration.Spec `json:"duration"`
	PacketCount   int           `json:"packet_count"`
	ErrorCount    int           `json:"error_count"`
	StartSeq      int64         `json:"start_seq"`
	EndSeq        int64         `json:"end_seq"`
	StartTs       int64         `json:"start_ts"`
	EndTs         int64         `json:"end_ts"`
	RtpDuration   duration.Spec `json:"rtp_duration"`
	TsAdjCount    int           `json:"ts_adj_count"`
	TsAdjDuration duration.Spec `json:"ts_adj_duration"`
	Gaps          []Gap         `json:"gaps"`
}

// ---------------------------------------------------------------------------------------------------------------------

type Gap struct {
	PacketNum int   `json:"packet_num"`
	Seq       int64 `json:"seq"`
	SeqPrev   int64 `json:"seq_prev"`
	SeqDiff   int64 `json:"seq_diff"`
	Ts        int64 `json:"ts"`
	TsPrev    int64 `json:"ts_prev"`
	TsDiff    int64 `json:"ts_diff"`
}
