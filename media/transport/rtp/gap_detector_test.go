package rtp_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/transport/rtp"
)

func TestGapDetector_Detect(t *testing.T) {
	detector := rtp.NewRtpGapDetector(1, time.Second)
	tests := []struct {
		seq        uint16
		ts         uint32
		wantSeq    int64
		wantTs     int64
		wantSeqErr bool
		wantTsErr  bool
	}{
		{0, 0, 0, 0, false, false},
		{0, 0, 0, 0, false, false}, // should no change in RTP sequence number trigger an error?
		{1, 0, 1, 0, false, false},
		{2, 0, 2, 0, false, false},
		{3, 0, 3, 0, false, false},
		{6, 0, 6, 0, true, false},                  // missing 4 & 5
		{4, 0, 4, 0, true, false},                  // jump back
		{32768, 0, 32768, 0, true, false},          // huge jump ahead (but less than 32768)
		{65530, 0, 65530, 0, true, false},          // huge jump ahead (but less than 32768)
		{65531, 0, 65531, 0, false, false},         // no gap
		{0, 0, 65536, 0, true, false},              // gap with wrap
		{65532, 0, 65532, 0, true, false},          // back across wrap
		{1, 0, 65537, 0, true, false},              // forward across wrap
		{2, 0, 65538, 0, false, false},             // no gap
		{3, 100_000, 65539, 100_000, false, true},  // timestamp gap
		{4, 110_000, 65540, 110_000, false, false}, // no gap
		{5, 120_000, 65541, 120_000, false, false}, // no gap
		{6, 115_000, 65542, 115_000, false, false}, // ts going backwards, but no gap
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.seq, tt.ts), func(t *testing.T) {
			gotSeq, gotTs, err := detector.Detect(tt.seq, tt.ts)
			require.Equal(t, tt.wantSeq, gotSeq)
			require.Equal(t, tt.wantTs, gotTs)
			if tt.wantSeqErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "sequence number")
			}
			if tt.wantTsErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "timestamp")
			}
			if !tt.wantSeqErr && !tt.wantTsErr {
				require.NoError(t, err, "")
			}
		})
	}
}
