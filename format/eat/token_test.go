package eat_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eluv-io/utc-go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/eat"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/ethutil"
	"github.com/eluv-io/common-go/util/jsonutil"
)

var (
	sid                  = id.MustParse("ispc2gfzuWxi2krZv2SqkNz3f6UpMbJe")
	lid                  = id.MustParse("ilib3RiwiP7UJJiHxFLbkL46BoVfKWrB")
	qid                  = id.MustParse("iq__3RiwiP7UJJiHxFLbkL46BoVfKWrB")
	clientSK, clientAddr = func() (*ecdsa.PrivateKey, common.Address) {
		sk, _ := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		return sk, addr
	}()
	serverSK, serverAddr = func() (*ecdsa.PrivateKey, common.Address) {
		sk, _ := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		return sk, addr
	}()
	txh = common.HexToHash("0xb478281f7481e1aba09d6f5e34403ccaab451b4f8f61e47b39d4b47e16ec8517")
)

var baseUnsignedToken = &eat.Token{
	Type:    eat.Types.Anonymous(),
	SigType: eat.SigTypes.Unsigned(),
	Format:  eat.Formats.Json(),
	TokenData: eat.TokenData{
		SID: sid,
		LID: lid,
		QID: qid,
		//EthAddr: clientAddr,
		//EthTxHash: txh.Bytes(),
		//GrantType: "read",
		//IssuedAt:  utc.Now().Truncate(time.Second),                // legacy format has second precision
		//Expires:   utc.Now().Truncate(time.Second).Add(time.Hour), // legacy format has second precision
		//Ctx: map[string]interface{}{
		//	"key1": "val1",
		//	"key2": "val2",
		//},
	},
}
var baseSignedToken = &eat.Token{
	Type:    eat.Types.Tx(),
	SigType: eat.SigTypes.Unsigned(),
	Format:  eat.Formats.Json(),
	TokenData: eat.TokenData{
		SID: sid,
		LID: lid,
		//EthAddr:   clientAddr,
		EthTxHash: txh,
		//Ctx:       map[string]interface{}{},
		//QID:       qid,
		//GrantType: "read",
		//IssuedAt:  utc.Now().Truncate(time.Second),                // legacy format has second precision
		//Expires:   utc.Now().Truncate(time.Second).Add(time.Hour), // legacy format has second precision
		//Ctx: map[string]interface{}{
		//	"key1": "val1",
		//	"key2": "val2",
		//},
	},
}

func TestFormatParse(t *testing.T) {
	tokens := map[string][]*eat.Token{
		"unsigned": {
			baseUnsignedToken.With(eat.Formats.Legacy()),
			baseUnsignedToken.With(eat.Formats.Json()),
			baseUnsignedToken.With(eat.Formats.JsonCompressed()),
			baseUnsignedToken.With(eat.Formats.Cbor()),
			baseUnsignedToken.With(eat.Formats.CborCompressed()),
			baseUnsignedToken.With(eat.Formats.Custom()),
		},
		"signed": {
			sign(t, baseSignedToken.With(eat.Formats.Legacy())),
			sign(t, baseSignedToken.With(eat.Formats.Json())),
			sign(t, baseSignedToken.With(eat.Formats.JsonCompressed())),
			sign(t, baseSignedToken.With(eat.Formats.Cbor())),
			sign(t, baseSignedToken.With(eat.Formats.CborCompressed())),
			sign(t, baseSignedToken.With(eat.Formats.Custom())),
		},
	}

	for setName, tokenSet := range tokens {
		t.Run(setName, func(t *testing.T) {
			sizes := make([]int, 0)
			for _, token := range tokenSet {
				t.Run(token.SigType.Name+" "+token.Format.Name, func(t *testing.T) {
					s, err := token.Encode()
					require.NoError(t, err)
					fmt.Println(token.Format.Name, len(s), s)

					tok, err := eat.FromString(s)
					require.NoError(t, err)
					require.NotNil(t, tok)
					require.Equal(t, token.TokenData, tok.TokenData)
					require.Equal(t, sid, tok.TokenData.SID)
					require.Equal(t, lid, tok.TokenData.LID)
					if token.Format != eat.Formats.Legacy() {
						require.Equal(t, token, tok, "expected type: %s, actual type: %s", token.Type, tok.Type)
					}
					sizes = append(sizes, len(s))
				})
			}
			if len(sizes) == len(tokenSet) {
				// otherwise there must have been a test failure...
				fmt.Println(jsonutil.MarshalString(baseUnsignedToken.TokenData))
				for i, token := range tokenSet {
					fmt.Printf("%-15s %db = %.0f%%\n", token.Format.Name, sizes[i], 100/float64(sizes[0])*float64(sizes[i]))
				}
			}
		})
	}

}

