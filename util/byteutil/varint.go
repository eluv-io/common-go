package byteutil

// LenUvarInt returns the number of bytes needed to encode the given uint64 as
// varint.
func LenUvarInt(x uint64) int {
	i := 0
	for x >= 0x80 {
		x >>= 7
		i++
	}
	return i + 1
}
