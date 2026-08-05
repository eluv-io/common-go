package tracker

import (
	"fmt"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/mpegts"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/histogram"
	"github.com/eluv-io/common-go/util/statsutil"
)

// Stats is the JSON-serializable snapshot of the stream's timing characteristics for a period or the whole capture.
type Stats struct {
	Source  string        `json:"source"`
	Elapsed duration.Spec `json:"elapsed"`
	Window  duration.Spec `json:"window,omitempty"` // reporting period (PeriodStats only)
	Packets uint64        `json:"packets"`
	Bytes   uint64        `json:"bytes"`
	Pps     float64       `json:"pps"`         // packets per second
	Bitrate uint64        `json:"bitrate_bps"` // bits per second
	Rate    *RateStats    `json:"rate,omitempty"`
	Ipd     Distribution  `json:"ipd_ms"` // inter-packet delay distribution (milliseconds)
	Clocks  []ClockStats  `json:"clocks"` // media-clock correlations (RTP timestamp and/or per-program MPEG-TS PCR)
	Outages OutageStats   `json:"outages"`
	Errors  ErrorStats    `json:"errors"`       // cumulative validation/integrity errors
	Ts      *mpegts.Stats `json:"ts,omitempty"` // MPEG-TS program/PID structure and integrity (Stats only)
}

// Distribution summarizes a set of millisecond measurements: count, extremes, mean, standard deviation and selected
// percentiles (estimated from a histogram). The millisecond values serialize with 3 digits after the decimal point.
type Distribution struct {
	Count  uint64          `json:"count"`
	Min    duration.Millis `json:"min"`
	Mean   duration.Millis `json:"mean"`
	Max    duration.Millis `json:"max"`
	Stddev duration.Millis `json:"stddev"`
	P50    duration.Millis `json:"p50"`
	P90    duration.Millis `json:"p90"`
	P95    duration.Millis `json:"p95"`
	P99    duration.Millis `json:"p99"`
}

// RateStats captures the variation of packet and bit rate across reporting periods (Stats only).
type RateStats struct {
	PpsMean   float64 `json:"pps_mean"`
	PpsStddev float64 `json:"pps_stddev"`
	PpsMin    float64 `json:"pps_min"`
	PpsMax    float64 `json:"pps_max"`
	BpsMean   uint64  `json:"bps_mean"`
	BpsStddev uint64  `json:"bps_stddev"`
	BpsMin    uint64  `json:"bps_min"`
	BpsMax    uint64  `json:"bps_max"`
}

// ClockStats describes the correlation between packet arrival and one of the stream's media clocks.
type ClockStats struct {
	Source          string          `json:"source"` // "rtp" or "pcr"
	Pid             int             `json:"pid,omitempty"`
	Samples         uint64          `json:"samples"`
	CurrentSkewMs   duration.Millis `json:"current_skew_ms"` // most recent arrival-minus-media skew
	SkewMinMs       duration.Millis `json:"skew_min_ms"`
	SkewMeanMs      duration.Millis `json:"skew_mean_ms"`
	SkewMaxMs       duration.Millis `json:"skew_max_ms"`
	JitterMs        duration.Millis `json:"jitter_ms"` // standard deviation of the skew (packet delay variation)
	DriftPpm        float64         `json:"drift_ppm"` // media-vs-wall clock rate error
	Discontinuities uint64          `json:"discontinuities"`
	ParseErrors     uint64          `json:"parse_errors"`

	// NumWraps is the number of true PCR wraparounds observed on this PID ("pcr" source only).
	NumWraps int64 `json:"num_wraps,omitempty"`

	// PacketCount, ErrorCount and Gaps describe RTP-level packet health ("rtp" source only). Gaps is bounded by
	// Config.MaxGaps; GapsOverflow counts any gaps beyond that bound that were not retained.
	PacketCount  uint64    `json:"packet_count,omitempty"`
	ErrorCount   uint64    `json:"error_count,omitempty"`
	Gaps         []rtp.Gap `json:"gaps,omitempty"`
	GapsOverflow uint64    `json:"gaps_overflow,omitempty"`
}

// OutageStats counts gaps between consecutive packets exceeding Config.OutageThreshold.
type OutageStats struct {
	Count   uint64          `json:"count"`
	TotalMs duration.Millis `json:"total_ms"`
}

// ErrorStats summarizes validation and input-integrity errors accumulated over the capture. Total and CcErrors are
// mpegts.TsStreamTracker's cumulative error count and continuity-counter-error subtotal; ByPid lists the PIDs that
// have seen continuity errors. The remaining fields cover input-integrity conditions mpegts.TsStreamTracker does not
// itself detect (malformed/undersized datagrams, RTP framing issues, faulty padding).
type ErrorStats struct {
	Total    int         `json:"total"`
	CcErrors int         `json:"cc_errors"`
	ByPid    []PidErrors `json:"by_pid,omitempty"`

	SmallPacketsDropped   uint64 `json:"small_packets_dropped"`
	RtcpPacketsDropped    uint64 `json:"rtcp_packets_dropped"`
	BadPackets            uint64 `json:"bad_packets"`
	IncompletePackets     uint64 `json:"incomplete_packets"`
	AdaptationFieldErrors uint64 `json:"adaptation_field_errors"`
	FaultyPaddingPackets  uint64 `json:"faulty_padding_packets"`
	LongHeaders           uint64 `json:"long_headers"`
}

// PidErrors is the per-PID continuity-counter error count.
type PidErrors struct {
	Pid      int `json:"pid"`
	CcErrors int `json:"cc_errors"`
}

// newDistribution builds a Distribution report from the collected statistics and the matching percentile histogram.
func newDistribution(s *statsutil.Statistics[duration.Millis], h *histogram.Histogram[duration.Millis]) Distribution {
	return Distribution{
		Count:  s.Count,
		Min:    s.Min,
		Mean:   s.Mean,
		Max:    s.Max,
		Stddev: duration.Millis(stddev(s)),
		P50:    h.Quantile(0.50),
		P90:    h.Quantile(0.90),
		P95:    h.Quantile(0.95),
		P99:    h.Quantile(0.99),
	}
}

// newIpdHistogram builds a fine-grained millisecond histogram suitable for inter-packet delays, which for a healthy
// stream are typically only a few milliseconds. Percentiles are estimated from these bins.
func newIpdHistogram() *histogram.Histogram[duration.Millis] {
	maxes := []float64{
		0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4, 5, 7, 10, 15, 20, 30, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 2000, 5000,
	}
	bins := make([]*histogram.HistogramBin[duration.Millis], 0, len(maxes)+1)
	for _, m := range maxes {
		bins = append(bins, &histogram.HistogramBin[duration.Millis]{
			Label: fmt.Sprintf("<=%gms", m),
			Max:   duration.MillisFromFloat(m),
		})
	}
	bins = append(bins, &histogram.HistogramBin[duration.Millis]{Label: ">5000ms"}) // unbounded final bin
	h, _ := histogram.NewHistogramBins(bins)
	return h
}