func TestLegacyTokens(t *testing.T) {
	tests := []struct {
		token       string
		wantType    eat.TokenType
		validate    func(t *eat.Token)
		trustedAddr string
	}{
		{
			/*
				{
				  "qspace_id": "ispc2TkJzkzLmkvpvH9xnSjKwb88mx3h",
				  "qlib_id": "ilib3hBr5NxcsEtqb2pb5ySTXqigfkHt",
				  "addr": "0xBBBA843E88DfFA1b7aF9388BAcDb52607Ed88d2E",
				  "qid": "iq__4BrYtKsWA6hRBvc55KMQB4rh16r3",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1599212641,
				  "exp": 1599216241,
				  "auth_sig": "ES256K_E1R5sm3zd9G1hhqt5DAVeanbrqB58FgBX69aLmCxcemQR9zXYqnZncT7TK5S15aG5sKTApJb2iyiHxJp1HavBc97D",
				  "afgh_pk": "ktpkAU7S6eBo3yycNSTFVJ9Msjqw6F24KvpDSBrN1EqAW3dd71RW9PbqpwULTc6797fDgB1iSg9Gjvs8nuZbRAkjRrQewXN11gAf8jbEZyoLLVR4T3S6dLK34mmfBGK41x2qksMqVdS8gdAZuoj41zDGLN6gvHbWNy6tnT3K9o2FKHuF6hFQDjaxd5f5huZ66Zxkg8qRQfS49Htue8xrxMWV7nWeVU8rbDBZsu8aSvBfQzZzu1A4jcyU2J5tRn17gqiC216NbnZ"
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjMlRrSnprekxta3Zwdkg5eG5Takt3Yjg4bXgzaCIsInFsaWJfaWQiOiJpbGliM2hCcjVOeGNzRXRxYjJwYjV5U1RYcWlnZmtIdCIsImFkZHIiOiIweEJCQkE4NDNFODhEZkZBMWI3YUY5Mzg4QkFjRGI1MjYwN0VkODhkMkUiLCJxaWQiOiJpcV9fNEJyWXRLc1dBNmhSQnZjNTVLTVFCNHJoMTZyMyIsImdyYW50IjoicmVhZCIsInR4X3JlcXVpcmVkIjpmYWxzZSwiaWF0IjoxNTk5MjEyNjQxLCJleHAiOjE1OTkyMTYyNDEsImF1dGhfc2lnIjoiRVMyNTZLX0UxUjVzbTN6ZDlHMWhocXQ1REFWZWFuYnJxQjU4RmdCWDY5YUxtQ3hjZW1RUjl6WFlxblpuY1Q3VEs1UzE1YUc1c0tUQXBKYjJpeWlIeEpwMUhhdkJjOTdEIiwiYWZnaF9wayI6Imt0cGtBVTdTNmVCbzN5eWNOU1RGVko5TXNqcXc2RjI0S3ZwRFNCck4xRXFBVzNkZDcxUlc5UGJxcHdVTFRjNjc5N2ZEZ0IxaVNnOUdqdnM4bnVaYlJBa2pSclFld1hOMTFnQWY4amJFWnlvTExWUjRUM1M2ZExLMzRtbWZCR0s0MXgycWtzTXFWZFM4Z2RBWnVvajQxekRHTE42Z3ZIYldOeTZ0blQzSzlvMkZLSHVGNmhGUURqYXhkNWY1aHVaNjZaeGtnOHFSUWZTNDlIdHVlOHhyeE1XVjduV2VWVThyYkRCWnN1OGFTdkJmUXpaenUxQTRqY3lVMko1dFJuMTdncWlDMjE2TmJuWiJ9.RVMyNTZLX0Rwc244SHpSTktndU1RNHpub0tvRng1c2ZEbVFrQ0ZCcVVEUlRmUDFCZUtrVU5VQjkyanFCcmh3cUR2ZzdqOFVTS2d5NWEyTVRGWXdKaDJNM2RuSGIzSjU2`,
			wantType: eat.Types.Client(),
		},
		{
			/*
				{
				  "qspace_id": "ispcgW66XjmbyfkJM4B6pRnJKA6Sxpx",
				  "qlib_id": "ilib27XDePAvVozZ1gN3EPX3Tq4Ng2gM",
				  "addr": "0x65419C9f653703ED7Fb6CC636cf9fda6cC024E2e",
				  "qid": "iq__3tHBnWZwZDafP1yuXurTUQGAYzXz",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1588242373,
				  "exp": 1588328773,
				  "auth_sig": "ES256K_J64bAKa6xrBPEvYYwGJdQ66t4ZqmuUieeh5xCgJ1YecmQTVF7KesS5NP9CgBLZg5XFHYb7Nj1PpUgJGqRhATYRU16",
				  "afgh_pk": ""
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjZ1c2NlhqbWJ5ZmtKTTRCNnBSbkpLQTZTeHB4IiwicWxpYl9pZCI6ImlsaWIyN1hEZVBBdlZveloxZ04zRVBYM1RxNE5nMmdNIiwiYWRkciI6IjB4NjU0MTlDOWY2NTM3MDNFRDdGYjZDQzYzNmNmOWZkYTZjQzAyNEUyZSIsInFpZCI6ImlxX18zdEhCbldad1pEYWZQMXl1WHVyVFVRR0FZelh6IiwiZ3JhbnQiOiJyZWFkIiwidHhfcmVxdWlyZWQiOmZhbHNlLCJpYXQiOjE1ODgyNDIzNzMsImV4cCI6MTU4ODMyODc3MywiYXV0aF9zaWciOiJFUzI1NktfSjY0YkFLYTZ4ckJQRXZZWXdHSmRRNjZ0NFpxbXVVaWVlaDV4Q2dKMVllY21RVFZGN0tlc1M1TlA5Q2dCTFpnNVhGSFliN05qMVBwVWdKR3FSaEFUWVJVMTYiLCJhZmdoX3BrIjoiIn0=.RVMyNTZLX0VaZmVMcVNkSHA1d1kyaDlZNEZYajc0dTJYM0FnWEJrOXhCUEJmN2NOdVI2M3JxTFMzRldmU2NBU3VSZzV6WUhhR29QMjRXcGtTemQyN3pYc1lyRzViYWZI`,
			wantType: eat.Types.Client(),
		},
		{
			/*
				{
				  "qspace_id": "ispc2gfzuWxi2krZv2SqkNz3f6UpMbJe",
				  "qlib_id": "ilib3RiwiP7UJJiHxFLbkL46BoVfKWrB",
				  "addr": "0xBB1039015306e4239c844F47Ce0655f27B6744ae",
				  "tx_id": "0xb478281f7481e1aba09d6f5e34403ccaab451b4f8f61e47b39d4b47e16ec8517"
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjMmdmenVXeGkya3JadjJTcWtOejNmNlVwTWJKZSIsInFsaWJfaWQiOiJpbGliM1Jpd2lQN1VKSmlIeEZMYmtMNDZCb1ZmS1dyQiIsImFkZHIiOiIweEJCMTAzOTAxNTMwNmU0MjM5Yzg0NEY0N0NlMDY1NWYyN0I2NzQ0YWUiLCJ0eF9pZCI6IjB4YjQ3ODI4MWY3NDgxZTFhYmEwOWQ2ZjVlMzQ0MDNjY2FhYjQ1MWI0ZjhmNjFlNDdiMzlkNGI0N2UxNmVjODUxNyJ9.RVMyNTZLXzNYaGVEOEo3S2QxSnJRbWtjaVdMQzZZWjQxZ01RSlRTaDhxbUJxSGMzVnFBZ0o4UXFBa2JoVjNRQ2I2dmhZb1pXZXRXNFlwclR1UkVnMTNBeWsxcEI0OURx`,
			wantType: eat.Types.Tx(),
		},
		{
			/*
				{
				  "qspace_id": "ispcgW66XjmbyfkJM4B6pRnJKA6Sxpx",
				  "qlib_id": "ilib27XDePAvVozZ1gN3EPX3Tq4Ng2gM",
				  "addr": "0x65419C9f653703ED7Fb6CC636cf9fda6cC024E2e",
				  "qid": "iq__3tHBnWZwZDafP1yuXurTUQGAYzXz",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1588242373,
				  "exp": 1588328773,
				  "auth_sig": "ES256K_J64bAKa6xrBPEvYYwGJdQ66t4ZqmuUieeh5xCgJ1YecmQTVF7KesS5NP9CgBLZg5XFHYb7Nj1PpUgJGqRhATYRU16",
				  "afgh_pk": ""
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjZ1c2NlhqbWJ5ZmtKTTRCNnBSbkpLQTZTeHB4IiwicWxpYl9pZCI6ImlsaWIyN1hEZVBBdlZveloxZ04zRVBYM1RxNE5nMmdNIiwiYWRkciI6IjB4NjU0MTlDOWY2NTM3MDNFRDdGYjZDQzYzNmNmOWZkYTZjQzAyNEUyZSIsInFpZCI6ImlxX18zdEhCbldad1pEYWZQMXl1WHVyVFVRR0FZelh6IiwiZ3JhbnQiOiJyZWFkIiwidHhfcmVxdWlyZWQiOmZhbHNlLCJpYXQiOjE1ODgyNDIzNzMsImV4cCI6MTU4ODMyODc3MywiYXV0aF9zaWciOiJFUzI1NktfSjY0YkFLYTZ4ckJQRXZZWXdHSmRRNjZ0NFpxbXVVaWVlaDV4Q2dKMVllY21RVFZGN0tlc1M1TlA5Q2dCTFpnNVhGSFliN05qMVBwVWdKR3FSaEFUWVJVMTYiLCJhZmdoX3BrIjoiIn0=.RVMyNTZLX0VaZmVMcVNkSHA1d1kyaDlZNEZYajc0dTJYM0FnWEJrOXhCUEJmN2NOdVI2M3JxTFMzRldmU2NBU3VSZzV6WUhhR29QMjRXcGtTemQyN3pYc1lyRzViYWZI`,
			wantType: eat.Types.Client(),
		},
		{
			/*
				{
				  "qspace_id": "ispc2RUoRe9eR2v33HARQUVSp1rYXzw1",
				  "qlib_id": "ilib2XX6yS9S8bgAeLVxDGKeoNcNVckN",
				  "addr": "arun+roartest_servicingandscreening@eluv.io",
				  "qid": "iq__ozFMc2617cYhVXKbjfDu6z4p9YW",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1597716565,
				  "exp": 1597802965,
				  "ctx": {
				    "elv:groupIds": [
				      "igrp2pv9sraBiiYFwoo7tASg1mhmowgF",
				      "igrp2VmTWQ3srMGGBHJdDQaemvPKWxVG",
				      "igrpXtdeWtLKf59kEDd7zetKPuoLxmZ"
				    ]
				  },
				  "auth_sig": "ES256K_6UdUKxkPmKaTFVeox2Q117tsqV1BcYLGYatFWcAxgtyW1bvvwGkzU7Rrm84vhVbV9iSz3PcQr63sSFbvpRSVVmdt3",
				  "afgh_pk": ""
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjMlJVb1JlOWVSMnYzM0hBUlFVVlNwMXJZWHp3MSIsInFsaWJfaWQiOiJpbGliMlhYNnlTOVM4YmdBZUxWeERHS2VvTmNOVmNrTiIsImFkZHIiOiJhcnVuK3JvYXJ0ZXN0X3NlcnZpY2luZ2FuZHNjcmVlbmluZ0BlbHV2LmlvIiwicWlkIjoiaXFfX296Rk1jMjYxN2NZaFZYS2JqZkR1Nno0cDlZVyIsImdyYW50IjoicmVhZCIsInR4X3JlcXVpcmVkIjpmYWxzZSwiaWF0IjoxNTk3NzE2NTY1LCJleHAiOjE1OTc4MDI5NjUsImN0eCI6eyJlbHY6Z3JvdXBJZHMiOlsiaWdycDJwdjlzcmFCaWlZRndvbzd0QVNnMW1obW93Z0YiLCJpZ3JwMlZtVFdRM3NyTUdHQkhKZERRYWVtdlBLV3hWRyIsImlncnBYdGRlV3RMS2Y1OWtFRGQ3emV0S1B1b0x4bVoiXX0sImF1dGhfc2lnIjoiRVMyNTZLXzZVZFVLeGtQbUthVEZWZW94MlExMTd0c3FWMUJjWUxHWWF0RldjQXhndHlXMWJ2dndHa3pVN1JybTg0dmhWYlY5aVN6M1BjUXI2M3NTRmJ2cFJTVlZtZHQzIiwiYWZnaF9wayI6IiJ9`,
			wantType: eat.Types.Client(),
		},
		{
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjM3ZHcW8zdlVNVEJVQnV3cGt5VVFkVjNrcGNzIiwicWxpYl9pZCI6ImlsaWI0VHNTWHdDTmNEdW5ON1dVRzhuNUVVWFNYZzdHIiwiYWRkciI6IjB4MGY5ZkIxNTM4MmE3NjZlOTY4MDkwYjU3NEJiZURGYTBkNkM1ZGMzNSIsInFpZCI6ImlxX180NUVYa1hnUW9hcTlldkhyZUdrc0d1N3diU3E5IiwiZ3JhbnQiOiJyZWFkIiwidHhfcmVxdWlyZWQiOmZhbHNlLCJpYXQiOjE1OTkyMjk4NzUsImV4cCI6MTU5OTMxNjI3NSwiYXV0aF9zaWciOiJFUzI1NktfdmMxQmt4WlJwQ0pQOGM0ZE1BOHRBaFNGbnJvMlJOdUxpTTdWNHVoek55Vm9rZHdHU0tMWThpZnd3VUpaOXVhellCWDZDWHV4TmNCSmNzYmhzSFlCZWJpZiIsImFmZ2hfcGsiOiIifQ==.RVMyNTZLXzY0QkJ4Y3M4V0M5bUFkY2NMR1NTN3JTMkFzbUtwN2hwdXc2NmNwRGh4a3VtVlE3TTY2TTZ4U2RlZTM2Vlp2SkVmcFVMQVVHV0M2QUNxZ3dnb3J2a2MzTnVl`,
			wantType: eat.Types.Client(),
			validate: func(tok *eat.Token) {
				require.NotEmpty(t, tok.GetQSpaceID())
				require.NotEmpty(t, tok.GetQLibID())
			},
		},
		{
			/*
				{
				  "qspace_id": "ispc3ANoVSzNA3P6t7abLR69ho5YPPZU",
				  "qlib_id": "ilib3dRfgrru2ZE5YVZmjuKUiznSjkrv",
				  "addr": "0xeA3f7BFd1929aEa551B458aD4360943Dd4b52ac9",
				  "qid": "iq__4NZ2yykmn2ngnSpYM2ZS4jJTzkjz",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1602645936,
				  "exp": 1602667536,
				  "auth_sig": "ES256K_ABT1Eg663CkffhH68Edq9rFmaBQBRv2oKfQ6snBBLVy4ndifjSbW85uuaVudKcz197Vokhszk776b9vyDHipASNc7",
				  "afgh_pk": ""
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjM0FOb1ZTek5BM1A2dDdhYkxSNjlobzVZUFBaVSIsInFsaWJfaWQiOiJpbGliM2RSZmdycnUyWkU1WVZabWp1S1Vpem5TamtydiIsImFkZHIiOiIweGVBM2Y3QkZkMTkyOWFFYTU1MUI0NThhRDQzNjA5NDNEZDRiNTJhYzkiLCJxaWQiOiJpcV9fNE5aMnl5a21uMm5nblNwWU0yWlM0akpUemtqeiIsImdyYW50IjoicmVhZCIsInR4X3JlcXVpcmVkIjpmYWxzZSwiaWF0IjoxNjAyNjQ1OTM2LCJleHAiOjE2MDI2Njc1MzYsImF1dGhfc2lnIjoiRVMyNTZLX0FCVDFFZzY2M0NrZmZoSDY4RWRxOXJGbWFCUUJSdjJvS2ZRNnNuQkJMVnk0bmRpZmpTYlc4NXV1YVZ1ZEtjejE5N1Zva2hzems3NzZiOXZ5REhpcEFTTmM3IiwiYWZnaF9wayI6IiJ9.RVMyNTZLX040N2tOOFpYNkNlS2VMYWZneGdwUnUxZ1gyQkJYaUJjM25pVUtLRTY1dmVMemEyNmNQQmIyblpxZFp6N29jMUdEc2UxVzFDQUhZMVNOeTdFb1J0djh5Z2lq`,
			wantType: eat.Types.EditorSigned(),
			validate: func(tok *eat.Token) {
				require.NotEmpty(t, tok.GetQSpaceID())
				require.NotEmpty(t, tok.GetQLibID())
			},
		},
		{
			/*
				{
				  "qspace_id": "ispc2gfzuWxi2krZv2SqkNz3f6UpMbJe",
				  "qlib_id": "ilib3RiwiP7UJJiHxFLbkL46BoVfKWrB",
				  "addr": "0xBB1039015306e4239c844F47Ce0655f27B6744ae",
				  "qid": "iq__AHih6kf7MinbbDPEnT4XFxSMuQT",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1604073260,
				  "exp": 1604076860,
				  "auth_sig": "ES256K_FsNgeH8CgeGR4vKZunjW7ySMQ5TndX1YphUyqreKnrA6rEy6EZm1X3VH58i1evTjKASMnw4xiRC8j7o88xDJNnwgU",
				  "afgh_pk": ""
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjMmdmenVXeGkya3JadjJTcWtOejNmNlVwTWJKZSIsInFsaWJfaWQiOiJpbGliM1Jpd2lQN1VKSmlIeEZMYmtMNDZCb1ZmS1dyQiIsImFkZHIiOiIweEJCMTAzOTAxNTMwNmU0MjM5Yzg0NEY0N0NlMDY1NWYyN0I2NzQ0YWUiLCJxaWQiOiJpcV9fQUhpaDZrZjdNaW5iYkRQRW5UNFhGeFNNdVFUIiwiZ3JhbnQiOiJyZWFkIiwidHhfcmVxdWlyZWQiOmZhbHNlLCJpYXQiOjE2MDQwNzMyNjAsImV4cCI6MTYwNDA3Njg2MCwiYXV0aF9zaWciOiJFUzI1NktfRnNOZ2VIOENnZUdSNHZLWnVualc3eVNNUTVUbmRYMVlwaFV5cXJlS25yQTZyRXk2RVptMVgzVkg1OGkxZXZUaktBU01udzR4aVJDOGo3bzg4eERKTm53Z1UiLCJhZmdoX3BrIjoiIn0=.RVMyNTZLXzNLVkxYOXpLanRLTlJlVHhOalh6cFpwWXI4ZW9aYmNCNkQySFVja1JlS2k4Y05kdURhdFFhYUF2bWtRcVhOWWcxeTRYam1jb3pnckZiQ2F3MzFuakJHOVF0`,
			wantType: eat.Types.Client(),
			validate: func(tok *eat.Token) {
				require.NotEmpty(t, tok.GetQSpaceID())
				require.NotEmpty(t, tok.GetQLibID())
			},
			trustedAddr: "0xd9DC97B58C5f2584062Cf69775d160ed9A3BFbC4",
		},
		{
			/*
				{
				  "qspace_id": "ispc2gfzuWxi2krZv2SqkNz3f6UpMbJe",
				  "qlib_id": "ilib3RiwiP7UJJiHxFLbkL46BoVfKWrB",
				  "addr": "0xBB1039015306e4239c844F47Ce0655f27B6744ae",
				  "qid": "iq__AHih6kf7MinbbDPEnT4XFxSMuQT",
				  "grant": "read",
				  "tx_required": false,
				  "iat": 1604073261,
				  "exp": 1604076861,
				  "ip_geo": "US",
				  "auth_sig": "ES256K_Ld6bVTzuPdgYLBuY9qP3X2PJnKciAwXC8U1fnAoLvqDa6V2MvvLE4mTmXczLTtxLvbv8wbVEJXxYLtUMLb3VHagiL",
				  "afgh_pk": ""
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjMmdmenVXeGkya3JadjJTcWtOejNmNlVwTWJKZSIsInFsaWJfaWQiOiJpbGliM1Jpd2lQN1VKSmlIeEZMYmtMNDZCb1ZmS1dyQiIsImFkZHIiOiIweEJCMTAzOTAxNTMwNmU0MjM5Yzg0NEY0N0NlMDY1NWYyN0I2NzQ0YWUiLCJxaWQiOiJpcV9fQUhpaDZrZjdNaW5iYkRQRW5UNFhGeFNNdVFUIiwiZ3JhbnQiOiJyZWFkIiwidHhfcmVxdWlyZWQiOmZhbHNlLCJpYXQiOjE2MDQwNzMyNjEsImV4cCI6MTYwNDA3Njg2MSwiaXBfZ2VvIjoiVVMiLCJhdXRoX3NpZyI6IkVTMjU2S19MZDZiVlR6dVBkZ1lMQnVZOXFQM1gyUEpuS2NpQXdYQzhVMWZuQW9MdnFEYTZWMk12dkxFNG1UbVhjekxUdHhMdmJ2OHdiVkVKWHhZTHRVTUxiM1ZIYWdpTCIsImFmZ2hfcGsiOiIifQ==.RVMyNTZLX0pwSkNjZVVXZmlZRGpHd2Vxazkyd29hZXdTZW8yNlVXNWJBWE5xc3VjODQ1N3hTZWs3V0J0S0s3ZHNuMUVGM2RWcUhMNGROM0o5aVBiUnIxaDNlb0VRN0ha`,
			wantType: eat.Types.Client(),
			validate: func(tok *eat.Token) {
				require.NotEmpty(t, tok.GetQSpaceID())
				require.NotEmpty(t, tok.GetQLibID())
			},
			trustedAddr: "0xd9DC97B58C5f2584062Cf69775d160ed9A3BFbC4",
		},
		{
			/*
				{
				  "qspace_id": "ispc2RUoRe9eR2v33HARQUVSp1rYXzw1",
				  "addr": "0xD962AFf088CA845de83CE0db0C91b9a0b93d294f",
				  "qphash": "hqp_6Z7u3fAsD54kYBuWzpD49AiphEYRHf5QppfJMZhJ4Uif8Ze"
				}
			*/
			token:    `eyJxc3BhY2VfaWQiOiJpc3BjMlJVb1JlOWVSMnYzM0hBUlFVVlNwMXJZWHp3MSIsImFkZHIiOiIweEQ5NjJBRmYwODhDQTg0NWRlODNDRTBkYjBDOTFiOWEwYjkzZDI5NGYiLCJxcGhhc2giOiJocXBfNlo3dTNmQXNENTRrWUJ1V3pwRDQ5QWlwaEVZUkhmNVFwcGZKTVpoSjRVaWY4WmUifQ==.RVMyNTZLX01YTmJyYTdZV2JTOW5zZHZFOTV5Y25mN0wyWmpaUlBaOUhDUHhpdXlZRDVyTlk2OWdESGdBNzRyb0dEUE5zbjQ0R3NmSkQ0NFU5UTNoRUUyUVVjdG1teEM3`,
			wantType: eat.Types.Node(),
			validate: func(tok *eat.Token) {
				require.NotEmpty(t, tok.GetQSpaceID())
				require.NotEmpty(t, tok.QPHash)
			},
			trustedAddr: "0xD962AFf088CA845de83CE0db0C91b9a0b93d294f",
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			jsn, _ := base64.StdEncoding.DecodeString(test.token)
			fmt.Println("original bytes:", string(jsn))

			tok, err := eat.FromString(test.token)
			require.NoError(t, err)
			require.NotNil(t, tok)
			require.Equal(t, test.wantType, tok.Type, tok.Type)

			fmt.Println(eat.Describe(test.token))

			leg := &eat.TokenDataLegacy{}
			leg.CopyFromTokenData(tok)
			// fmt.Println(leg)

			err = tok.VerifySignature()
			require.NoError(t, err)

			if tok.Type == eat.Types.Client() {
				if test.trustedAddr != "" {
					err = tok.Embedded.VerifySignatureFrom(common.HexToAddress(test.trustedAddr))
					require.NoError(t, err)
				}

				require.Equal(t, eat.Types.StateChannel(), tok.Embedded.Type)
				bearer, err := tok.OriginalBearer()
				require.NoError(t, err)
				require.Equal(t, test.token, bearer)

				s, err := tok.Encode()
				require.NoError(t, err)
				requireEquivalentLegacyToken(t, test.token, s)
			}

			err = tok.VerifySignature()
			require.NoError(t, err)

			if test.validate != nil {
				test.validate(tok)
			}
		})
	}
}

