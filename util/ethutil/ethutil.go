package ethutil

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"io/ioutil"
	"os"
	"strings"

	"github.com/eluv-io/errors-go"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/keys"
	"github.com/qluvio/content-fabric/format/types"
)

func HashFromHex(hash string) (*common.Hash, error) {

	h := common.Hash{}
	hash = strings.ToLower(hash)

	if strings.HasPrefix(hash, "0x") || strings.HasPrefix(hash, "0X") {
		hash = hash[2:]
	}

	if length := len(hash); length != 2*common.HashLength {
		return nil, errors.E("HashFromHex", errors.K.Invalid,
			"reason", "invalid hash hex length",
			"length", length,
			"expected_length", 2*common.HashLength)
	}

	bin, err := hex.DecodeString(hash)
	if err != nil {
		return nil, errors.E("HashFromHex", errors.K.Invalid, err,
			"reason", "invalid hash hex representation")
	}
	copy(h[:], bin)
	return &h, nil
}

// AddrToID converts the given ethereum address to an ID
// The address is expected to be hex encoded
func AddrToID(addr string, code id.Code) (id.ID, error) {
	if strings.HasPrefix(addr, "0x") || strings.HasPrefix(addr, "0X") {
		addr = addr[2:]
	}
	bufAddr, err := hex.DecodeString(addr)
	if err != nil || len(bufAddr) == 0 {
		return nil, errors.E("AddrToID", errors.K.Invalid, err,
			"reason", "invalid address",
			"addr", addr)
	}
	return id.ID(append([]byte{byte(code)}, bufAddr...)), nil
}

func AddressToID(addr common.Address, code id.Code) id.ID {
	return id.NewID(code, addr.Bytes())
}

func IDToAddress(id id.ID) common.Address {
	return common.BytesToAddress(id.Bytes())
}

func IDStringToAddress(idString string) (common.Address, error) {
	id, err := id.FromString(idString)
	if err != nil {
		return common.Address{}, err
	}
	return IDToAddress(id), nil
}

// HexToAddress converts the given hex string into an ethereum address.
// Similar to common.HexToAddress but returning an error and not setting bytes
// in the address in case of error.
func HexToAddress(s string) (common.Address, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	var a common.Address
	if err != nil {
		return a, errors.E("HexToAddress", errors.K.Invalid, err,
			"value", s)
	}
	a.SetBytes(b)
	return a, nil
}

// AddressEqualsID compares an ethereum address and an eluvio ID.
// The ethereum address is first converted to an eluvio ID.
func AddressEqualsID(ethAddr common.Address, elvID id.ID) bool {
	if elvID == nil {
		return false
	}
	return bytes.Equal(ethAddr.Bytes(), elvID.Bytes())
}

// AddrEqualsID compares an ethereum address and an eluvio ID.
// The ethereum address is first converted to an eluvio ID.
func AddrEqualsID(ethAddr string, code id.Code, elvID string) (bool, error) {
	ethId, err := AddrToID(ethAddr, code)
	if err != nil {
		return false, err
	}
	elv, err := id.FromString(elvID)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ethId, elv), nil
}

func GetPublicKeyBytes(key *ecdsa.PublicKey) []byte {
	return crypto.FromECDSAPub(key)
}

func GetPrivateKeyBytes(key *ecdsa.PrivateKey) []byte {
	return crypto.FromECDSA(key)
}

func PrivateKeyFromString(keyStr string) (*ecdsa.PrivateKey, error) {
	has0xPrefix := func(input string) bool {
		return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
	}
	if has0xPrefix(keyStr) {
		keyStr = keyStr[2:]
	}

	ret, err := crypto.HexToECDSA(keyStr)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func DecryptKeyFile(keyfile string, password string) (*keystore.Key, error) {
	keyBytes, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return nil, errors.E("decrypt key file", err, "file", keyfile)
	}

	return keystore.DecryptKey(keyBytes, password)
}

func DecryptKeyFileSK(keyfile string, password string) ([]byte, error) {
	privKey, err := DecryptKeyFile(keyfile, password)
	if err != nil {
		return nil, err
	}
	return privKey.PrivateKey.D.Bytes(), nil
}

func RecryptKeyFile(keyfile string, password string, newpassword string, scryptN int) error {

	fileInfo, err := os.Stat(keyfile)
	if err != nil {
		return err
	}

	keyBytes, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return err
	}

	privKey, err := keystore.DecryptKey(keyBytes, password)
	if err != nil {
		return err
	}

	// if newpassword is provided, encrypt keyfile with it.
	if newpassword != "" {
		password = newpassword
	}

	newKeyBytes, err := keystore.EncryptKey(privKey, password, scryptN, 1)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(keyfile, newKeyBytes, fileInfo.Mode())
	if err != nil {
		return err
	}

	return nil
}

// NewKeyFile generates a new key and stores it into the key directory,
// encrypting it with the passphrase
func NewKeyFile(keystoreDir, password string) (common.Address, accounts.URL, error) {
	ks := keystore.NewKeyStore(keystoreDir, keystore.StandardScryptN, keystore.StandardScryptP)
	acct, err := ks.NewAccount(password)
	if err != nil {
		return common.Address{}, accounts.URL{}, err
	}
	return acct.Address, acct.URL, nil
}

func ToKeyFile(keystoreDir, privateKeyHex, password string) (common.Address, accounts.URL, error) {
	e := errors.Template("ToKeyFile", "dir", keystoreDir)
	pk, err := PrivateKeyFromString(privateKeyHex)
	if err != nil {
		return common.Address{}, accounts.URL{}, e(err)
	}
	ks := keystore.NewKeyStore(keystoreDir, keystore.StandardScryptN, keystore.StandardScryptP)
	account, err := ks.ImportECDSA(pk, password)
	if err != nil {
		return common.Address{}, accounts.URL{}, e(err)
	}
	return account.Address, account.URL, nil
}

// ToNodePublicKey returns the public key of the given key in keys.KID format
// as well as as an address in id format. All returns use node prefixes.
func ToNodePublicKey(privateKey *ecdsa.PrivateKey) (keys.KID, types.QNodeID) {
	return ToPublicKeyAndID(privateKey, id.QNode)
}

func ToUserPublicKey(privateKey *ecdsa.PrivateKey) (keys.KID, types.UserID) {
	return ToPublicKeyAndID(privateKey, id.User)
}

func ToPublicKeyAndID(privateKey *ecdsa.PrivateKey, c id.Code) (keys.KID, id.ID) {
	var keyCode = keys.KUNKNOWN
	switch c {
	case id.QNode:
		keyCode = keys.FabricNodePublicKey
	case id.User:
		keyCode = keys.UserPublicKey
	}
	return keys.NewKID(keyCode, crypto.CompressPubkey(&privateKey.PublicKey)),
		AddressToID(crypto.PubkeyToAddress(privateKey.PublicKey), c)
}
