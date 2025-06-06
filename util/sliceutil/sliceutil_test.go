package sliceutil

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/constraints"
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

func TestContainsEqualer(t *testing.T) {
	tests := []struct {
		slice []*Eq
		el    *Eq
		want  bool
	}{
		{
			slice: nil,
			el:    newEq(3),
			want:  false,
		},
		{
			slice: []*Eq{},
			el:    newEq(3),
			want:  false,
		},
		{
			slice: []*Eq{newEq(1), newEq(2), newEq(3), newEq(4), newEq(5)},
			el:    newEq(3),
			want:  true,
		},
		{
			slice: []*Eq{newEq(1), newEq(2), newEq(3), newEq(4), newEq(5)},
			el:    newEq(6),
			want:  false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.slice, test.el), func(t *testing.T) {
			res := Contains(test.slice, test.el)
			require.Equal(t, test.want, res)
		})
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

func TestCopy(t *testing.T) {
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
			cln := Clone(tt.target)
			assert.Equalf(t, tt.want, dup, "Copy(%v)", tt.target)
			assert.Equalf(t, tt.want, cln, "Copy(%v)", tt.target)

			// modify target and ensure it's not reflected in duplicate
			if len(tt.target) > 0 {
				tt.target[0] = 123
			}
			assert.Equalf(t, tt.want, dup, "Copy(%v)", tt.target)
		})
	}
}

func TestCopyWithCap(t *testing.T) {
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

func TestRemove(t *testing.T) {
	tests := []struct {
		slice  []int
		remove int
		want   []int
	}{
		{
			slice:  nil,
			remove: 3,
			want:   nil,
		},
		{
			slice:  []int{},
			remove: 3,
			want:   []int{},
		},
		{ // no match
			slice:  []int{1, 2, 3, 4, 5},
			remove: 6,
			want:   []int{1, 2, 3, 4, 5},
		},
		{ // one match at beginning
			slice:  []int{1, 2, 3, 4, 5},
			remove: 1,
			want:   []int{2, 3, 4, 5},
		},
		{ // one match in middle
			slice:  []int{1, 2, 3, 4, 5},
			remove: 3,
			want:   []int{1, 2, 4, 5},
		},
		{ // one match at end
			slice:  []int{1, 2, 3, 4, 5},
			remove: 5,
			want:   []int{1, 2, 3, 4},
		},
		{ // multiple matches
			slice:  []int{1, 2, 1, 2, 1, 2, 3},
			remove: 2,
			want:   []int{1, 1, 1, 3},
		},
		{ // all match
			slice:  []int{1, 1, 1, 1, 1},
			remove: 1,
			want:   []int{},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.slice, test.remove), func(t *testing.T) {
			res, removed := Remove(test.slice, test.remove)
			require.Equal(t, test.want, res)
			require.Equal(t, removed, len(test.slice)-len(res))
			// make sure freed up elements at the end are zeroed out
			for i := 0; i < removed; i++ {
				require.Equal(t, 0, test.slice[len(test.slice)-1-i])
			}
		})
	}
}

type absInt int

func (n absInt) Equal(other absInt) bool {
	if n < 0 {
		n = -n
	}
	if other < 0 {
		other = -other
	}
	return n == other
}

func TestRemoveEqualer(t *testing.T) {
	tests := []struct {
		slice []absInt
		el    absInt
		want  []absInt
		wantN int
	}{
		{[]absInt{1, 2, 3, 4, 5, 6}, 3, []absInt{1, 2, 4, 5, 6}, 1},
		{[]absInt{1, 2, 3, 4, 5, 6}, -3, []absInt{1, 2, 4, 5, 6}, 1},
	}

	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			res, n := Remove(test.slice, test.el)
			require.Equal(t, test.want, res)
			require.Equal(t, test.wantN, n)
		})
	}
}

func TestRemoveIndex(t *testing.T) {
	tests := []struct {
		slice  []string
		remove int
		want   []string
	}{
		{
			slice:  nil,
			remove: 3,
			want:   nil,
		},
		{
			slice:  []string{},
			remove: 3,
			want:   []string{},
		},
		{
			slice:  []string{"1", "2", "3", "4", "5"},
			remove: 3,
			want:   []string{"1", "2", "3", "5"},
		},
		{
			slice:  []string{"1", "2", "3", "4", "5"},
			remove: 99,
			want:   []string{"1", "2", "3", "4", "5"},
		},
		{
			slice:  []string{"1", "2", "3", "4", "5"},
			remove: -1,
			want:   []string{"1", "2", "3", "4", "5"},
		},
		{
			slice:  []string{"1", "2", "3", "4", "5"},
			remove: 6,
			want:   []string{"1", "2", "3", "4", "5"},
		},
		{
			slice:  []string{"1", "2", "1", "2", "1", "2", "3"},
			remove: 2,
			want:   []string{"1", "2", "2", "1", "2", "3"},
		},
		{
			slice:  []string{"1", "1", "1", "1", "1"},
			remove: 1,
			want:   []string{"1", "1", "1", "1"},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.slice, test.remove), func(t *testing.T) {
			res := RemoveIndex(test.slice, test.remove)
			require.Equal(t, test.want, res)
		})
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		slce []string
		want []string
	}{
		{
			slce: nil,
			want: nil,
		},
		{
			slce: []string{},
			want: []string{},
		},
		{
			slce: []string{"a"},
			want: []string{"a"},
		},
		{
			slce: []string{"a", "b"},
			want: []string{"b", "a"},
		},
		{
			slce: []string{"a", "b", "c"},
			want: []string{"c", "b", "a"},
		},
		{
			slce: []string{"a", "b", "c", "d"},
			want: []string{"d", "c", "b", "a"},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.slce), func(t *testing.T) {
			Reverse(test.slce)
			require.Equal(t, test.want, test.slce)
		})
	}
}