func TestSignatures(t *testing.T) {
	tok := baseUnsignedToken.With(eat.Formats.Cbor())
	tok.Type = eat.Types.Plain()
	err := tok.SignWith(clientSK)
	require.NoError(t, err)

	fmt.Println(tok.SigType)

	assertEncodeDecode(t, tok)
}

func assertEncodeDecode(t *testing.T, tok *eat.Token) *eat.Token {
	encoded, err := tok.Encode()
	require.NoError(t, err)

	fmt.Println(encoded)

	tokDecoded, err := eat.Parse(encoded)
	require.NoError(t, err)

	err = tokDecoded.VerifySignature()
	require.NoError(t, err)

	return tokDecoded
}

func TestClientTokens(t *testing.T) {
	st := &eat.Token{
		Type:    eat.Types.StateChannel(),
		SigType: eat.SigTypes.Unsigned(),
		Format:  eat.Formats.CborCompressed(),
		TokenData: eat.TokenData{
			SID:      sid,
			LID:      lid,
			QID:      qid,
			EthAddr:  clientAddr,
			Grant:    "read",
			Subject:  clientAddr.Hex(),
			IssuedAt: utc.Now().Truncate(time.Second),                // legacy format has second precision
			Expires:  utc.Now().Truncate(time.Second).Add(time.Hour), // legacy format has second precision
			Ctx: map[string]interface{}{
				"key1": "val1",
				"key2": "val2",
			},
		},
	}

	fmt.Println(st.Explain())
	sign(t, st)
	fmt.Println(st.Explain())

	ct, err := eat.NewClientToken(st)
	require.NoError(t, err)

	ct.AFGHPublicKey = "afgh pub key"

	fmt.Println(ct.Explain())
	sign(t, ct, clientSK)
	fmt.Println(ct.Explain())

	decoded := assertEncodeDecode(t, ct)

	require.NotNil(t, decoded.Embedded.TokenBytes)
	require.NoError(t, decoded.Embedded.VerifySignatureFrom(serverAddr))
	require.Equal(t, st, decoded.Embedded)
}

