package duration

type Rounded Spec

func (r Rounded) String() string {
	return Spec(r).RoundTo(1).String()
}
