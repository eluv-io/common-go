package preamble_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
	"math/rand"
	"testing"
	"time"

	"github.com/eluv-io/errors-go"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/preamble"
	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/stringutil"
)

var data, data2 []byte
var format, format2 string
var size, size2 int64

func init() {
	rand.Seed(time.Now().UnixNano())

	data = []byte("{\"hello\":\"world\"}")
	format = "/json"
	size = 25

	data2 = []byte(stringutil.RandomString(rand.Intn(10240)))
	format2 = "/string"
	size2 = 9 + int64(len(data2))
	size2 += int64(binary.PutUvarint(make([]byte, binary.MaxVarintLen64), uint64(size2)))
}

func TestPreambleWrite(t *testing.T) {
	buf := &bytes.Buffer{}
	n, err := preamble.Write(buf, data, format)
	require.NoError(t, err)
	require.Equal(t, size, n)
	preambleData, preambleFormat, preambleSize, err := preamble.Read(bytes.NewReader(buf.Bytes()), true)
	require.NoError(t, err)
	require.Equal(t, data, preambleData)
	require.Equal(t, format, preambleFormat)
	require.Equal(t, size, preambleSize)

	buf = &bytes.Buffer{}
	n, err = preamble.Write(buf, data, format[1:])
	require.NoError(t, err)
	require.Equal(t, size, n)
	preambleData, preambleFormat, preambleSize, err = preamble.Read(bytes.NewReader(buf.Bytes()), true)
	require.NoError(t, err)
	require.Equal(t, data, preambleData)
	require.Equal(t, format, preambleFormat)
	require.Equal(t, size, preambleSize)

	buf = &bytes.Buffer{}
	n, err = preamble.Write(buf, data)
	require.NoError(t, err)
	require.Equal(t, size-1, n)
	preambleData, preambleFormat, preambleSize, err = preamble.Read(bytes.NewReader(buf.Bytes()), true)
	require.NoError(t, err)
	require.Equal(t, data, preambleData)
	require.Equal(t, "/raw", preambleFormat)
	require.Equal(t, size-1, preambleSize)
}

func TestPreambleRead(t *testing.T) {
	buf := &bytes.Buffer{}
	n, err := preamble.Write(buf, data2, format2)
	require.NoError(t, err)
	require.Equal(t, size2, n)
	str := stringutil.RandomString(rand.Intn(10240))
	n2, err := buf.WriteString(str)
	require.NoError(t, err)
	require.Equal(t, len(str), n2)
	rdr := bytes.NewReader(buf.Bytes())

	preambleData, preambleFormat, preambleSize, err := preamble.Read(rdr, true)
	require.NoError(t, err)
	require.Equal(t, data2, preambleData)
	require.Equal(t, format2, preambleFormat)
	require.Equal(t, size2, preambleSize)
	b, err := ioutil.ReadAll(rdr)
	require.NoError(t, err)
	require.Equal(t, str, string(b))

	_, _, _, err = preamble.Read(rdr, true)
	require.Error(t, err)
	require.True(t, errors.IsKind(errors.K.Invalid, err))

	preambleData, preambleFormat, preambleSize, err = preamble.Read(rdr, false, size2)
	require.NoError(t, err)
	require.Equal(t, data2, preambleData)
	require.Equal(t, format2, preambleFormat)
	require.Equal(t, size2, preambleSize)
	off, err := rdr.Seek(0, io.SeekCurrent)
	require.Equal(t, size2+int64(len(str)), off)

	_, _, _, err = preamble.Read(rdr, false, int64(len(data2)))
	require.Error(t, err)
	require.True(t, errors.IsNotExist(err))

	for i := 0; i < 64; i++ {
		_, _, _, err = preamble.Read(bytes.NewReader(byteutil.RandomBytes(rand.Intn(10240))), true, 10240)
		require.Error(t, err)
		require.True(t, errors.IsNotExist(err))
	}
}

func TestPreambleSeek(t *testing.T) {
	buf := &bytes.Buffer{}
	n, err := preamble.Write(buf, data2, format2)
	require.NoError(t, err)
	require.Equal(t, size2, n)
	str := ""
	for len(str) < 4 {
		str = stringutil.RandomString(rand.Intn(10240))
	}
	n2, err := buf.WriteString(str)
	require.NoError(t, err)
	require.Equal(t, len(str), n2)
	rdr := bytes.NewReader(buf.Bytes())
	off := int64(0)

	off, err = preamble.Seek(rdr, size2, -1, off, int64(len(str)/2), io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(len(str)/2), off)
	realOff, err := rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2+int64(len(str)/2), realOff)

	off, err = preamble.Seek(rdr, size2, -1, off, int64(len(str)/4), io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, int64(len(str)/2)+int64(len(str)/4), off)
	realOff, err = rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2+int64(len(str)/2)+int64(len(str)/4), realOff)

	off, err = preamble.Seek(rdr, size2, -1, off, int64(-len(str)/4), io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(len(str))-int64(len(str)/4), off)
	realOff, err = rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2+int64(len(str))-int64(len(str)/4), realOff)

	off, err = preamble.Seek(rdr, size2, int64(len(str)), off, int64(-len(str)), io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(0), off)
	realOff, err = rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2, realOff)

	_, err = preamble.Seek(rdr, size2, -1, off, int64(-len(str)-1), io.SeekEnd)
	require.Error(t, err)
	realOff, err = rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2, realOff)

	_, err = preamble.Seek(rdr, size2, int64(len(str)), off, int64(-len(str)-1), io.SeekEnd)
	require.Error(t, err)
	realOff, err = rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2, realOff)

	_, err = preamble.Seek(rdr, size2, int64(len(str)), off, size2+int64(len(str)+1), io.SeekStart)
	require.Error(t, err)
	realOff, err = rdr.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, size2, realOff)
}

func TestPreambleSizer(t *testing.T) {
	buf := &bytes.Buffer{}
	n, err := preamble.Write(buf, data, format)
	require.NoError(t, err)
	require.Equal(t, size, n)
	str := stringutil.RandomString(rand.Intn(10240))
	n2, err := buf.WriteString(str)
	require.NoError(t, err)
	require.Equal(t, len(str), n2)
	szr := preamble.NewSizer()
	n2, err = szr.Write(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, size+int64(len(str)), int64(n2))
	sz, err := szr.Size()
	require.NoError(t, err)
	require.Equal(t, size, sz)

	buf = &bytes.Buffer{}
	n, err = preamble.Write(buf, data2, format2)
	require.NoError(t, err)
	require.Equal(t, size2, n)
	str = stringutil.RandomString(rand.Intn(10240))
	n2, err = buf.WriteString(str)
	require.NoError(t, err)
	require.Equal(t, len(str), n2)
	szr = preamble.NewSizer()
	n2, err = szr.Write(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, size2+int64(len(str)), int64(n2))
	sz, err = szr.Size()
	require.NoError(t, err)
	require.Equal(t, size2, sz)
}
