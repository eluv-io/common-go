package duration

// Rounded is a display-only variant of Spec that formats with 1 decimal place of precision via String(). It
// intentionally omits MarshalText/UnmarshalText/MarshalJSON/UnmarshalJSON — it is not suitable for serialization.
// Use Spec directly when the value must survive a marshal/unmarshal round-trip.
type Rounded Spec

func (r Rounded) String() string {
	return Spec(r).RoundTo(1).String()
}
