// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package dtls

import (
	"github.com/censys-oss/dtls/v2/pkg/protocol/recordlayer"
)

type packet struct {
	record                   *recordlayer.RecordLayer
	shouldEncrypt            bool
	shouldWrapCID            bool
	resetLocalSequenceNumber bool
}
