package bytesize

import "strconv"

// HR is the same as Spec, but producing a human readable format in the String()
// call.
type HR Spec

func (h HR) Bytes() uint64 {
	return uint64(h)
}

func (h HR) String() string {
	return Spec(h).HR()
}

func (h HR) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (h *HR) UnmarshalText(t []byte) error {
	res, err := unmarshalText(t)
	*h = HR(res)
	return err
}

func (h *HR) UnmarshalJSON(t []byte) error {
	if t[0] == '"' && t[len(t)-1] == '"' {
		return h.UnmarshalText(t[1 : len(t)-1])
	}
	v, err := strconv.ParseUint(string(t), 10, 64)
	*h = HR(v)
	return err
}