func TestLegacySignedToken(t *testing.T) {
	st := &eat.Token{
		Type:    eat.Types.StateChannel(),
		SigType: eat.SigTypes.Unsigned(),
		Format:  eat.Formats.CborCompressed(),
		TokenData: eat.TokenData{
			SID:      sid,
			LID:      lid,
			QID:      qid,
			EthAddr:  clientAddr,
			Grant:    "read",
			Subject:  clientAddr.Hex(),
			IssuedAt: utc.Now().Truncate(time.Second),                // legacy format has second precision
			Expires:  utc.Now().Truncate(time.Second).Add(time.Hour), // legacy format has second precision
			Ctx: map[string]interface{}{
				"key1": "val1",
				"key2": "val2",
			},
		},
	}

	sign(t, st)

	stEnc, err := st.Encode()
	require.NoError(t, err)

	legacySigned, err := eat.LegacySign(stEnc, clientSK)
	require.NoError(t, err)

	tokDecoded, err := eat.Parse(legacySigned)
	require.NoError(t, err)

	fmt.Println("token", legacySigned)
	fmt.Println("server-addr", serverAddr.String(), "client-addr", clientAddr.String())

	require.NoError(t, tokDecoded.Embedded.VerifySignatureFrom(serverAddr))
	require.NoError(t, tokDecoded.VerifySignature())

	require.NotNil(t, tokDecoded.Embedded.TokenBytes)
	require.Equal(t, st, tokDecoded.Embedded)

	fmt.Println(eat.Describe(legacySigned))
}

