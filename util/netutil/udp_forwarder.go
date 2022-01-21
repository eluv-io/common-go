package netutil

import (
	"net"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"

	"github.com/qluvio/content-fabric/util/filterutil"
)

func NewUdpForwarder(listenAddress string, targetAddress string, filter filterutil.Filter) *UdpForwarder {
	return &UdpForwarder{
		listedAddress: listenAddress,
		targetAddress: targetAddress,
		filter:        filter,
		log:           log.Get("/"),
	}
}

type UdpForwarder struct {
	listedAddress string
	targetAddress string
	filter        filterutil.Filter
	listenConn    net.PacketConn
	targetConn    *net.UDPConn
	log           *log.Log
}

func (f *UdpForwarder) SetLog(l *log.Log) {
	f.log = l
}

func (f *UdpForwarder) Start() error {
	e := errors.Template("start")

	var err error
	f.listenConn, err = net.ListenPacket("udp", f.listedAddress)
	if err != nil {
		return e(err)
	}

	targetAddr, err := net.ResolveUDPAddr("udp", f.targetAddress)
	if err != nil {
		return e(err)
	}

	f.targetConn, err = net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		return e(err)
	}

	go f.loop()

	return nil
}

func (f UdpForwarder) Stop() {
	_ = f.listenConn.Close()
}

func (f *UdpForwarder) loop() {
	buf := make([]byte, 1_000_000)
	prevAccept := true
	count := 0

	for {
		n, _, err := f.listenConn.ReadFrom(buf)
		if err != nil {
			f.log.Error("failed to read UDP packet - bailing out", err)
			return
		}

		count++
		accept := f.filter.Filter()
		if !accept {
			if accept != prevAccept {
				f.log.Info("dropping UDP packets", "accepted", count)
				prevAccept = accept
				count = 1
			}
			continue
		}

		if accept != prevAccept {
			f.log.Info("forwarding UDP packets", "dropped", count)
			prevAccept = accept
			count = 1
		}
		n, err = f.targetConn.Write(buf[:n])
		if err != nil {
			f.log.Error("failed to send UDP packet - bailing out", err)
			return
		}
	}
}

func (f *UdpForwarder) ListenPort() int {
	if f.listenConn == nil {
		return 0
	}
	return f.listenConn.LocalAddr().(*net.UDPAddr).Port
}
