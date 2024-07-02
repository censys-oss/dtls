// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package prf

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/censys-oss/dtls/v2/pkg/crypto/elliptic"
)

func TestPreMasterSecret(t *testing.T) {
	privateKey := []byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f}
	publicKey := []byte{0x9f, 0xd7, 0xad, 0x6d, 0xcf, 0xf4, 0x29, 0x8d, 0xd3, 0xf9, 0x6d, 0x5b, 0x1b, 0x2a, 0xf9, 0x10, 0xa0, 0x53, 0x5b, 0x14, 0x88, 0xd7, 0xf8, 0xfa, 0xbb, 0x34, 0x9a, 0x98, 0x28, 0x80, 0xb6, 0x15}
	expectedPreMasterSecret := []byte{0xdf, 0x4a, 0x29, 0x1b, 0xaa, 0x1e, 0xb7, 0xcf, 0xa6, 0x93, 0x4b, 0x29, 0xb4, 0x74, 0xba, 0xad, 0x26, 0x97, 0xe2, 0x9f, 0x1f, 0x92, 0x0d, 0xcc, 0x77, 0xc8, 0xa0, 0xa0, 0x88, 0x44, 0x76, 0x24}

	preMasterSecret, err := PreMasterSecret(publicKey, privateKey, elliptic.X25519)
	if err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(expectedPreMasterSecret, preMasterSecret) {
		t.Fatalf("PremasterSecret exp: % 02x actual: % 02x", expectedPreMasterSecret, preMasterSecret)
	}
}

func TestMasterSecret(t *testing.T) {
	preMasterSecret := []byte{0xdf, 0x4a, 0x29, 0x1b, 0xaa, 0x1e, 0xb7, 0xcf, 0xa6, 0x93, 0x4b, 0x29, 0xb4, 0x74, 0xba, 0xad, 0x26, 0x97, 0xe2, 0x9f, 0x1f, 0x92, 0x0d, 0xcc, 0x77, 0xc8, 0xa0, 0xa0, 0x88, 0x44, 0x76, 0x24}
	clientRandom := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}
	serverRandom := []byte{0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f}
	expectedMasterSecret := []byte{0x91, 0x6a, 0xbf, 0x9d, 0xa5, 0x59, 0x73, 0xe1, 0x36, 0x14, 0xae, 0x0a, 0x3f, 0x5d, 0x3f, 0x37, 0xb0, 0x23, 0xba, 0x12, 0x9a, 0xee, 0x02, 0xcc, 0x91, 0x34, 0x33, 0x81, 0x27, 0xcd, 0x70, 0x49, 0x78, 0x1c, 0x8e, 0x19, 0xfc, 0x1e, 0xb2, 0xa7, 0x38, 0x7a, 0xc0, 0x6a, 0xe2, 0x37, 0x34, 0x4c}

	masterSecret, err := MasterSecret(preMasterSecret, clientRandom, serverRandom, sha256.New)
	if err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(expectedMasterSecret, masterSecret) {
		t.Fatalf("masterSecret exp: % 02x actual: % 02x", expectedMasterSecret, masterSecret)
	}
}

