package io

import (
	srt "github.com/datarhei/gosrt"
)

// ConnStats reports runtime statistics of an open sink or source connection. It is returned by
// connections that implement StatsReporter.
type ConnStats struct {
	// RemoteAddr is the remote peer's address ("host:port") - the destination for a sink, or the
	// connected client for a source. Empty if unavailable (e.g. not yet connected, or a connectionless
	// UDP source that has no single remote peer).
	RemoteAddr string
	// LocalAddr is the local address ("host:port") of the connection - the address a source listens on,
	// or the local socket of a sink. Empty if unavailable.
	LocalAddr string
	// SRT carries SRT-protocol statistics; nil for non-SRT connections.
	SRT *SrtConnStats
}

// SrtConnStats holds the SRT-protocol statistics of a connection.
type SrtConnStats struct {
	// Version is the negotiated SRT protocol version of the peer.
	Version uint32
	// Encrypted reports whether the connection is encrypted (a passphrase was configured).
	Encrypted bool
	// Statistics holds the detailed SRT connection statistics. It is only populated when ConnStats is
	// called with details=true (gathering it has a cost).
	Statistics srt.Statistics
}

// StatsReporter is implemented by the connection value returned by PacketSink.Open (and
// PacketSource.Open) when it can report runtime statistics. Callers obtain stats via a type
// assertion on the opened writer/reader:
//
//	if r, ok := wc.(io.StatsReporter); ok { stats := r.ConnStats(true) }
type StatsReporter interface {
	// ConnStats returns the connection's current statistics. When details is false, expensive
	// protocol-level fields (SrtConnStats.Statistics) may be left zero.
	ConnStats(details bool) ConnStats
}
