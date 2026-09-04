//go:build testing

// testing flag, because it uses the test assets

package mpegts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/testutil"
)

func TestTsValidator(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testutil.AssetsPathT(t, 2), "media", "mpeg-ts", "ts-segment.ts"))
	require.NoError(t, err)

	t.Run("valid", func(t *testing.T) {
		validator := NewTsValidator()
		require.NoError(t, validator.Validate(source))
	})
	t.Run("short packet", func(t *testing.T) {
		validator := NewTsValidator()
		require.ErrorContains(t, validator.Validate(source[len(source)-180:]), "short")
		require.ErrorContains(t, validator.Validate(source[len(source)-200:]), "short")
	})
	t.Run("invalid sync byte", func(t *testing.T) {
		validator := NewTsValidator()
		pkt := make([]byte, 188)
		copy(pkt, source[:188])
		pkt[0] = 0x2F
		require.ErrorContains(t, validator.Validate(pkt), "sync byte is not valid")
	})
	t.Run("invalid continuity counter", func(t *testing.T) {
		validator := NewTsValidator()

		p1 := packet.New()
		p1.SetPID(1)
		_, _ = p1.SetPayload(byteutil.RandomBytes(20))
		p1.SetContinuityCounter(4)

		p2 := packet.New()
		p2.SetPID(1)
		_, _ = p2.SetPayload(byteutil.RandomBytes(20))
		p2.SetContinuityCounter(15)

		p3 := packet.New()
		p3.SetPID(1)
		_, _ = p3.SetPayload(byteutil.RandomBytes(20))
		p3.SetContinuityCounter(0)

		require.NoError(t, validator.Validate(p1[:]))
		require.ErrorContains(t, validator.Validate(p2[:]), "continuity counter mismatch")
		require.NoError(t, validator.Validate(p3[:]))
	})
}