func TestLegacySignedTokenParseError(t *testing.T) {
	missingPadding :=
		"ascsj_5hZL3eWQopZJVry4A1jR3yxZ69QxeySh1ithBGFuF9JACBZCQRriFHRLT7WBwp" +
			"KcKVjFJHzpXk87jTmfj2Vq7Hch9uo7CrSaowvgVay4s9LnwTEaM7DCM2E9oAT7Av" +
			"GvJDju1x3hYrhZz6s4oGgsQX9wFSWWCNCxgNebUjAgtsbXgGp2Zy9oVYco1aNpVj" +
			"FronE5QtWgk48BFw8QAqwCUAW7XFRi5BjPexomcMD5TBaqZYtZ97CXGKNeoFtjy2" +
			"CUCaRzuf3XSuGSEA73NDWGRqhZ85sLtd8fthpKFMhfWzbgbLekdA8zPVTVcRCWVR" +
			"Etv3uXUrWZeHLQiTT1t2LA3LJwbwYjXP8Sp8VkBmSHkaVKnH6AU9TVjTtkjejos5" +
			"r1UV4fwbtKS6Ficy6E6yKjopp6r3gonV3vXP7jgD3cZEfnNtrU19Hn.RVMyNTZLX" +
			"zhWdlNRUEJ3WlRGSEpmVnV1OUZCdnVZOVh4UjdxN0ZGQ01zVjIzakJkc0h2MVEzY" +
			"zNoZnhIVGllbVA5UlJjOVRjQnV6bUpud0g3NFhOc3RIbTZwVXZCRUc"
	correct := missingPadding + "="
	for i, s := range []string{correct, missingPadding} {
		tok, err := eat.Parse(s)
		if i == 0 {
			require.NoError(t, err, i)
			err = tok.VerifySignature()
			require.NoError(t, err, i)
		} else {
			require.Error(t, err)
		}

	}
}