func TestEncryptionKeys(t *testing.T) {
	clientRandom := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}
	serverRandom := []byte{0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f}
	masterSecret := []byte{0x91, 0x6a, 0xbf, 0x9d, 0xa5, 0x59, 0x73, 0xe1, 0x36, 0x14, 0xae, 0x0a, 0x3f, 0x5d, 0x3f, 0x37, 0xb0, 0x23, 0xba, 0x12, 0x9a, 0xee, 0x02, 0xcc, 0x91, 0x34, 0x33, 0x81, 0x27, 0xcd, 0x70, 0x49, 0x78, 0x1c, 0x8e, 0x19, 0xfc, 0x1e, 0xb2, 0xa7, 0x38, 0x7a, 0xc0, 0x6a, 0xe2, 0x37, 0x34, 0x4c}

	expectedEncryptionKeys := &EncryptionKeys{
		MasterSecret:   masterSecret,
		ClientMACKey:   []byte{},
		ServerMACKey:   []byte{},
		ClientWriteKey: []byte{0x1b, 0x7d, 0x11, 0x7c, 0x7d, 0x5f, 0x69, 0x0b, 0xc2, 0x63, 0xca, 0xe8, 0xef, 0x60, 0xaf, 0x0f},
		ServerWriteKey: []byte{0x18, 0x78, 0xac, 0xc2, 0x2a, 0xd8, 0xbd, 0xd8, 0xc6, 0x01, 0xa6, 0x17, 0x12, 0x6f, 0x63, 0x54},
		ClientWriteIV:  []byte{0x0e, 0xb2, 0x09, 0x06},
		ServerWriteIV:  []byte{0xf7, 0x81, 0xfa, 0xd2},
	}
	keys, err := GenerateEncryptionKeys(masterSecret, clientRandom, serverRandom, 0, 16, 4, sha256.New)

	if err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(expectedEncryptionKeys, keys) {
		t.Fatalf("masterSecret exp: %q actual: %q", expectedEncryptionKeys, keys)
	}
}

