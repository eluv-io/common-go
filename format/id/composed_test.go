package id

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompose(t *testing.T) {
	fullID := NewID(TQ, []byte{1, 1, 1})
	require.Equal(
		t,
		Composed{
			full:     fullID,
			primary:  NewID(Q, []byte{1}),
			embedded: NewID(Tenant, []byte{1}),
			strCache: fullID.String(),
		},
		Compose(TQ, []byte{1}, []byte{1}),
	)
	fmt.Println(fullID, len(fullID.String()))

	fullID = NewID(TQ, []byte{2, 55, 99, 1, 2, 3, 4})
	require.Equal(
		t,
		Composed{
			full:     fullID,
			primary:  NewID(Q, []byte{1, 2, 3, 4}),
			embedded: NewID(Tenant, []byte{55, 99}),
			strCache: fullID.String(),
		},
		Compose(TQ, []byte{1, 2, 3, 4}, []byte{55, 99}),
	)
	fmt.Println(fullID, len(fullID.String()))

	qid := Generate(Q)
	tid := Generate(Tenant)
	fullID = NewID(TQ, append(append([]byte{16}, tid.Bytes()...), qid.Bytes()...))
	require.Equal(
		t,
		Composed{
			full:     fullID,
			primary:  qid,
			embedded: tid,
			strCache: fullID.String(),
		},
		Compose(TQ, qid.Bytes(), tid.Bytes()),
	)
	fmt.Println(fullID, len(fullID.String()))
}

func TestComposeInvalid(t *testing.T) {
	qid := Generate(Q)
	tid := Generate(Tenant)
	tests := []struct {
		src  ID
		want Composed
	}{
		{qid, Composed{full: qid, primary: qid, strCache: qid.String()}},
		{tid, Composed{full: tid, primary: tid, strCache: tid.String()}},
	}

	for _, test := range tests {
		t.Run(test.src.String(), func(t *testing.T) {
			require.Equal(t, test.want, Compose(test.src.Code(), test.src.Bytes(), tid))
		})
	}
}

func TestDecompose(t *testing.T) {
	tests := []ID{
		Compose(TLib, []byte{1}, []byte{1}).ID(),
		Compose(TLib, []byte{1, 2, 3}, []byte{4, 5, 6}).ID(),
		Compose(TLib, Generate(Q), Generate(Tenant)).ID(),
		nil,
	}

	for _, test := range tests {
		t.Run(test.String(), func(t *testing.T) {
			require.Equal(t, test, Decompose(test).ID())
		})
	}
}

func TestDecomposeInvalid(t *testing.T) {
	qid := Generate(Q)
	tid := Generate(Tenant)
	tests := []struct {
		src  ID
		want Composed
	}{
		{nil, Composed{}},
		{qid, Composed{full: qid, primary: qid, strCache: qid.String()}},
		{tid, Composed{full: tid, primary: tid, strCache: tid.String()}},
	}

	for _, test := range tests {
		t.Run(test.src.String(), func(t *testing.T) {
			require.Equal(t, test.want, Decompose(test.src))
		})
	}
}
