package structured

import (
	"testing"
)

var benchSink any // prevents dead-code elimination

// maps

func BenchmarkClone_MapStringAny_Small(b *testing.B) {
	m := map[string]any{"a": 1, "b": "two", "c": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(m)
	}
}

func BenchmarkCloneMap_MapStringAny_Small(b *testing.B) {
	m := map[string]any{"a": 1, "b": "two", "c": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneMap(m)
	}
}

func BenchmarkClone_MapStringAny_Large(b *testing.B) {
	m := make(map[string]any, 100)
	for i := 0; i < 100; i++ {
		m[string(rune('a'+i%26))+string(rune('0'+i%10))] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(m)
	}
}

func BenchmarkCloneMap_MapStringAny_Large(b *testing.B) {
	m := make(map[string]any, 100)
	for i := 0; i < 100; i++ {
		m[string(rune('a'+i%26))+string(rune('0'+i%10))] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneMap(m)
	}
}

func BenchmarkClone_MapStringAny_Nested(b *testing.B) {
	m := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"vals": []any{1, 2, 3, 4, 5},
			},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(m)
	}
}

func BenchmarkCloneMap_MapStringAny_Nested(b *testing.B) {
	m := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"vals": []any{1, 2, 3, 4, 5},
			},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneMap(m)
	}
}

func BenchmarkCloneMap_StringInt(b *testing.B) {
	m := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneMap(m)
	}
}

// slices

func BenchmarkClone_SliceAny(b *testing.B) {
	s := make([]any, 50)
	for i := range s {
		s[i] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(s)
	}
}

func BenchmarkCloneSlice_SliceAny(b *testing.B) {
	s := make([]any, 50)
	for i := range s {
		s[i] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneSlice(s)
	}
}

func BenchmarkClone_SliceInt(b *testing.B) {
	s := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(s)
	}
}

func BenchmarkCloneSlice_SliceInt(b *testing.B) {
	s := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneSlice(s)
	}
}

func BenchmarkClone_SliceString(b *testing.B) {
	s := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(s)
	}
}

func BenchmarkCloneSlice_SliceString(b *testing.B) {
	s := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneSlice(s)
	}
}

// byte slices

func BenchmarkClone_SliceByte(b *testing.B) {
	buf := make([]byte, 25600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(buf)
	}
}

func BenchmarkCloneSlice_SliceByte(b *testing.B) {
	buf := make([]byte, 25600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneSlice(buf)
	}
}

type Bytes []byte

func BenchmarkClone_NamedSliceByte(b *testing.B) {
	buf := make(Bytes, 25600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(buf)
	}
}

func BenchmarkCloneSlice_NamedSliceByte(b *testing.B) {
	buf := make(Bytes, 25600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneSlice(buf)
	}
}

// custom map types

type myKey string
type myMap map[myKey]any

func BenchmarkClone_MapCustom(b *testing.B) {
	m := myMap{"a": 1, "b": "two", "c": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = Clone(m)
	}
}

func BenchmarkCloneMap_MapCustom(b *testing.B) {
	m := myMap{"a": 1, "b": "two", "c": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = CloneMap(m)
	}
}
