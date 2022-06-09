package sliceutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var appendTests = []struct {
	source              []interface{}
	target              []interface{}
	wantAppend          []interface{}
	wantSquash          []interface{}
	wantSquashAndDedupe []interface{}
}{
	{
		source:              nil,
		target:              nil,
		wantAppend:          nil,
		wantSquash:          nil,
		wantSquashAndDedupe: nil,
	},
	{
		source:              []interface{}{},
		target:              nil,
		wantAppend:          nil,
		wantSquash:          nil,
		wantSquashAndDedupe: nil,
	},
	{
		source:              nil,
		target:              []interface{}{},
		wantAppend:          []interface{}{},
		wantSquash:          []interface{}{},
		wantSquashAndDedupe: []interface{}{},
	},
	{
		source:              []interface{}{},
		target:              []interface{}{},
		wantAppend:          []interface{}{},
		wantSquash:          []interface{}{},
		wantSquashAndDedupe: []interface{}{},
	},
	{
		source:              []interface{}{"a", "b", "c"},
		target:              nil,
		wantAppend:          []interface{}{"a", "b", "c"},
		wantSquash:          []interface{}{"a", "b", "c"},
		wantSquashAndDedupe: []interface{}{"a", "b", "c"},
	},
	{
		source:              nil,
		target:              []interface{}{"a", "b", "c"},
		wantAppend:          []interface{}{"a", "b", "c"},
		wantSquash:          []interface{}{"a", "b", "c"},
		wantSquashAndDedupe: []interface{}{"a", "b", "c"},
	},
	{
		source:              []interface{}{"a", "b", "c"},
		target:              []interface{}{"a", "b", "c"},
		wantAppend:          []interface{}{"a", "b", "c", "a", "b", "c"},
		wantSquash:          []interface{}{"a", "b", "c"},
		wantSquashAndDedupe: []interface{}{"a", "b", "c"},
	},
	{
		source:              []interface{}{"a", "a", "d", "d"},
		target:              []interface{}{"a", "b", "b", "c"},
		wantAppend:          []interface{}{"a", "b", "b", "c", "a", "a", "d", "d"},
		wantSquash:          []interface{}{"a", "b", "b", "c", "d"},
		wantSquashAndDedupe: []interface{}{"a", "b", "c", "d"},
	},
	{
		source:              []interface{}{2, "b", []interface{}{"nested"}},
		target:              []interface{}{"a", 1, 1.3, []interface{}{"nested"}, 1},
		wantAppend:          []interface{}{"a", 1, 1.3, []interface{}{"nested"}, 1, 2, "b", []interface{}{"nested"}},
		wantSquash:          []interface{}{"a", 1, 1.3, []interface{}{"nested"}, 1, 2, "b"},
		wantSquashAndDedupe: []interface{}{"a", 1, 1.3, []interface{}{"nested"}, 2, "b"},
	},
}

func TestAppend(t *testing.T) {
	for _, tt := range appendTests {
		for _, makeCopy := range []bool{false, true} {
			t.Run(fmt.Sprint(tt.source, tt.target, makeCopy), func(t *testing.T) {
				target := tt.target
				targetCopy := Copy(target)
				assert.Equalf(t, tt.wantAppend, Append(tt.source, tt.target, makeCopy), "Append(%v, %v, %v)", tt.source, tt.target, makeCopy)
				if makeCopy {
					assert.Equal(t, targetCopy, tt.target)
				}
			})
		}
	}
}

func TestSquash(t *testing.T) {
	for _, tt := range appendTests {
		for _, makeCopy := range []bool{false, true} {
			t.Run(fmt.Sprint(tt.source, tt.target, makeCopy), func(t *testing.T) {
				target := tt.target
				targetCopy := Copy(target)
				assert.Equalf(t, tt.wantSquash, Squash(tt.source, tt.target, makeCopy), "Append(%v, %v, %v)", tt.source, tt.target, makeCopy)
				if makeCopy {
					assert.Equal(t, targetCopy, tt.target)
				}
			})
		}
	}
}

