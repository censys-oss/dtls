// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package handshake

import (
	"encoding/binary"

	"github.com/censys-oss/dtls/v2/pkg/protocol"
	"github.com/censys-oss/dtls/v2/pkg/protocol/extension"
	"github.com/zmap/zcrypto/tls"
)

// MessageServerHello is sent in response to a ClientHello
// message when it was able to find an acceptable set of algorithms.
// If it cannot find such a match, it will respond with a handshake
// failure alert.
//
// https://tools.ietf.org/html/rfc5246#section-7.4.1.3
type MessageServerHello struct {
	Version protocol.Version
	Random  Random

	SessionID []byte

	CipherSuiteID     *uint16
	CompressionMethod *protocol.CompressionMethod
	Extensions        []extension.Extension
}

const messageServerHelloVariableWidthStart = 2 + RandomLength

// Type returns the Handshake Type
func (m MessageServerHello) Type() Type {
	return TypeServerHello
}

// Marshal encodes the Handshake
func (m *MessageServerHello) Marshal() ([]byte, error) {
	if m.CipherSuiteID == nil {
		return nil, errCipherSuiteUnset
	} else if m.CompressionMethod == nil {
		return nil, errCompressionMethodUnset
	}

	out := make([]byte, messageServerHelloVariableWidthStart)
	out[0] = m.Version.Major
	out[1] = m.Version.Minor

	rand := m.Random.MarshalFixed()
	copy(out[2:], rand[:])

	out = append(out, byte(len(m.SessionID)))
	out = append(out, m.SessionID...)

	out = append(out, []byte{0x00, 0x00}...)
	binary.BigEndian.PutUint16(out[len(out)-2:], *m.CipherSuiteID)

	out = append(out, byte(m.CompressionMethod.ID))

	extensions, err := extension.Marshal(m.Extensions)
	if err != nil {
		return nil, err
	}

	return append(out, extensions...), nil
}

// Unmarshal populates the message from encoded data
func (m *MessageServerHello) Unmarshal(data []byte) error {
	if len(data) < 2+RandomLength {
		return errBufferTooSmall
	}

	m.Version.Major = data[0]
	m.Version.Minor = data[1]

	var random [RandomLength]byte
	copy(random[:], data[2:])
	m.Random.UnmarshalFixed(random)

	currOffset := messageServerHelloVariableWidthStart
	currOffset++
	if len(data) <= currOffset {
		return errBufferTooSmall
	}

	n := int(data[currOffset-1])
	if len(data) <= currOffset+n {
		return errBufferTooSmall
	}
	m.SessionID = append([]byte{}, data[currOffset:currOffset+n]...)
	currOffset += len(m.SessionID)

	if len(data) < currOffset+2 {
		return errBufferTooSmall
	}
	m.CipherSuiteID = new(uint16)
	*m.CipherSuiteID = binary.BigEndian.Uint16(data[currOffset:])
	currOffset += 2

	if len(data) <= currOffset {
		return errBufferTooSmall
	}
	if compressionMethod, ok := protocol.CompressionMethods()[protocol.CompressionMethodID(data[currOffset])]; ok {
		m.CompressionMethod = compressionMethod
		currOffset++
	} else {
		return errInvalidCompressionMethod
	}

	if len(data) <= currOffset {
		m.Extensions = []extension.Extension{}
		return nil
	}

	extensions, err := extension.Unmarshal(data[currOffset:])
	if err != nil {
		return err
	}
	m.Extensions = extensions
	return nil
}

func (m *MessageServerHello) MakeLog() *tls.ServerHello {
	ret := &tls.ServerHello{}

	ret.Version = tls.TLSVersion((uint16(m.Version.Major) << 8) | uint16(m.Version.Minor))

	ret.Random = make([]byte, RandomLength)
	binary.BigEndian.PutUint32(ret.Random[:4], uint32(m.Random.GMTUnixTime.Unix()))
	copy(ret.Random[4:], m.Random.RandomBytes[:])

	ret.SessionID = make([]byte, len(m.SessionID))
	copy(ret.SessionID, m.SessionID)

	ret.CipherSuite = tls.CipherSuiteID(*m.CipherSuiteID)

	ret.CompressionMethod = uint8(m.CompressionMethod.ID)

	for _, anyExt := range m.Extensions {
		switch e := anyExt.(type) {
		case *extension.ALPN:
			if len(e.ProtocolNameList) > 0 {
				ret.AlpnProtocol = e.ProtocolNameList[0]
			}
		case *extension.RenegotiationInfo:
			ret.SecureRenegotiation = true
		case *extension.UseExtendedMasterSecret:
			ret.ExtendedMasterSecret = e.Supported

		// unimplemented in zcrypto
		case *extension.ConnectionID:
		case *extension.SupportedPointFormats:
		default:
		}

	}
	return ret
}