func TestOTPBackwardsCompat(t *testing.T) {
	//st := &eat.Token{
	//	Type:    eat.Types.StateChannel(),
	//	SigType: eat.SigTypes.Unsigned(),
	//	Format:  eat.Formats.CborCompressed(),
	//	TokenData: eat.TokenData{
	//		SID:      sid,
	//		LID:      lid,
	//		QID:      qid,
	//		EthAddr:  clientAddr,
	//		Grant:    "read",
	//		IssuedAt: utc.Now().Truncate(time.Second),                // legacy format has second precision
	//		Expires:  utc.Now().Truncate(time.Second).Add(time.Hour), // legacy format has second precision
	//		Ctx: map[string]interface{}{
	//			"key1": "val1",
	//			"key2": "val2",
	//		},
	//	},
	//}
	//
	//sign(t, st)
	//
	//stEnc, err := st.Encode()
	//require.NoError(t, err)

	stEnc, err := eat.NewStateChannel(sid, lid, qid, clientAddr.Hex()).
		WithGrant(eat.Grants.Read).
		WithCtx(map[string]interface{}{
			"key1": "val1",
			"key2": "val2",
		}).
		WithIssuedAt(utc.Now().Truncate(time.Second)).
		WithExpires(utc.Now().Truncate(time.Second).Add(time.Hour)).
		Sign(serverSK).
		Encode()
	require.NoError(t, err)

	bts, err := json.Marshal(map[string]interface{}{
		"qid": qid,
		"tok": stEnc,
	})
	require.NoError(t, err)

	otpTok := base64.StdEncoding.EncodeToString(bts)

	tokDecoded, err := eat.Parse(otpTok)
	require.NoError(t, err)

	fmt.Println("token", otpTok)
	fmt.Println("server-addr", serverAddr.String(), "client-addr", clientAddr.String())

	require.NoError(t, tokDecoded.VerifySignatureFrom(serverAddr))
	// require.Equal(t, st, tokDecoded)

	s := "ascsj_3rTJP4Prj6RuqofLqaaRPSJyWUE5c8Njdxh4wYP7sma38iqY81fCqEEgswtXQQTFxBydyyEEkctY2BmKTrx8jGgC2GWYChgYm3n3PWFfr4PZoFY7TQ2S2Bv4DHeX5T76LbcEZ2LuY79xZ8FXrCLkRCpc5udTbkJVoQgzzM4X2Y6sGYLi3KAfnevmBpZKEnqkGYSKM7TiFWGbSqyRccGjdz5qQLuzToGQXQfZ2DzZGGz1AZmR5z2bWGAEqPxdxyWJPL2MUiprm7Rja2JxnypmxPckrv3WiRxT2tMdyWPWxbCV6dwvd9fcJxbfebtJKTvBxQ8GaiWJv72HeaQHCrY61QYZEZpAQTsmHHnjaEyvKsP4nJLqZzNV7ZkWQ6XrNcLkxNVVgVrrxc6qcw85hVQYvHaopCSbyMtBVVxkSZmysqLeXrNFS2rSHiaDGx3w78uceYNWYqyX7eq9bhagQ4BFatKq1rb1yBqNgE92bjuDmxCVfdh6JfqZRWR5q5Nk"
	/* Validation passes because Subject is empty but Ctx is not
	{
	  "tok": "ascsj_3rTJP4Prj6RuqofLqaaRPSJyWUE5c8Njdxh4wYP7sma38iqY81fCqEEgswtXQQTFxBydyyEEkctY2BmKTrx8jGgC2GWYChgYm3n3PWFfr4PZoFY7TQ2S2Bv4DHeX5T76LbcEZ2LuY79xZ8FXrCLkRCpc5udTbkJVoQgzzM4X2Y6sGYLi3KAfnevmBpZKEnqkGYSKM7TiFWGbSqyRccGjdz5qQLuzToGQXQfZ2DzZGGz1AZmR5z2bWGAEqPxdxyWJPL2MUiprm7Rja2JxnypmxPckrv3WiRxT2tMdyWPWxbCV6dwvd9fcJxbfebtJKTvBxQ8GaiWJv72HeaQHCrY61QYZEZpAQTsmHHnjaEyvKsP4nJLqZzNV7ZkWQ6XrNcLkxNVVgVrrxc6qcw85hVQYvHaopCSbyMtBVVxkSZmysqLeXrNFS2rSHiaDGx3w78uceYNWYqyX7eq9bhagQ4BFatKq1rb1yBqNgE92bjuDmxCVfdh6JfqZRWR5q5Nk",
	  "qid": "iq__27jxWVjbos5zLtwmeZ4HcZbpQomL"
	}

	{
	  "EthTxHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	  "EthAddr": "0xd962aff088ca845de83ce0db0c91b9a0b93d294f",
	  "AFGHPublicKey": "",
	  "QPHash": null,
	  "SID": "ispc2RUoRe9eR2v33HARQUVSp1rYXzw1",
	  "LID": "ilib31RD8PXrsdvSppy2p78LU3C9JdME",
	  "QID": "iq__27jxWVjbos5zLtwmeZ4HcZbpQomL",
	  "Subject": "",
	  "Grant": "read",
	  "IssuedAt": "2020-12-01T08:35:34.708Z",
	  "Expires": "2020-12-02T08:35:34.708Z",
	  "Ctx": {
	    "elv:delegation-id": "iq__27jxWVjbos5zLtwmeZ4HcZbpQomL",
	    "elv:otpId": "QOTP5zvPup5LiwF"
	  }
	}
	*/
	res, err := eat.Parse(s)
	require.NoError(t, err)
	require.Equal(t, eat.Types.StateChannel(), res.Type)
	//fmt.Println(res.Explain())

}

