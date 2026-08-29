package pktpool_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/eluv-io/common-go/media/pktpool"
	"github.com/eluv-io/common-go/media/tlv"
	"github.com/eluv-io/common-go/util/testutil"
)

func TestPacket(t *testing.T) {
	path, err := testutil.AssetsPath(2)
	if err != nil {
		t.Skip("skipping test: ", err)
	}
	source, err := os.ReadFile(filepath.Join(path, "media", "mpeg-ts", "tlv-rtp-ts-segment-00001.ts"))
	require.NoError(t, err)

	pool := pktpool.NewPacketPool(0, 1500)

	eg := errgroup.Group{}

	sinks := []*sink{
		&sink{in: make(chan pktpool.Resource, 10), stripRtp: false},
		&sink{in: make(chan pktpool.Resource, 10), stripRtp: true},
	}

	for _, s := range sinks {
		eg.Go(s.run)
	}

	packetizer := tlv.NewTlvPacketizer(1500)
	packetizer.Write(source)
	wantPacketCount := 0
	for {
		bts, err := packetizer.Next()
		require.NoError(t, err)
		if len(bts) == 0 {
			for _, s := range sinks {
				close(s.in)
			}
			break
		}
		wantPacketCount++

		pkt := pool.Borrow()
		require.NoError(t, pkt.T.From(bts))
		for _, s := range sinks {
			pkt.Reference()
			s.in <- pkt
		}
		pkt.Release()
	}

	require.NoError(t, eg.Wait())

	for _, s := range sinks {
		require.Equal(t, wantPacketCount, s.packetCount)
	}
}

type sink struct {
	in          chan pktpool.Resource
	stripRtp    bool
	packetCount int
	data        []byte
}

func (s *sink) run() error {
	for resource := range s.in {
		s.packetCount++

		if s.stripRtp {
			rtp, err := resource.T.Rtp()
			if err != nil {
				return err
			}
			s.data = append(s.data, rtp.Payload...)
		} else {
			s.data = append(s.data, resource.T.Data...)
		}

		resource.Release()
	}
	return nil
}
