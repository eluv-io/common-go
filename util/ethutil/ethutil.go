package ethutil

import (
	"bytes"
	"crypto/ecdsa"
	"github.com/qluvio/content-fabric/format/keys"
	"github.com/qluvio/content-fabric/format/types"
	"encoding/hex"
	"io/ioutil"
	"os"
	"strings"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/id"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func HashFromHex(hash string) (*common.Hash, error) {

	h := common.Hash{}
	hash = strings.ToLower(hash)

	if strings.HasPrefix(hash, "0x") {
		hash = hash[2:]
	}

	if length := len(hash); length != 2*common.HashLength {
		return nil, errors.E("HashFromHex", errors.K.Invalid,
			"reason", "invalid hash hex length", length, 2*common.HashLength)
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
	if strings.HasPrefix(addr, "0x") {
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

func DecryptKeyFile(keyfile string, password string) (*keystore.Key, error) {
	keyBytes, err := ioutil.ReadFile(keyfile)
	if err != nil {
		return nil, errors.E("decrypt key file", err, "file", keyfile)
	}

	privKey, err := keystore.DecryptKey(keyBytes, password)
	if err != nil {
		return nil, err
	}
	return privKey, nil
}

func DecryptKeyFileSK(keyfile string, password string) ([]byte, error) {
	privKey, err := DecryptKeyFile(keyfile, password)
	if err != nil {
		return nil, err
	}
	return privKey.PrivateKey.D.Bytes(), nil
}

/*
 * Decrypts an Ethereum encrypted keystore file
 * Return: private key
 */
func DecryptKeyFileJSON(keyfile string, password string) (*keystore.Key, error) {
	keyBytes, err := ioutil.ReadFile(keyfile)

	if err != nil {
		return nil, err
	}

	return keystore.DecryptKey(keyBytes, password)
}

func RecryptKeyFile(keyfile string, password string, scryptN int) error {

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

// ToNodePublicKey returns the public key of the given key in keys.KID format
// as well as as an address in id format. All returns use node prefixes.
func ToNodePublicKey(privateKey *ecdsa.PrivateKey) (keys.KID, types.QNodeID) {
	pubKey := keys.NewKID(keys.FabricNodePublicKey,
		crypto.CompressPubkey(&privateKey.PublicKey))
	nid := AddressToID(
		crypto.PubkeyToAddress(privateKey.PublicKey),
		id.QNode)
	return pubKey, nid
}