func TestVerifyData(t *testing.T) {
	clientHello := []byte{0x01, 0x00, 0x00, 0xa1, 0x03, 0x03, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x00, 0x00, 0x20, 0xcc, 0xa8, 0xcc, 0xa9, 0xc0, 0x2f, 0xc0, 0x30, 0xc0, 0x2b, 0xc0, 0x2c, 0xc0, 0x13, 0xc0, 0x09, 0xc0, 0x14, 0xc0, 0x0a, 0x00, 0x9c, 0x00, 0x9d, 0x00, 0x2f, 0x00, 0x35, 0xc0, 0x12, 0x00, 0x0a, 0x01, 0x00, 0x00, 0x58, 0x00, 0x00, 0x00, 0x18, 0x00, 0x16, 0x00, 0x00, 0x13, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x2e, 0x75, 0x6c, 0x66, 0x68, 0x65, 0x69, 0x6d, 0x2e, 0x6e, 0x65, 0x74, 0x00, 0x05, 0x00, 0x05, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00, 0x0a, 0x00, 0x08, 0x00, 0x1d, 0x00, 0x17, 0x00, 0x18, 0x00, 0x19, 0x00, 0x0b, 0x00, 0x02, 0x01, 0x00, 0x00, 0x0d, 0x00, 0x12, 0x00, 0x10, 0x04, 0x01, 0x04, 0x03, 0x05, 0x01, 0x05, 0x03, 0x06, 0x01, 0x06, 0x03, 0x02, 0x01, 0x02, 0x03, 0xff, 0x01, 0x00, 0x01, 0x00, 0x00, 0x12, 0x00, 0x00}
	serverHello := []byte{0x02, 0x00, 0x00, 0x2d, 0x03, 0x03, 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f, 0x00, 0xc0, 0x13, 0x00, 0x00, 0x05, 0xff, 0x01, 0x00, 0x01, 0x00}
	serverCertificate := []byte{0x0b, 0x00, 0x03, 0x2b, 0x00, 0x03, 0x28, 0x00, 0x03, 0x25, 0x30, 0x82, 0x03, 0x21, 0x30, 0x82, 0x02, 0x09, 0xa0, 0x03, 0x02, 0x01, 0x02, 0x02, 0x08, 0x15, 0x5a, 0x92, 0xad, 0xc2, 0x04, 0x8f, 0x90, 0x30, 0x0d, 0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0b, 0x05, 0x00, 0x30, 0x22, 0x31, 0x0b, 0x30, 0x09, 0x06, 0x03, 0x55, 0x04, 0x06, 0x13, 0x02, 0x55, 0x53, 0x31, 0x13, 0x30, 0x11, 0x06, 0x03, 0x55, 0x04, 0x0a, 0x13, 0x0a, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x41, 0x30, 0x1e, 0x17, 0x0d, 0x31, 0x38, 0x31, 0x30, 0x30, 0x35, 0x30, 0x31, 0x33, 0x38, 0x31, 0x37, 0x5a, 0x17, 0x0d, 0x31, 0x39, 0x31, 0x30, 0x30, 0x35, 0x30, 0x31, 0x33, 0x38, 0x31, 0x37, 0x5a, 0x30, 0x2b, 0x31, 0x0b, 0x30, 0x09, 0x06, 0x03, 0x55, 0x04, 0x06, 0x13, 0x02, 0x55, 0x53, 0x31, 0x1c, 0x30, 0x1a, 0x06, 0x03, 0x55, 0x04, 0x03, 0x13, 0x13, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x2e, 0x75, 0x6c, 0x66, 0x68, 0x65, 0x69, 0x6d, 0x2e, 0x6e, 0x65, 0x74, 0x30, 0x82, 0x01, 0x22, 0x30, 0x0d, 0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01, 0x05, 0x00, 0x03, 0x82, 0x01, 0x0f, 0x00, 0x30, 0x82, 0x01, 0x0a, 0x02, 0x82, 0x01, 0x01, 0x00, 0xc4, 0x80, 0x36, 0x06, 0xba, 0xe7, 0x47, 0x6b, 0x08, 0x94, 0x04, 0xec, 0xa7, 0xb6, 0x91, 0x04, 0x3f, 0xf7, 0x92, 0xbc, 0x19, 0xee, 0xfb, 0x7d, 0x74, 0xd7, 0xa8, 0x0d, 0x00, 0x1e, 0x7b, 0x4b, 0x3a, 0x4a, 0xe6, 0x0f, 0xe8, 0xc0, 0x71, 0xfc, 0x73, 0xe7, 0x02, 0x4c, 0x0d, 0xbc, 0xf4, 0xbd, 0xd1, 0x1d, 0x39, 0x6b, 0xba, 0x70, 0x46, 0x4a, 0x13, 0xe9, 0x4a, 0xf8, 0x3d, 0xf3, 0xe1, 0x09, 0x59, 0x54, 0x7b, 0xc9, 0x55, 0xfb, 0x41, 0x2d, 0xa3, 0x76, 0x52, 0x11, 0xe1, 0xf3, 0xdc, 0x77, 0x6c, 0xaa, 0x53, 0x37, 0x6e, 0xca, 0x3a, 0xec, 0xbe, 0xc3, 0xaa, 0xb7, 0x3b, 0x31, 0xd5, 0x6c, 0xb6, 0x52, 0x9c, 0x80, 0x98, 0xbc, 0xc9, 0xe0, 0x28, 0x18, 0xe2, 0x0b, 0xf7, 0xf8, 0xa0, 0x3a, 0xfd, 0x17, 0x04, 0x50, 0x9e, 0xce, 0x79, 0xbd, 0x9f, 0x39, 0xf1, 0xea, 0x69, 0xec, 0x47, 0x97, 0x2e, 0x83, 0x0f, 0xb5, 0xca, 0x95, 0xde, 0x95, 0xa1, 0xe6, 0x04, 0x22, 0xd5, 0xee, 0xbe, 0x52, 0x79, 0x54, 0xa1, 0xe7, 0xbf, 0x8a, 0x86, 0xf6, 0x46, 0x6d, 0x0d, 0x9f, 0x16, 0x95, 0x1a, 0x4c, 0xf7, 0xa0, 0x46, 0x92, 0x59, 0x5c, 0x13, 0x52, 0xf2, 0x54, 0x9e, 0x5a, 0xfb, 0x4e, 0xbf, 0xd7, 0x7a, 0x37, 0x95, 0x01, 0x44, 0xe4, 0xc0, 0x26, 0x87, 0x4c, 0x65, 0x3e, 0x40, 0x7d, 0x7d, 0x23, 0x07, 0x44, 0x01, 0xf4, 0x84, 0xff, 0xd0, 0x8f, 0x7a, 0x1f, 0xa0, 0x52, 0x10, 0xd1, 0xf4, 0xf0, 0xd5, 0xce, 0x79, 0x70, 0x29, 0x32, 0xe2, 0xca, 0xbe, 0x70, 0x1f, 0xdf, 0xad, 0x6b, 0x4b, 0xb7, 0x11, 0x01, 0xf4, 0x4b, 0xad, 0x66, 0x6a, 0x11, 0x13, 0x0f, 0xe2, 0xee, 0x82, 0x9e, 0x4d, 0x02, 0x9d, 0xc9, 0x1c, 0xdd, 0x67, 0x16, 0xdb, 0xb9, 0x06, 0x18, 0x86, 0xed, 0xc1, 0xba, 0x94, 0x21, 0x02, 0x03, 0x01, 0x00, 0x01, 0xa3, 0x52, 0x30, 0x50, 0x30, 0x0e, 0x06, 0x03, 0x55, 0x1d, 0x0f, 0x01, 0x01, 0xff, 0x04, 0x04, 0x03, 0x02, 0x05, 0xa0, 0x30, 0x1d, 0x06, 0x03, 0x55, 0x1d, 0x25, 0x04, 0x16, 0x30, 0x14, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x02, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x01, 0x30, 0x1f, 0x06, 0x03, 0x55, 0x1d, 0x23, 0x04, 0x18, 0x30, 0x16, 0x80, 0x14, 0x89, 0x4f, 0xde, 0x5b, 0xcc, 0x69, 0xe2, 0x52, 0xcf, 0x3e, 0xa3, 0x00, 0xdf, 0xb1, 0x97, 0xb8, 0x1d, 0xe1, 0xc1, 0x46, 0x30, 0x0d, 0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0b, 0x05, 0x00, 0x03, 0x82, 0x01, 0x01, 0x00, 0x59, 0x16, 0x45, 0xa6, 0x9a, 0x2e, 0x37, 0x79, 0xe4, 0xf6, 0xdd, 0x27, 0x1a, 0xba, 0x1c, 0x0b, 0xfd, 0x6c, 0xd7, 0x55, 0x99, 0xb5, 0xe7, 0xc3, 0x6e, 0x53, 0x3e, 0xff, 0x36, 0x59, 0x08, 0x43, 0x24, 0xc9, 0xe7, 0xa5, 0x04, 0x07, 0x9d, 0x39, 0xe0, 0xd4, 0x29, 0x87, 0xff, 0xe3, 0xeb, 0xdd, 0x09, 0xc1, 0xcf, 0x1d, 0x91, 0x44, 0x55, 0x87, 0x0b, 0x57, 0x1d, 0xd1, 0x9b, 0xdf, 0x1d, 0x24, 0xf8, 0xbb, 0x9a, 0x11, 0xfe, 0x80, 0xfd, 0x59, 0x2b, 0xa0, 0x39, 0x8c, 0xde, 0x11, 0xe2, 0x65, 0x1e, 0x61, 0x8c, 0xe5, 0x98, 0xfa, 0x96, 0xe5, 0x37, 0x2e, 0xef, 0x3d, 0x24, 0x8a, 0xfd, 0xe1, 0x74, 0x63, 0xeb, 0xbf, 0xab, 0xb8, 0xe4, 0xd1, 0xab, 0x50, 0x2a, 0x54, 0xec, 0x00, 0x64, 0xe9, 0x2f, 0x78, 0x19, 0x66, 0x0d, 0x3f, 0x27, 0xcf, 0x20, 0x9e, 0x66, 0x7f, 0xce, 0x5a, 0xe2, 0xe4, 0xac, 0x99, 0xc7, 0xc9, 0x38, 0x18, 0xf8, 0xb2, 0x51, 0x07, 0x22, 0xdf, 0xed, 0x97, 0xf3, 0x2e, 0x3e, 0x93, 0x49, 0xd4, 0xc6, 0x6c, 0x9e, 0xa6, 0x39, 0x6d, 0x74, 0x44, 0x62, 0xa0, 0x6b, 0x42, 0xc6, 0xd5, 0xba, 0x68, 0x8e, 0xac, 0x3a, 0x01, 0x7b, 0xdd, 0xfc, 0x8e, 0x2c, 0xfc, 0xad, 0x27, 0xcb, 0x69, 0xd3, 0xcc, 0xdc, 0xa2, 0x80, 0x41, 0x44, 0x65, 0xd3, 0xae, 0x34, 0x8c, 0xe0, 0xf3, 0x4a, 0xb2, 0xfb, 0x9c, 0x61, 0x83, 0x71, 0x31, 0x2b, 0x19, 0x10, 0x41, 0x64, 0x1c, 0x23, 0x7f, 0x11, 0xa5, 0xd6, 0x5c, 0x84, 0x4f, 0x04, 0x04, 0x84, 0x99, 0x38, 0x71, 0x2b, 0x95, 0x9e, 0xd6, 0x85, 0xbc, 0x5c, 0x5d, 0xd6, 0x45, 0xed, 0x19, 0x90, 0x94, 0x73, 0x40, 0x29, 0x26, 0xdc, 0xb4, 0x0e, 0x34, 0x69, 0xa1, 0x59, 0x41, 0xe8, 0xe2, 0xcc, 0xa8, 0x4b, 0xb6, 0x08, 0x46, 0x36, 0xa0}
	serverKeyExchange := []byte{0x0c, 0x00, 0x01, 0x28, 0x03, 0x00, 0x1d, 0x20, 0x9f, 0xd7, 0xad, 0x6d, 0xcf, 0xf4, 0x29, 0x8d, 0xd3, 0xf9, 0x6d, 0x5b, 0x1b, 0x2a, 0xf9, 0x10, 0xa0, 0x53, 0x5b, 0x14, 0x88, 0xd7, 0xf8, 0xfa, 0xbb, 0x34, 0x9a, 0x98, 0x28, 0x80, 0xb6, 0x15, 0x04, 0x01, 0x01, 0x00, 0x04, 0x02, 0xb6, 0x61, 0xf7, 0xc1, 0x91, 0xee, 0x59, 0xbe, 0x45, 0x37, 0x66, 0x39, 0xbd, 0xc3, 0xd4, 0xbb, 0x81, 0xe1, 0x15, 0xca, 0x73, 0xc8, 0x34, 0x8b, 0x52, 0x5b, 0x0d, 0x23, 0x38, 0xaa, 0x14, 0x46, 0x67, 0xed, 0x94, 0x31, 0x02, 0x14, 0x12, 0xcd, 0x9b, 0x84, 0x4c, 0xba, 0x29, 0x93, 0x4a, 0xaa, 0xcc, 0xe8, 0x73, 0x41, 0x4e, 0xc1, 0x1c, 0xb0, 0x2e, 0x27, 0x2d, 0x0a, 0xd8, 0x1f, 0x76, 0x7d, 0x33, 0x07, 0x67, 0x21, 0xf1, 0x3b, 0xf3, 0x60, 0x20, 0xcf, 0x0b, 0x1f, 0xd0, 0xec, 0xb0, 0x78, 0xde, 0x11, 0x28, 0xbe, 0xba, 0x09, 0x49, 0xeb, 0xec, 0xe1, 0xa1, 0xf9, 0x6e, 0x20, 0x9d, 0xc3, 0x6e, 0x4f, 0xff, 0xd3, 0x6b, 0x67, 0x3a, 0x7d, 0xdc, 0x15, 0x97, 0xad, 0x44, 0x08, 0xe4, 0x85, 0xc4, 0xad, 0xb2, 0xc8, 0x73, 0x84, 0x12, 0x49, 0x37, 0x25, 0x23, 0x80, 0x9e, 0x43, 0x12, 0xd0, 0xc7, 0xb3, 0x52, 0x2e, 0xf9, 0x83, 0xca, 0xc1, 0xe0, 0x39, 0x35, 0xff, 0x13, 0xa8, 0xe9, 0x6b, 0xa6, 0x81, 0xa6, 0x2e, 0x40, 0xd3, 0xe7, 0x0a, 0x7f, 0xf3, 0x58, 0x66, 0xd3, 0xd9, 0x99, 0x3f, 0x9e, 0x26, 0xa6, 0x34, 0xc8, 0x1b, 0x4e, 0x71, 0x38, 0x0f, 0xcd, 0xd6, 0xf4, 0xe8, 0x35, 0xf7, 0x5a, 0x64, 0x09, 0xc7, 0xdc, 0x2c, 0x07, 0x41, 0x0e, 0x6f, 0x87, 0x85, 0x8c, 0x7b, 0x94, 0xc0, 0x1c, 0x2e, 0x32, 0xf2, 0x91, 0x76, 0x9e, 0xac, 0xca, 0x71, 0x64, 0x3b, 0x8b, 0x98, 0xa9, 0x63, 0xdf, 0x0a, 0x32, 0x9b, 0xea, 0x4e, 0xd6, 0x39, 0x7e, 0x8c, 0xd0, 0x1a, 0x11, 0x0a, 0xb3, 0x61, 0xac, 0x5b, 0xad, 0x1c, 0xcd, 0x84, 0x0a, 0x6c, 0x8a, 0x6e, 0xaa, 0x00, 0x1a, 0x9d, 0x7d, 0x87, 0xdc, 0x33, 0x18, 0x64, 0x35, 0x71, 0x22, 0x6c, 0x4d, 0xd2, 0xc2, 0xac, 0x41, 0xfb}
	serverHelloDone := []byte{0x0e, 0x00, 0x00, 0x00}
	clientKeyExchange := []byte{0x10, 0x00, 0x00, 0x21, 0x20, 0x35, 0x80, 0x72, 0xd6, 0x36, 0x58, 0x80, 0xd1, 0xae, 0xea, 0x32, 0x9a, 0xdf, 0x91, 0x21, 0x38, 0x38, 0x51, 0xed, 0x21, 0xa2, 0x8e, 0x3b, 0x75, 0xe9, 0x65, 0xd0, 0xd2, 0xcd, 0x16, 0x62, 0x54}

	finalMsg := append(append(append(append(append(clientHello, serverHello...), serverCertificate...), serverKeyExchange...), serverHelloDone...), clientKeyExchange...)
	masterSecret := []byte{0x91, 0x6a, 0xbf, 0x9d, 0xa5, 0x59, 0x73, 0xe1, 0x36, 0x14, 0xae, 0x0a, 0x3f, 0x5d, 0x3f, 0x37, 0xb0, 0x23, 0xba, 0x12, 0x9a, 0xee, 0x02, 0xcc, 0x91, 0x34, 0x33, 0x81, 0x27, 0xcd, 0x70, 0x49, 0x78, 0x1c, 0x8e, 0x19, 0xfc, 0x1e, 0xb2, 0xa7, 0x38, 0x7a, 0xc0, 0x6a, 0xe2, 0x37, 0x34, 0x4c}

	expectedVerifyData := []byte{0xcf, 0x91, 0x96, 0x26, 0xf1, 0x36, 0x0c, 0x53, 0x6a, 0xaa, 0xd7, 0x3a}
	verifyData, err := VerifyDataClient(masterSecret, finalMsg, sha256.New)
	if err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(expectedVerifyData, verifyData) {
		t.Fatalf("verifyData exp: %q actual: %q", expectedVerifyData, verifyData)
	}
}