func TestCompare(t *testing.T) {
	type testCase[T constraints.Ordered] struct {
		name string
		s1   []T
		s2   []T
		want int
	}
	tests := []testCase[int]{
		{"both nil", nil, nil, 0},
		{"both empty", []int{}, []int{}, 0},
		{"nil & empty", nil, []int{}, 0},
		{"identical", []int{1, 2, 3}, []int{1, 2, 3}, 0},
		{"shorter", []int{1, 2}, []int{1, 2, 3}, -1},
		{"longer", []int{1, 2, 3}, []int{1, 2}, 1},
		{"smaller", []int{1, 2, 3}, []int{1, 3, 3}, -1},
		{"larger", []int{1, 2, 3}, []int{1, 1, 3}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Compare(tt.s1, tt.s2), "Compare(%v, %v)", tt.s1, tt.s2)
		})
	}
}

func TestFirst(t *testing.T) {
	tests := []struct {
		slce []string
		want string
	}{
		{
			slce: nil,
			want: "",
		},
		{
			slce: []string{},
			want: "",
		},
		{
			slce: []string{"a"},
			want: "a",
		},
		{
			slce: []string{"a", "b"},
			want: "a",
		},
		{
			slce: []string{"a", "b", "c"},
			want: "a",
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.slce), func(t *testing.T) {
			res := First(test.slce)
			require.Equal(t, test.want, res)
		})
	}
}

func TestLast(t *testing.T) {
	tests := []struct {
		slce []string
		want string
	}{
		{
			slce: nil,
			want: "",
		},
		{
			slce: []string{},
			want: "",
		},
		{
			slce: []string{"a"},
			want: "a",
		},
		{
			slce: []string{"a", "b"},
			want: "b",
		},
		{
			slce: []string{"a", "b", "c"},
			want: "c",
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.slce), func(t *testing.T) {
			res := Last(test.slce)
			require.Equal(t, test.want, res)
		})
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		slice         []int
		fn            func(int) int
		wantConverted []int
	}{
		{nil, nil, nil},
		{[]int{}, nil, []int{}},
		{[]int{1, 2, 3}, func(i int) int { return i + 1 }, []int{2, 3, 4}},
		{[]int{1, 2, 3, 4}, func(i int) int { return -i }, []int{-1, -2, -3, -4}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.slice), func(t *testing.T) {
			assert.Equal(t, tt.wantConverted, Convert(tt.slice, tt.fn))
		})
	}
}

func TestConvertString(t *testing.T) {
	tests := []struct {
		slice         []FmtStringer
		wantConverted []string
	}{
		{nil, nil},
		{[]FmtStringer{}, []string{}},
		{[]FmtStringer{{1}, {2}, {3}}, []string{"1", "2", "3"}},
		{[]FmtStringer{{1.1}, {2.2}, {3.3}}, []string{"1.1", "2.2", "3.3"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.slice), func(t *testing.T) {
			assert.Equal(t, tt.wantConverted, ConvertString(tt.slice))
		})
	}
}

func TestRepeatElement(t *testing.T) {
	tests := []struct {
		el    string
		count int
		want  []string
	}{
		{"a", 0, []string{}},
		{"a", 1, []string{"a"}},
		{"a", 3, []string{"a", "a", "a"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.el, "x", tt.count, "=", tt.want), func(t *testing.T) {
			assert.Equal(t, tt.want, RepeatElement(tt.el, tt.count))
		})
	}
}

func TestRepeat(t *testing.T) {
	tests := []struct {
		slice []int
		count int
		want  []int
	}{
		{[]int{}, 0, []int{}},
		{[]int{}, 3, []int{}},
		{[]int{1}, 0, []int{}},
		{[]int{1}, 1, []int{1}},
		{[]int{1}, 3, []int{1, 1, 1}},
		{[]int{1, 2, 3}, 2, []int{1, 2, 3, 1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.slice, "x", tt.count, "=", tt.want), func(t *testing.T) {
			assert.Equal(t, tt.want, Repeat(tt.slice, tt.count))
		})
	}
}

////////////////////////////////////////////////////////////////////////////////

func newEq(i int) *Eq {
	return &Eq{i, rand.Intn(1000)}
}

type Eq struct {
	i   int
	rnd int
}

func (e *Eq) Equal(other *Eq) bool {
	return e.i == other.i
}

func (e *Eq) String() string {
	return fmt.Sprintf("%d-%4d", e.i, e.rnd)
}

////////////////////////////////////////////////////////////////////////////////

type FmtStringer struct {
	i any
}

func (is FmtStringer) String() string {
	return fmt.Sprint(is.i)
}
