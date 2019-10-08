package codecs

import (
	mc "github.com/multiformats/go-multicodec"
	// mux "github.com/multiformats/go-multicodec/mux"
)

// var standardMux mc.Multicodec = mux.MuxMulticodec([]mc.Multicodec{
// 	NewGobCodec(),
// 	NewBase64Codec(),
// }, mux.SelectFirst)

// MdsImexCodec returns the codec for metadata store exports / imports.
func MdsImexCodec() mc.Multicodec {
	return NewGobCodec()
	// return NewCborCodec()
}
