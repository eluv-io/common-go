package mpegts

// TsSyncModes is the enum for the different ways to synchronize packet reading on a TS stream.
const TsSyncModes tsSyncModes = 0

type TsSyncMode string

type tsSyncModes int

// Modulo will position the stream on multiples of the TS packet size in the first part without inspecting the TS stream
// for packet boundaries.
func (tsSyncModes) Modulo() TsSyncMode { return "modulo" }

// Stable works like Modulo(), but waits until the download rate of the first part has stabilized
// func (tsSyncModes) Stable() TsSyncMode { return "stable" }

// Once will inspect the stream and sync on packet boundaries only once at the beginning of the stream.
func (tsSyncModes) Once() TsSyncMode { return "once" }

// Continuous will continually verify that the stream is synced on packet boundaries and skip ahead to the next boundary
// otherwise.
func (tsSyncModes) Continuous() TsSyncMode { return "continuous" }
