package partitionutil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartitionMask(t *testing.T) {
	tests := []struct {
		level int
		want  []byte
	}{
		{0, []byte{0, 0}},
		{1, []byte{0b1000_0000, 0}},
		{2, []byte{0b1100_0000, 0}},
		{3, []byte{0b1110_0000, 0}},
		{7, []byte{0b1111_1110, 0}},
		{8, []byte{0b1111_1111, 0}},
		{9, []byte{0b1111_1111, 0b1000_0000}},
		{15, []byte{0b1111_1111, 0b1111_1110}},
		{16, []byte{0b1111_1111, 0b1111_1111}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.level), func(t *testing.T) {
			require.Equal(t, tt.want, PartitionMask(tt.level))
		})
	}
}

func TestPartitionMatch(t *testing.T) {
	tests := []struct {
		partition []byte
		level     int
		want      []byte
		wantName  string
		wantErr   bool
	}{
		{nil, 17, nil, "", true},
		{[]byte{0b1001_0110}, 3, []byte{0b1000_0000, 0b0000_0000}, "p100", false},
		{[]byte{0b1001_0110, 0b0000_0000}, 6, []byte{0b1001_0100, 0b0000_0000}, "p100101", false},
		{[]byte{0b1001_0110, 0b0000_0000}, 8, []byte{0b1001_0110, 0b0000_0000}, "p10010110", false},
		{[]byte{0b1001_0110, 0b0000_0000}, 9, []byte{0b1001_0110, 0b0000_0000}, "p10010110_0", false},
		{[]byte{0b1001_0110, 0b0000_0000}, 16, []byte{0b1001_0110, 0b0000_0000}, "p10010110_00000000", false},
		{[]byte{0b1001_0110, 0b0001_0111}, 16, []byte{0b1001_0110, 0b0001_0111}, "p10010110_00010111", false},
		{nil, 12, []byte{0, 0}, "p00000000_0000", false},
		{[]byte{0b1001_0110}, 12, []byte{0b1001_0110, 0b0000_0000}, "p10010110_0000", false},
		{[]byte{0b1001_0110}, 16, []byte{0b1001_0110, 0b0000_0000}, "p10010110_00000000", false},
	}
	for _, tt := range tests {
		name := PartitionName(tt.want, tt.level)
		t.Run(name, func(t *testing.T) {
			match, err := PartitionMatch(tt.partition, tt.level)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, match)
				require.Equal(t, tt.wantName, name)
			}
		})
	}
}

func TestPartitionPrefix(t *testing.T) {
	tests := []struct {
		partition []byte
		want      []byte
	}{
		{nil, []byte{0x00, 0x00}},
		{[]byte{}, []byte{0x00, 0x00}},
		{[]byte{0xab}, []byte{0xab, 0x00}},
		{[]byte{0xab, 0xcd}, []byte{0xab, 0xcd}},
		{[]byte{0xab, 0xcd, 0xef}, []byte{0xab, 0xcd}},
	}
	for _, tt := range tests {
		t.Run(hex.EncodeToString(tt.partition), func(t *testing.T) {
			require.Equal(t, tt.want, PartitionPrefix(tt.partition))
		})
	}
}

func TestPartitionRange(t *testing.T) {
	tests := []struct {
		partition []byte
		level     int
		wantLow   uint16
		wantHigh  uint16
		wantErr   bool
	}{
		{[]byte{0b1001_0110, 0b1100_0011}, 0, 0, 0xFFFF, false},
		{[]byte{0b0000_0000, 0x00}, 2, 0, 16383, false},
		{[]byte{0b0100_0000, 0x00}, 2, 16384, 32767, false},
		{[]byte{0b1000_0000, 0x00}, 2, 32768, 49151, false},
		{[]byte{0b1100_0000, 0x00}, 2, 49152, 65535, false},
		{[]byte{0b1001_0110, 0b1100_0011}, 3, num(0b1000_0000, 0b0000_0000), num(0b1001_1111, 0b1111_1111), false},
		{[]byte{0b1001_0110, 0b1100_0011}, 4, num(0b1001_0000, 0b0000_0000), num(0b1001_1111, 0b1111_1111), false},
		{[]byte{0b1001_0110, 0b1100_0011}, 8, num(0b1001_0110, 0b0000_0000), num(0b1001_0110, 0b1111_1111), false},
		{[]byte{0b1001_0110, 0b1100_0011}, 9, num(0b1001_0110, 0b1000_0000), num(0b1001_0110, 0b1111_1111), false},
		{[]byte{0b1001_0110, 0b1100_0011}, 10, num(0b1001_0110, 0b1100_0000), num(0b1001_0110, 0b1111_1111), false},
		{[]byte{0b1001_0110, 0b1100_0011}, 16, num(0b1001_0110, 0b1100_0011), num(0b1001_0110, 0b1100_0011), false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%x,%d,[%d-%d]", tt.partition, tt.level, tt.wantLow, tt.wantHigh), func(t *testing.T) {
			gotLow, gotHigh, err := PartitionRange(tt.partition, tt.level)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantLow, gotLow, "invalid low value: expected %d got %d", tt.wantLow, gotLow)
				require.Equal(t, tt.wantHigh, gotHigh, "invalid high value: expected %d got %d", tt.wantHigh, gotHigh)
			}
		})
	}
}

