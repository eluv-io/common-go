package partitionutil

import (
	"encoding/binary"
	"fmt"

	"github.com/eluv-io/errors-go"
)

// PartitionPrefixBytes is the number of bytes used for partition matching. This is the number of bytes stored for each
// part in the indexer DB in binary format to enable efficient partition matching.
const PartitionPrefixBytes = 2

// PartitionPrefixBits is the number of bits used for partition matching: PartitionPrefixBytes * 8
const PartitionPrefixBits = PartitionPrefixBytes * 8

// PartitionPrefixNum returns the partition prefix as a 16-bit integer. This allows to match partitions based on the
// numeric partition range as returned by PartitionRange.
func PartitionPrefixNum(digest []byte) uint16 {
	return binary.BigEndian.Uint16(PartitionPrefix(digest))
}

// PartitionRange returns the range of partition prefix values that match the given partition and level
func PartitionRange(partition []byte, level int) (low, high uint16, err error) {
	if err = ValidateLevel(level); err != nil {
		return 0, 0, err
	}

	for i := 0; i < PartitionPrefixBytes; i++ {
		pi := uint16(0)
		if i < len(partition) {
			pi = uint16(partition[i])
		}
		if level >= 8 {
			low = (low << 8) + pi
			high = (high << 8) + pi
			level -= 8
		} else {
			low = (low << 8) + pi&(0xFF<<(8-level))
			high = (high << 8) + (pi | (0xFF >> level))
			level = 0
		}
	}
	return
}

// PartitionIndex returns the index of the partition that matches the given digest with the given partition level. The
// index is in the range [0, 2^level[ and denotes the ith partition
func PartitionIndex(digest []byte, level int) (int, error) {
	if err := ValidateLevel(level); err != nil {
		return 0, err
	}
	return int(PartitionPrefixNum(digest)) >> (PartitionPrefixBits - level), nil
}

// PartitionPrefixes returns the list of 2^level partition prefixes for the given level.
func PartitionPrefixes(level int) ([][]byte, error) {
	if err := ValidateLevel(level); err != nil {
		return nil, err
	}

	numPartitions := PartitionCount(level)
	partitions := make([][]byte, numPartitions)
	for i := 0; i < numPartitions; i++ {
		partitions[i], _ = PartitionPrefixForIndex(i, level)
	}

	return partitions, nil
}

// PartitionPrefixForIndex returns the partition prefix for the given partition index and level.
func PartitionPrefixForIndex(index, level int) ([]byte, error) {
	if err := ValidateLevel(level); err != nil {
		return nil, err
	}
	if index < 0 || index >= PartitionCount(level) {
		return nil, errors.NoTrace("PartitionPrefixForIndex", errors.K.Invalid,
			"reason", "index out of range",
			"index", index,
			"level", level)
	}
	si := index << (16 - level)
	return []byte{byte(si >> 8), byte(si)}, nil
}

// PartitionCount returns the total number of partitions at the given partition level (2^level).
func PartitionCount(level int) int {
	return 1 << level
}

// PartitionPrefix returns the first PartitionPrefixBytes bytes of the digest. That is the bit pattern stored in the
// indexer DB for each part and allows "simple" partition matching (does not support "partition numbers" > 1).
func PartitionPrefix(digest []byte) []byte {
	if len(digest) < PartitionPrefixBytes {
		d := make([]byte, PartitionPrefixBytes)
		copy(d, digest)
		return d
	}
	return digest[:PartitionPrefixBytes]
}

// PartitionMatch generates a byte slice for the given node's partition that can be used to match parts. It only retains
// the first "level" bits of the partition, all other bits are zeroed out. The result is always a slice of
// PartitionPrefixBytes. E.g. 1010 1100 with level 3 becomes 1010 0000.
func PartitionMatch(partition []byte, level int) ([]byte, error) {
	if err := ValidateLevel(level); err != nil {
		return nil, err
	}
	mask := make([]byte, PartitionPrefixBytes)
	for i := range partition {
		if level > 8 {
			mask[i] = partition[i]
			level -= 8
		} else {
			mask[i] = partition[i] & (0xFF << (8 - level))
			break
		}
	}
	return mask, nil
}

// PartitionMask generates a bitmask for the given partition level that should be applied to the partition prefix before
// matching with the bit pattern of partition, i.e. partition_match =? (partition_prefix & mask)
func PartitionMask(level int) []byte {
	mask := make([]byte, PartitionPrefixBytes)
	for i := range mask {
		if level > 8 {
			mask[i] = 0xff
			level -= 8
		} else {
			mask[i] = 0xff << (8 - level)
			break
		}
	}
	return mask
}

// ValidateLevel validates that the partition level is valid and in the supported range.
func ValidateLevel(level int) error {
	if level < 0 || level > PartitionPrefixBits {
		return errors.E(fmt.Sprintf("partition level must be in [0, %d]", PartitionPrefixBits),
			errors.K.Invalid, "level", level)
	}
	return nil
}

// AdjustLevel validates that the partition level and number are valid and in the supported range and returns the
// adjusted level according to the number of partitions. Number of partitions must be a power of 2, each power reducing
// the level by 1. See partition.NewNodePartition.
func AdjustLevel(level int, num int) (int, error) {
	if err := ValidateLevel(level); err != nil {
		return level, err
	}
	if num < 1 {
		return level, errors.E("partition number must be >= 1", errors.K.Invalid, "level", level, "num", num)
	}

	// adjust level
	adjusted := level
	for num > 1 {
		if (num>>1)<<1 != num { // num/2*2
			return level, errors.E("partition number must be a power of 2", errors.K.Invalid, "level", level, "num", num)
		}
		num >>= 1
		if adjusted > 0 {
			adjusted -= 1
		}
	}
	return adjusted, nil
}

// PartitionName returns the name of the given partition at the given level. The name is a string of the form "p000",
// "p001", etc.
func PartitionName(partition []byte, level int) string {
	idx, err := PartitionIndex(partition, level)
	if err != nil {
		return "pErr"
	}
	if level <= 8 {
		return fmt.Sprintf("p%0*b", level, idx)
	}
	return fmt.Sprintf("p%0*b", 8, idx>>(level-8)) + "_" + fmt.Sprintf("%0*b", level-8, idx&((1<<(level-8))-1))
}