func TestSquashAndDedupe(t *testing.T) {
	for _, tt := range appendTests {
		for _, makeCopy := range []bool{false, true} {
			t.Run(fmt.Sprint(tt.source, tt.target, makeCopy), func(t *testing.T) {
				target := tt.target
				targetCopy := Copy(target)
				assert.Equalf(t, tt.wantSquashAndDedupe, SquashAndDedupe(tt.source, tt.target, makeCopy), "Append(%v, %v, %v)", tt.source, tt.target, makeCopy)
				if makeCopy {
					assert.Equal(t, targetCopy, tt.target)
				}
			})
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		slice    []interface{}
		elements []interface{}
		wantRes  bool
	}{
		{
			slice:    nil,
			elements: []interface{}{nil, "a", 1},
			wantRes:  false,
		},
		{
			slice:    []interface{}{},
			elements: []interface{}{nil, "a", 1},
			wantRes:  false,
		},
		{
			slice:    []interface{}{"a", 1, true},
			elements: []interface{}{"a", 1, true},
			wantRes:  true,
		},
		{
			slice:    []interface{}{"a", 1, true},
			elements: []interface{}{"b", 2, false},
			wantRes:  false,
		},
		{
			slice:    []interface{}{"a", []interface{}{nil, "a", 1}},
			elements: []interface{}{[]interface{}{nil, "a", 1}},
			wantRes:  true,
		},
	}
	for _, tt := range tests {
		for _, element := range tt.elements {
			t.Run(fmt.Sprint(element, "in", tt.slice, "?"), func(t *testing.T) {
				assert.Equalf(t, tt.wantRes, Contains(tt.slice, element), "Contains(%v, %v)", tt.slice, element)
			})
		}
	}
}

func TestDedupe(t *testing.T) {
	tests := []struct {
		target  []interface{}
		wantRes []interface{}
	}{
		{
			target:  nil,
			wantRes: nil,
		},
		{
			target:  []interface{}{},
			wantRes: []interface{}{},
		},
		{
			target:  []interface{}{"a", "b", "c"},
			wantRes: []interface{}{"a", "b", "c"},
		},
		{
			target:  []interface{}{"a", "a", "b"},
			wantRes: []interface{}{"a", "b"},
		},
		{
			target:  []interface{}{"a", "b", "a"},
			wantRes: []interface{}{"a", "b"},
		},
		{
			target:  []interface{}{[]interface{}{"a", "b", "c"}, []interface{}{"a", "b", "c"}},
			wantRes: []interface{}{[]interface{}{"a", "b", "c"}},
		},
	}
	for _, tt := range tests {
		for _, makeCopy := range []bool{false, true} {
			t.Run(fmt.Sprint(tt.target, makeCopy), func(t *testing.T) {
				assert.Equalf(t, tt.wantRes, Dedupe(tt.target, makeCopy), "Dedupe(%v, %v)", tt.target, makeCopy)
			})
		}
	}
}

func TestDuplicate(t *testing.T) {
	tests := []struct {
		target []interface{}
		want   []interface{}
	}{
		{
			target: nil,
			want:   nil,
		},
		{
			target: []interface{}{},
			want:   []interface{}{},
		},
		{
			target: []interface{}{"a", "b", "c"},
			want:   []interface{}{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.target), func(t *testing.T) {
			dup := Copy(tt.target)
			assert.Equalf(t, tt.want, dup, "Copy(%v)", tt.target)

			// modify target and ensure it's not reflected in duplicate
			if len(tt.target) > 0 {
				tt.target[0] = 123
			}
			assert.Equalf(t, tt.want, dup, "Copy(%v)", tt.target)
		})
	}
}

func TestDuplicateWithCap(t *testing.T) {
	tests := []struct {
		target       []interface{}
		capacity     int
		want         []interface{}
		wantCapacity int
	}{
		{
			target:       nil,
			capacity:     5,
			want:         nil,
			wantCapacity: 5,
		},
		{
			target:       []interface{}{},
			capacity:     5,
			want:         []interface{}{},
			wantCapacity: 5,
		},
		{
			target:       []interface{}{"a"},
			capacity:     5,
			want:         []interface{}{"a"},
			wantCapacity: 5,
		},
		{
			target:       []interface{}{"a", "b", "c"},
			capacity:     1,
			want:         []interface{}{"a", "b", "c"},
			wantCapacity: 3,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.target, tt.capacity), func(t *testing.T) {
			dup := CopyWithCap(tt.target, tt.capacity)
			assert.Equalf(t, tt.want, dup, "CopyWithCap(%v, %v)", tt.target, tt.capacity)
			if tt.want != nil {
				assert.Equalf(t, tt.wantCapacity, cap(dup), "CopyWithCap(%v, %v)", tt.target, tt.capacity)
			}

			// modify target and ensure it's not reflected in duplicate
			if len(tt.target) > 0 {
				tt.target[0] = 123
			}
			assert.Equalf(t, tt.want, dup, "Copy(%v)", tt.target)
		})
	}
}
