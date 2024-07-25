// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package handshake

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/censys-oss/dtls/v2/pkg/protocol"
	"github.com/censys-oss/dtls/v2/pkg/protocol/extension"
)

func TestHandshakeMessageServerHello(t *testing.T) {
	rawServerHello := []byte{
		0xfe, 0xfd, 0x21, 0x63, 0x32, 0x21, 0x81, 0x0e, 0x98, 0x6c,
		0x85, 0x3d, 0xa4, 0x39, 0xaf, 0x5f, 0xd6, 0x5c, 0xcc, 0x20,
		0x7f, 0x7c, 0x78, 0xf1, 0x5f, 0x7e, 0x1c, 0xb7, 0xa1, 0x1e,
		0xcf, 0x63, 0x84, 0x28, 0x00, 0xc0, 0x2b, 0x00, 0x00, 0x00,
	}

	cipherSuiteID := uint16(0xc02b)

	parsedServerHello := &MessageServerHello{
		Version: protocol.Version{Major: 0xFE, Minor: 0xFD},
		Random: Random{
			GMTUnixTime: time.Unix(560149025, 0),
			RandomBytes: [28]byte{0x81, 0x0e, 0x98, 0x6c, 0x85, 0x3d, 0xa4, 0x39, 0xaf, 0x5f, 0xd6, 0x5c, 0xcc, 0x20, 0x7f, 0x7c, 0x78, 0xf1, 0x5f, 0x7e, 0x1c, 0xb7, 0xa1, 0x1e, 0xcf, 0x63, 0x84, 0x28},
		},
		SessionID:         []byte{},
		CipherSuiteID:     &cipherSuiteID,
		CompressionMethod: &protocol.CompressionMethod{},
		Extensions:        []extension.Extension{},
	}

	c := &MessageServerHello{}
	if err := c.Unmarshal(rawServerHello); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(c, parsedServerHello) {
		t.Errorf("handshakeMessageServerHello unmarshal: got %#v, want %#v", c, parsedServerHello)
	}

	raw, err := c.Marshal()
	if err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(raw, rawServerHello) {
		t.Errorf("handshakeMessageServerHello marshal: got %#v, want %#v", raw, rawServerHello)
	}
}

func TestHandshakeMessageServerHelloSessionID(t *testing.T) {
	rawServerHello := []byte{
		0xfe, 0xfd, 0x21, 0x63, 0x32, 0x21, 0x81, 0x0e, 0x98, 0x6c,
		0x85, 0x3d, 0xa4, 0x39, 0xaf, 0x5f, 0xd6, 0x5c, 0xcc, 0x20,
		0x7f, 0x7c, 0x78, 0xf1, 0x5f, 0x7e, 0x1c, 0xb7, 0xa1, 0x1e,
		0xcf, 0x63, 0x84, 0x28, 0x20, 0xe0, 0xe1, 0xe2, 0xe3, 0xe4,
		0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xeb, 0xec, 0xed, 0xee,
		0xef, 0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8,
		0xf9, 0xfa, 0xfb, 0xfc, 0xfd, 0xfe, 0xff, 0xc0, 0x2b, 0x00,
		0x00, 0x00,
	}

	sessionID := []byte{
		0xe0, 0xe1, 0xe2, 0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9,
		0xea, 0xeb, 0xec, 0xed, 0xee, 0xef, 0xf0, 0xf1, 0xf2, 0xf3,
		0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9, 0xfa, 0xfb, 0xfc, 0xfd,
		0xfe, 0xff,
	}

	c := &MessageServerHello{}
	if err := c.Unmarshal(rawServerHello); err != nil {
		t.Error(err)
	} else if !bytes.Equal(c.SessionID, sessionID) {
		t.Errorf("handshakeMessageServerHello invalid SessionID: got %#v, want %#v", c.SessionID, sessionID)
	}

	raw, err := c.Marshal()
	if err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(raw, rawServerHello) {
		t.Errorf("handshakeMessageServerHello marshal: got %#v, want %#v", raw, rawServerHello)
	}
}
