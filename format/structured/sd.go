package structured

// SD is a wrapper around structured data that exposes the data query and
// manipulation functions of the structured package on this data.
type SD struct {
	Data interface{}
}

// Wrap wraps the given data structure as an SD object, offering query and
// manipulation functions for the data.
func Wrap(data interface{}) *SD {
	return &SD{data}
}

func (sd *SD) Set(path Path, data interface{}) error {
	data, err := Set(sd.Data, path, data)
	if err != nil {
		return err
	}
	sd.Data = data
	return nil
}

func (sd *SD) Merge(path Path, data interface{}) error {
	data, err := Merge(sd.Data, path, data)
	if err != nil {
		return err
	}
	sd.Data = data
	return nil
}

func (sd *SD) Delete(path ...string) error {
	return sd.Set(path, nil)
}

func (sd *SD) Get(path ...string) *Value {
	return NewValue(Resolve(path, sd.Data))
}

func (sd *SD) Query(query string) *Value {
	filter, err := NewFilter(query)
	if err != nil {
		return NewValue(nil, err)
	}
	return NewValue(filter.Apply(sd.Data))
}

func (sd *SD) Clear() error {
	return sd.Set(nil, nil)
}

// Path is a convenience method to create a path from an arbitrary number of
// strings.
func (sd *SD) Path(p ...string) Path {
	return Path(p)
}
