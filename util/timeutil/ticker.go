package timeutil

import (
	"sync"
	"time"
)

// Ticker is a utility that calls the Tick() function of all registered listeners at a configured interval from a single
// goroutine. Tickers must be registered to start receiving ticks and unregistered to stop. A Ticker is similar to a
// time.Ticker, but provides notifications through a function call instead of a channel and allows for multiple
// listeners.
type Ticker interface {
	Register(TickListener)
	Unregister(TickListener)
}

// ManualTicker is a ticker that can be triggered manually (mainly useful for testing). It does not periodically tick
// its listeners, only when Tick() is called.
type ManualTicker interface {
	Ticker
	// Tick manually ticks all registered listeners.
	Tick()
}

// TickListener is an interface for objects that can be ticked.
type TickListener interface {
	Tick()
}

// ---------------------------------------------------------------------------------------------------------------------

// DefaultTicker is the default Ticker that notifies all registered listeners every 5 seconds.
var DefaultTicker = NewTicker(5 * time.Second)

// ---------------------------------------------------------------------------------------------------------------------

var _ Ticker = (*ticker)(nil)

// ticker is the default implementation of Ticker.
type ticker struct {
	tickInterval time.Duration // no ticks are sent if <= 0
	mutex        sync.RWMutex
	started      bool
	listeners    map[TickListener]struct{}
	stop         chan struct{}
}

// NewTicker creates a new Ticker that notifies all registered listeners every interval.
func NewTicker(interval time.Duration) Ticker {
	return &ticker{tickInterval: interval, listeners: make(map[TickListener]struct{})}
}

func (ci *ticker) Register(t TickListener) {
	ci.mutex.Lock()
	defer ci.mutex.Unlock()
	ci.listeners[t] = struct{}{}

	// ensure ticker goroutine is running
	if !ci.started && ci.tickInterval > 0 {
		ci.started = true
		ci.stop = make(chan struct{})
		go ci.run(ci.stop)
	}
}

// Unregister removes the given ticker and stops the ticker goroutine if no Tickers are registered.
func (ci *ticker) Unregister(t TickListener) {
	ci.mutex.Lock()
	defer ci.mutex.Unlock()
	delete(ci.listeners, t)

	// stop the ticker goroutine if no Tickers are registered
	if len(ci.listeners) == 0 && ci.started {
		ci.started = false
		close(ci.stop)
		ci.stop = nil
	}
}

// Starts the ticker goroutine that ticks all registered Tickers periodically.
func (ci *ticker) run(stop chan struct{}) {
	ticker := time.NewTicker(ci.tickInterval)
	for {
		select {
		case <-ticker.C:
			ci.tick()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

// tick ticks all registered Tickers.
func (ci *ticker) tick() {
	ci.mutex.RLock()
	defer ci.mutex.RUnlock()
	for meter := range ci.listeners {
		meter.Tick()
	}
}

// ---------------------------------------------------------------------------------------------------------------------

func NewManualTicker() ManualTicker {
	return &manualTicker{NewTicker(0).(*ticker)}
}

type manualTicker struct {
	*ticker
}

func (m *manualTicker) Tick() {
	m.ticker.tick()
}
