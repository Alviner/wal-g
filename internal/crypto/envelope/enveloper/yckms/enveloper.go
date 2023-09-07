package yckms

import (
	"context"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/kms/v1"
	ycsdk "github.com/yandex-cloud/go-sdk"
	"github.com/yandex-cloud/go-sdk/iamkey"

	"github.com/wal-g/wal-g/internal/crypto/envelope"
)

const (
	magic              = "envelope-yc-kms"
	schemeVersion byte = 1
)

type Enveloper struct {
	keyID string
	sdk   *ycsdk.SDK
}

func (enveloper *Enveloper) Name() string {
	return "yckms"
}

func (enveloper *Enveloper) ReadEncryptedKey(r io.Reader) ([]byte, error) {
	return readEncryptedKey(r)
}

func (enveloper *Enveloper) DecryptKey(encryptedKey []byte) ([]byte, error) {
	ctx := context.Background()
	rsp, err := enveloper.sdk.KMSCrypto().SymmetricCrypto().Decrypt(ctx, &kms.SymmetricDecryptRequest{
		KeyId:      enveloper.keyID,
		Ciphertext: encryptedKey,
		AadContext: nil,
	})

	if err != nil {
		return nil, err
	}

	return rsp.Plaintext, nil
}

func (enveloper *Enveloper) SerializeEncryptedKey(encryptedKey []byte) []byte {
	return serializeEncryptedKey(encryptedKey)
}

func serializeEncryptedKey(encryptedKey []byte) []byte {
	/*
		magic value "envelope-yc-kms"
		scheme version (current version is 1)
		uint32 - encrypted key len
		encrypted key ...
	*/

	encryptedKeyLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(encryptedKeyLen, uint32(len(encryptedKey)))
	result := append([]byte(magic), schemeVersion)
	result = append(result, encryptedKeyLen...)

	return append(result, encryptedKey...)
}

func readEncryptedKey(r io.Reader) ([]byte, error) {
	magicSchemeBytes := make([]byte, len(magic)+1)
	_, err := r.Read(magicSchemeBytes)
	if err != nil {
		return nil, err
	}

	if string(magicSchemeBytes[0:len(magic)]) != magic {
		return nil, errors.New("envelope yc kms: invalid encrypted header format")
	}

	if schemeVersion != magicSchemeBytes[len(magic)] {
		return nil, errors.New("envelope yc kms: scheme version is not supported")
	}

	encryptedKeyLenBytes := make([]byte, 4)
	_, err = r.Read(encryptedKeyLenBytes)
	if err != nil {
		return nil, err
	}

	encryptedKeyLen := binary.LittleEndian.Uint32(encryptedKeyLenBytes)
	encryptedKey := make([]byte, encryptedKeyLen)
	_, err = r.Read(encryptedKey)
	if err != nil {
		return nil, err
	}

	return encryptedKey, nil
}

func EnveloperFromKeyIDAndCredential(keyID string, saFilePath string) (envelope.Enveloper, error) {
	var authorizedKey, authErr = iamkey.ReadFromJSONFile(saFilePath)
	if authErr != nil {
		return nil, errors.Wrap(authErr, "Can't initialize yc sdk")
	}
	var credentials, credErr = ycsdk.ServiceAccountKey(authorizedKey)
	if credErr != nil {
		return nil, errors.Wrap(credErr, "Can't initialize yc sdk")
	}

	var sdk, sdkErr = ycsdk.Build(context.Background(), ycsdk.Config{
		Credentials: credentials,
	})
	if sdkErr != nil {
		return nil, errors.Wrap(sdkErr, "Can't initialize yc sdk")
	}
	return &Enveloper{
		keyID: keyID,
		sdk:   sdk,
	}, nil
}