func num(bts ...byte) uint16 {
	return binary.BigEndian.Uint16(bts)
}

func TestPartitionPrefixNum(t *testing.T) {
	tests := []struct {
		digest []byte
		want   uint16
	}{
		{nil, 0},
		{[]byte{}, 0},
		{[]byte{0xab}, 0xab00},
		{[]byte{0xab, 0xcd}, 0xabcd},
		{[]byte{0xab, 0xcd, 0xef}, 0xabcd},
	}
	for _, tt := range tests {
		t.Run(hex.EncodeToString(tt.digest), func(t *testing.T) {
			require.EqualValues(t, tt.want, PartitionPrefixNum(tt.digest))
		})
	}
}

func TestPartitionPrefixes(t *testing.T) {
	for level := 0; level < 16; level++ {
		numPartitions := 1 << uint(level)
		t.Run(fmt.Sprint("level-", level, "-num-", numPartitions), func(t *testing.T) {
			prefixes, err := PartitionPrefixes(level)
			require.NoError(t, err)
			require.Equal(t, numPartitions, len(prefixes))

			for i, prefix := range prefixes {
				match, err := PartitionMatch(prefix, level)
				require.NoError(t, err)
				require.Equal(t, match, prefix)

				idx, err := PartitionIndex(prefix, level)

				// fmt.Printf("level: %d, i: %d, prefix: %x/%08[3]b, match: %x/%08[4]b, idx: %d\n", level, i, prefix, match, idx)

				require.NoError(t, err)
				require.Equal(t, i, idx, "invalid index for prefix %x/%08[1]b", prefix)
			}
		})
	}
	t.Run("level-17-error", func(t *testing.T) {
		_, err := PartitionPrefixes(17)
		require.Error(t, err)
	})
}

func TestPartitionIndex(t *testing.T) {
	tests := []struct {
		digest []byte
		level  int
		want   int
	}{
		{nil, 0, 0},
		{[]byte{0b0000_0000}, 0, 0},
		{[]byte{0b0000_0000}, 1, 0},
		{[]byte{0b0000_0000}, 2, 0},
		{[]byte{0b0000_0000}, 3, 0},
		{[]byte{0b0000_0000}, 8, 0},
		{[]byte{0b0000_0000}, 9, 0},
		{[]byte{0b0000_0000}, 16, 0},
		{[]byte{0b1000_0000}, 0, 0},
		{[]byte{0b1000_0000}, 1, 1},
		{[]byte{0b1000_0000}, 2, 2},
		{[]byte{0b1100_0000}, 2, 3},
		{[]byte{0b1100_0000, 0b0110_1111}, 6, 0b11_0000},
		{[]byte{0b1100_0000, 0b0110_1111}, 9, 0b1_1000_0000},
		{[]byte{0b1100_0000, 0b0110_1111}, 10, 0b11_0000_0001},
		{[]byte{0b1100_0000, 0b0110_1111}, 14, 0b11_0000_0001_1011},
		{[]byte{0xab, 0xcd, 0xef}, 8, 0xab},
		{[]byte{0xab, 0xcd, 0xef}, 16, 0xabcd},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%08b,lvl=%d,idx=%d", tt.digest, tt.level, tt.want), func(t *testing.T) {
			idx, err := PartitionIndex(tt.digest, tt.level)
			require.NoError(t, err)
			require.Equal(t, tt.want, idx)
		})
	}
	t.Run("level-17-error", func(t *testing.T) {
		_, err := PartitionIndex(nil, 17)
		require.Error(t, err)
	})

}

func TestAdjustLevel(t *testing.T) {
	tests := []struct {
		level     int
		num       int
		wantLevel int
		wantErr   bool
	}{
		{-1, 1, 0, true},
		{17, 1, 0, true},
		{0, 0, 0, true},
		{0, -1, 0, true},
		{0, 3, 0, true},
		{0, 6, 0, true},
		{0, 1, 0, false},
		{1, 1, 1, false},
		{1, 2, 0, false},
		{2, 4, 0, false},
		{3, 4, 1, false},
		{16, 4, 14, false},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("level-%d-num-%d", test.level, test.num), func(t *testing.T) {
			level, err := AdjustLevel(test.level, test.num)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.wantLevel, level)
			}
		})
	}
}