func sign(t *testing.T, tok *eat.Token, key ...*ecdsa.PrivateKey) *eat.Token {
	skey := serverSK
	if len(key) > 0 {
		skey = key[0]
	}
	err := tok.SignWith(skey)
	require.NoError(t, err)
	return tok
}

func requireEquivalentLegacyToken(t *testing.T, expected string, actual string) {
	var gens []interface{}
	var sigs []string
	for _, ts := range []string{expected, actual} {
		parts := strings.SplitN(ts, ".", 2)
		require.LessOrEqual(t, 1, len(parts))

		var gen interface{}
		dec, err := base64.StdEncoding.DecodeString(parts[0])
		require.NoError(t, err)

		// convert json to lower case because legacy tokens have a weird
		// mixed-case hex encoding of addresses and hashes...
		err = json.Unmarshal(bytes.ToLower(dec), &gen)
		require.NoError(t, err)
		gens = append(gens, gen)

		if len(parts) > 1 {
			sigs = append(sigs, parts[1])
		}
	}

	if len(sigs) == 2 {
		require.Equal(t, sigs[0], sigs[1])
	}

	require.Equal(t, 2, len(gens))
	//goland:noinspection GoNilness
	require.Equal(t, gens[0], gens[1])
}

func TestEditorSignedSubject(t *testing.T) {
	token := eat.New(eat.Types.EditorSigned(), eat.Formats.Json(), eat.SigTypes.Unsigned())
	token.SID = sid
	token.LID = lid
	token.QID = qid
	token.Grant = eat.Grants.Read
	token.IssuedAt = utc.Now()
	token.Expires = token.IssuedAt.Add(time.Hour)

	// subject is not necessarily the signer
	token.Subject = "me"

	err := token.SignWith(clientSK)
	require.NoError(t, err)
	_, err = token.Encode()
	require.NoError(t, err)

	signAddr := crypto.PubkeyToAddress(clientSK.PublicKey)
	token.Subject = signAddr.String()
	err = token.SignWith(clientSK)
	require.NoError(t, err)
	_, err = token.Encode()
	require.NoError(t, err)

	token.Subject = ethutil.AddressToID(signAddr, id.User).String()
	err = token.SignWith(clientSK)
	require.NoError(t, err)
	_, err = token.Encode()
	require.NoError(t, err)

	ess := eat.NewEditorSigned(sid, lid, qid).
		WithSubject("me").
		WithExpires(token.Expires)
	_, err = ess.Sign(clientSK).Encode()
	require.NoError(t, err)
	require.Equal(t, "me", ess.Token().Subject)

	ess = eat.NewEditorSigned(sid, lid, qid).
		WithExpires(token.Expires)
	_, err = ess.Sign(clientSK).Encode()
	require.NoError(t, err)
	require.Equal(t, token.Subject, ess.Token().Subject)
}
