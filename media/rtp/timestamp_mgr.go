package rtp

import (
	"time"

	"github.com/eluv-io/utc-go"
)

type timestampMgr struct {
	resyncThreshold int32 // the max accepted difference between last and new timestamp to resync the timestamp

	start     utc.UTC // the wall clock time when the stream started
	rtp0      utc.UTC // the wall clock time corresponding to rtp timestamp 0, calculated from the first packet
	wrapCount uint64  // number of times the RTP timestamp wrapped around

	lastTs        uint32            // the last timestamp received
	lastUtc       utc.UTC           // reception of last timestamp as wall clock time
	resyncs       int               // number of times the timestamp has been resynced
	lastResyncUtc utc.UTC           // wall clock time of last resync event
	seq           SequenceUnwrapper // converts 16-bit RTP sequence numbers to 64-bit without wrap-around
}

func (m *timestampMgr) CalculateWaitTime(timestamp uint32, sequence uint16) (keep bool, wait time.Duration) {
	now := utc.Now()
	last, current := m.seq.Unwrap(sequence)
	if current == last+1 {
		// sequence number is contiguous, timestamp should be good
		log.Trace("packet in order", "last_seq", last, "current_seq", current)
	} else if current < last {
		log.Warn("packet reordering detected, ignoring timestamp", "last_seq", last, "current_seq", current)
		return true, 0
	} else if current == last {
		log.Warn("duplicate packet detected, dropping", "last_seq", last, "current_seq", current)
		return false, 0
	} else { // current > last+1
		log.Warn("packet loss detected, resyncing timestamp", "last_seq", last, "current_seq", current)
		m.resyncs++
		m.lastResyncUtc = now
		m.lastTs = timestamp
		m.lastUtc = now
		return true, 0
	}

	diff := int32(timestamp - m.lastTs)
	if diff < -1000 || diff > 1000 {

	}
	return false, 0
}
