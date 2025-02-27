// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package dtls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v3/deadline"
	"github.com/pion/transport/v3/netctx"
	"github.com/pion/transport/v3/replaydetector"
	"github.com/censys-oss/dtls/v2/internal/closer"
	"github.com/censys-oss/dtls/v2/pkg/crypto/elliptic"
	"github.com/censys-oss/dtls/v2/pkg/crypto/signaturehash"
	"github.com/censys-oss/dtls/v2/pkg/protocol"
	"github.com/censys-oss/dtls/v2/pkg/protocol/alert"
	"github.com/censys-oss/dtls/v2/pkg/protocol/handshake"
	"github.com/censys-oss/dtls/v2/pkg/protocol/recordlayer"
	"github.com/zmap/zcrypto/tls"
)

const (
	initialTickerInterval = time.Second
	cookieLength          = 20
	sessionLength         = 32
	defaultNamedCurve     = elliptic.X25519
	inboundBufferSize     = 8192
	// Default replay protection window is specified by RFC 6347 Section 4.1.2.6
	defaultReplayProtectionWindow = 64
	// maxAppDataPacketQueueSize is the maximum number of app data packets we will
	// enqueue before the handshake is completed
	maxAppDataPacketQueueSize = 100
)

func invalidKeyingLabels() map[string]bool {
	return map[string]bool{
		"client finished": true,
		"server finished": true,
		"master secret":   true,
		"key expansion":   true,
	}
}

type addrPkt struct {
	rAddr net.Addr
	data  []byte
}

// Conn represents a DTLS connection
type Conn struct {
	lock           sync.RWMutex      // Internal lock (must not be public)
	nextConn       netctx.PacketConn // Embedded Conn, typically a udpconn we read/write from
	fragmentBuffer *fragmentBuffer   // out-of-order and missing fragment handling
	handshakeCache *handshakeCache   // caching of handshake messages for verifyData generation
	decrypted      chan interface{}  // Decrypted Application Data or error, pull by calling `Read`
	rAddr          net.Addr
	state          State // Internal state

	maximumTransmissionUnit int
	paddingLengthGenerator  func(uint) uint

	handshakeCompletedSuccessfully atomic.Value

	encryptedPackets []addrPkt

	connectionClosedByUser bool
	closeLock              sync.Mutex
	closed                 *closer.Closer
	handshakeLoopsFinished sync.WaitGroup

	readDeadline  *deadline.Deadline
	writeDeadline *deadline.Deadline

	log          logging.LeveledLogger
	handshakeLog tls.ServerHandshake

	reading               chan struct{}
	handshakeRecv         chan chan struct{}
	cancelHandshaker      func()
	cancelHandshakeReader func()

	fsm *handshakeFSM

	replayProtectionWindow uint
}

func createConn(nextConn net.PacketConn, rAddr net.Addr, config *Config, isClient bool) (*Conn, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	if nextConn == nil {
		return nil, errNilNextConn
	}

	loggerFactory := config.LoggerFactory
	if loggerFactory == nil {
		loggerFactory = logging.NewDefaultLoggerFactory()
	}

	logger := loggerFactory.NewLogger("dtls")

	mtu := config.MTU
	if mtu <= 0 {
		mtu = defaultMTU
	}

	replayProtectionWindow := config.ReplayProtectionWindow
	if replayProtectionWindow <= 0 {
		replayProtectionWindow = defaultReplayProtectionWindow
	}

	paddingLengthGenerator := config.PaddingLengthGenerator
	if paddingLengthGenerator == nil {
		paddingLengthGenerator = func(uint) uint { return 0 }
	}

	c := &Conn{
		rAddr:                   rAddr,
		nextConn:                netctx.NewPacketConn(nextConn),
		fragmentBuffer:          newFragmentBuffer(),
		handshakeCache:          newHandshakeCache(),
		maximumTransmissionUnit: mtu,
		paddingLengthGenerator:  paddingLengthGenerator,

		decrypted: make(chan interface{}, 1),
		log:       logger,

		readDeadline:  deadline.New(),
		writeDeadline: deadline.New(),

		reading:          make(chan struct{}, 1),
		handshakeRecv:    make(chan chan struct{}),
		closed:           closer.NewCloser(),
		cancelHandshaker: func() {},

		replayProtectionWindow: uint(replayProtectionWindow),

		state: State{
			isClient: isClient,
		},
	}

	c.setRemoteEpoch(0)
	c.setLocalEpoch(0)
	return c, nil
}

func handshakeConn(ctx context.Context, conn *Conn, config *Config, isClient bool, initialState *State) (*Conn, error) {
	if conn == nil {
		return nil, errNilNextConn
	}

	cipherSuites, err := parseCipherSuites(config.CipherSuites, config.CustomCipherSuites, config.includeCertificateSuites(), config.PSK != nil)
	if err != nil {
		return nil, err
	}

	signatureSchemes, err := signaturehash.ParseSignatureSchemes(config.SignatureSchemes, config.InsecureHashes)
	if err != nil {
		return nil, err
	}

	workerInterval := initialTickerInterval
	if config.FlightInterval != 0 {
		workerInterval = config.FlightInterval
	}

	serverName := config.ServerName
	// Do not allow the use of an IP address literal as an SNI value.
	// See RFC 6066, Section 3.
	if net.ParseIP(serverName) != nil {
		serverName = ""
	}

	curves := config.EllipticCurves
	if len(curves) == 0 {
		curves = defaultCurves
	}

	hsCfg := &handshakeConfig{
		localPSKCallback:              config.PSK,
		localPSKIdentityHint:          config.PSKIdentityHint,
		localCipherSuites:             cipherSuites,
		localSignatureSchemes:         signatureSchemes,
		extendedMasterSecret:          config.ExtendedMasterSecret,
		localSRTPProtectionProfiles:   config.SRTPProtectionProfiles,
		serverName:                    serverName,
		supportedProtocols:            config.SupportedProtocols,
		clientAuth:                    config.ClientAuth,
		localCertificates:             config.Certificates,
		insecureSkipVerify:            config.InsecureSkipVerify,
		verifyPeerCertificate:         config.VerifyPeerCertificate,
		verifyConnection:              config.VerifyConnection,
		rootCAs:                       config.RootCAs,
		clientCAs:                     config.ClientCAs,
		customCipherSuites:            config.CustomCipherSuites,
		retransmitInterval:            workerInterval,
		log:                           conn.log,
		initialEpoch:                  0,
		keyLogWriter:                  config.KeyLogWriter,
		sessionStore:                  config.SessionStore,
		ellipticCurves:                curves,
		localGetCertificate:           config.GetCertificate,
		localGetClientCertificate:     config.GetClientCertificate,
		insecureSkipHelloVerify:       config.InsecureSkipVerifyHello,
		connectionIDGenerator:         config.ConnectionIDGenerator,
		helloRandomBytesGenerator:     config.HelloRandomBytesGenerator,
		clientHelloMessageHook:        config.ClientHelloMessageHook,
		serverHelloMessageHook:        config.ServerHelloMessageHook,
		certificateRequestMessageHook: config.CertificateRequestMessageHook,
	}

	// rfc5246#section-7.4.3
	// In addition, the hash and signature algorithms MUST be compatible
	// with the key in the server's end-entity certificate.
	if !isClient {
		cert, err := hsCfg.getCertificate(&ClientHelloInfo{})
		if err != nil && !errors.Is(err, errNoCertificates) {
			return nil, err
		}
		hsCfg.localCipherSuites = filterCipherSuitesForCertificate(cert, cipherSuites)
	}

	var initialFlight flightVal
	var initialFSMState handshakeState

	if initialState != nil {
		if conn.state.isClient {
			initialFlight = flight5
		} else {
			initialFlight = flight6
		}
		initialFSMState = handshakeFinished

		conn.state = *initialState
	} else {
		if conn.state.isClient {
			initialFlight = flight1
		} else {
			initialFlight = flight0
		}
		initialFSMState = handshakePreparing
	}
	// Do handshake
	if err := conn.handshake(ctx, hsCfg, initialFlight, initialFSMState); err != nil {
		return nil, err
	}

	conn.log.Trace("Handshake Completed")

	return conn, nil
}

// Dial connects to the given network address and establishes a DTLS connection on top.
// Connection handshake will timeout using ConnectContextMaker in the Config.
// If you want to specify the timeout duration, use DialWithContext() instead.
func Dial(network string, rAddr *net.UDPAddr, config *Config) (*Conn, error) {
	ctx, cancel := config.connectContextMaker()
	defer cancel()

	return DialWithContext(ctx, network, rAddr, config)
}

// Client establishes a DTLS connection over an existing connection.
// Connection handshake will timeout using ConnectContextMaker in the Config.
// If you want to specify the timeout duration, use ClientWithContext() instead.
func Client(conn net.PacketConn, rAddr net.Addr, config *Config) (*Conn, error) {
	ctx, cancel := config.connectContextMaker()
	defer cancel()

	return ClientWithContext(ctx, conn, rAddr, config)
}

// Server listens for incoming DTLS connections.
// Connection handshake will timeout using ConnectContextMaker in the Config.
// If you want to specify the timeout duration, use ServerWithContext() instead.
func Server(conn net.PacketConn, rAddr net.Addr, config *Config) (*Conn, error) {
	ctx, cancel := config.connectContextMaker()
	defer cancel()

	return ServerWithContext(ctx, conn, rAddr, config)
}

// DialWithContext connects to the given network address and establishes a DTLS
// connection on top.
func DialWithContext(ctx context.Context, network string, rAddr *net.UDPAddr, config *Config) (*Conn, error) {
	// net.ListenUDP is used rather than net.DialUDP as the latter prevents the
	// use of net.PacketConn.WriteTo.
	// https://github.com/golang/go/blob/ce5e37ec21442c6eb13a43e68ca20129102ebac0/src/net/udpsock_posix.go#L115
	pConn, err := net.ListenUDP(network, nil)
	if err != nil {
		return nil, err
	}

	return ClientWithContext(ctx, pConn, rAddr, config)
}

// ClientWithContext establishes a DTLS connection over an existing connection.
func ClientWithContext(ctx context.Context, conn net.PacketConn, rAddr net.Addr, config *Config) (*Conn, error) {
	switch {
	case config == nil:
		return nil, errNoConfigProvided
	case config.PSK != nil && config.PSKIdentityHint == nil:
		return nil, errPSKAndIdentityMustBeSetForClient
	}

	dconn, err := createConn(conn, rAddr, config, true)
	if err != nil {
		return nil, err
	}

	return handshakeConn(ctx, dconn, config, true, nil)
}

// ServerWithContext listens for incoming DTLS connections.
func ServerWithContext(ctx context.Context, conn net.PacketConn, rAddr net.Addr, config *Config) (*Conn, error) {
	if config == nil {
		return nil, errNoConfigProvided
	}
	dconn, err := createConn(conn, rAddr, config, false)
	if err != nil {
		return nil, err
	}
	return handshakeConn(ctx, dconn, config, false, nil)
}

// Read reads data from the connection.
func (c *Conn) Read(p []byte) (n int, err error) {
	if !c.isHandshakeCompletedSuccessfully() {
		return 0, errHandshakeInProgress
	}

	select {
	case <-c.readDeadline.Done():
		return 0, errDeadlineExceeded
	default:
	}

	for {
		select {
		case <-c.readDeadline.Done():
			return 0, errDeadlineExceeded
		case out, ok := <-c.decrypted:
			if !ok {
				return 0, io.EOF
			}
			switch val := out.(type) {
			case ([]byte):
				if len(p) < len(val) {
					return 0, errBufferTooSmall
				}
				copy(p, val)
				return len(val), nil
			case (error):
				return 0, val
			}
		}
	}
}

// Write writes len(p) bytes from p to the DTLS connection
func (c *Conn) Write(p []byte) (int, error) {
	if c.isConnectionClosed() {
		return 0, ErrConnClosed
	}

	select {
	case <-c.writeDeadline.Done():
		return 0, errDeadlineExceeded
	default:
	}

	if !c.isHandshakeCompletedSuccessfully() {
		return 0, errHandshakeInProgress
	}

	return len(p), c.writePackets(c.writeDeadline, []*packet{
		{
			record: &recordlayer.RecordLayer{
				Header: recordlayer.Header{
					Epoch:   c.state.getLocalEpoch(),
					Version: protocol.Version1_2,
				},
				Content: &protocol.ApplicationData{
					Data: p,
				},
			},
			shouldWrapCID: len(c.state.remoteConnectionID) > 0,
			shouldEncrypt: true,
		},
	})
}

// Close closes the connection.
func (c *Conn) Close() error {
	err := c.close(true) //nolint:contextcheck
	c.handshakeLoopsFinished.Wait()
	return err
}

// ConnectionState returns basic DTLS details about the connection.
// Note that this replaced the `Export` function of v1.
func (c *Conn) ConnectionState() State {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return *c.state.clone()
}

// SelectedSRTPProtectionProfile returns the selected SRTPProtectionProfile
func (c *Conn) SelectedSRTPProtectionProfile() (SRTPProtectionProfile, bool) {
	profile := c.state.getSRTPProtectionProfile()
	if profile == 0 {
		return 0, false
	}

	return profile, true
}

func (c *Conn) writePackets(ctx context.Context, pkts []*packet) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	var rawPackets [][]byte

	for _, p := range pkts {
		if h, ok := p.record.Content.(*handshake.Handshake); ok {
			handshakeRaw, err := p.record.Marshal()
			if err != nil {
				return err
			}

			c.log.Tracef("[handshake:%v] -> %s (epoch: %d, seq: %d)",
				srvCliStr(c.state.isClient), h.Header.Type.String(),
				p.record.Header.Epoch, h.Header.MessageSequence)

			c.handshakeCache.push(handshakeRaw[recordlayer.FixedHeaderSize:], p.record.Header.Epoch, h.Header.MessageSequence, h.Header.Type, c.state.isClient)

			rawHandshakePackets, err := c.processHandshakePacket(p, h)
			if err != nil {
				return err
			}
			rawPackets = append(rawPackets, rawHandshakePackets...)
		} else {
			rawPacket, err := c.processPacket(p)
			if err != nil {
				return err
			}
			rawPackets = append(rawPackets, rawPacket)
		}
	}
	if len(rawPackets) == 0 {
		return nil
	}
	compactedRawPackets := c.compactRawPackets(rawPackets)

	for _, compactedRawPackets := range compactedRawPackets {
		if _, err := c.nextConn.WriteToContext(ctx, compactedRawPackets, c.rAddr); err != nil {
			return netError(err)
		}
	}

	return nil
}

func (c *Conn) compactRawPackets(rawPackets [][]byte) [][]byte {
	// avoid a useless copy in the common case
	if len(rawPackets) == 1 {
		return rawPackets
	}

	combinedRawPackets := make([][]byte, 0)
	currentCombinedRawPacket := make([]byte, 0)

	for _, rawPacket := range rawPackets {
		if len(currentCombinedRawPacket) > 0 && len(currentCombinedRawPacket)+len(rawPacket) >= c.maximumTransmissionUnit {
			combinedRawPackets = append(combinedRawPackets, currentCombinedRawPacket)
			currentCombinedRawPacket = []byte{}
		}
		currentCombinedRawPacket = append(currentCombinedRawPacket, rawPacket...)
	}

	combinedRawPackets = append(combinedRawPackets, currentCombinedRawPacket)

	return combinedRawPackets
}

func (c *Conn) processPacket(p *packet) ([]byte, error) {
	epoch := p.record.Header.Epoch
	for len(c.state.localSequenceNumber) <= int(epoch) {
		c.state.localSequenceNumber = append(c.state.localSequenceNumber, uint64(0))
	}
	seq := atomic.AddUint64(&c.state.localSequenceNumber[epoch], 1) - 1
	if seq > recordlayer.MaxSequenceNumber {
		// RFC 6347 Section 4.1.0
		// The implementation must either abandon an association or rehandshake
		// prior to allowing the sequence number to wrap.
		return nil, errSequenceNumberOverflow
	}
	p.record.Header.SequenceNumber = seq

	var rawPacket []byte
	if p.shouldWrapCID {
		// Record must be marshaled to populate fields used in inner plaintext.
		if _, err := p.record.Marshal(); err != nil {
			return nil, err
		}
		content, err := p.record.Content.Marshal()
		if err != nil {
			return nil, err
		}
		inner := &recordlayer.InnerPlaintext{
			Content:  content,
			RealType: p.record.Header.ContentType,
		}
		rawInner, err := inner.Marshal() //nolint:govet
		if err != nil {
			return nil, err
		}
		cidHeader := &recordlayer.Header{
			Version:        p.record.Header.Version,
			ContentType:    protocol.ContentTypeConnectionID,
			Epoch:          p.record.Header.Epoch,
			ContentLen:     uint16(len(rawInner)),
			ConnectionID:   c.state.remoteConnectionID,
			SequenceNumber: p.record.Header.SequenceNumber,
		}
		rawPacket, err = cidHeader.Marshal()
		if err != nil {
			return nil, err
		}
		p.record.Header = *cidHeader
		rawPacket = append(rawPacket, rawInner...)
	} else {
		var err error
		rawPacket, err = p.record.Marshal()
		if err != nil {
			return nil, err
		}
	}

	if p.shouldEncrypt {
		var err error
		rawPacket, err = c.state.cipherSuite.Encrypt(p.record, rawPacket)
		if err != nil {
			return nil, err
		}
	}

	return rawPacket, nil
}

func (c *Conn) processHandshakePacket(p *packet, h *handshake.Handshake) ([][]byte, error) {
	rawPackets := make([][]byte, 0)

	handshakeFragments, err := c.fragmentHandshake(h)
	if err != nil {
		return nil, err
	}
	epoch := p.record.Header.Epoch
	for len(c.state.localSequenceNumber) <= int(epoch) {
		c.state.localSequenceNumber = append(c.state.localSequenceNumber, uint64(0))
	}

	for _, handshakeFragment := range handshakeFragments {
		seq := atomic.AddUint64(&c.state.localSequenceNumber[epoch], 1) - 1
		if seq > recordlayer.MaxSequenceNumber {
			return nil, errSequenceNumberOverflow
		}

		var rawPacket []byte
		if p.shouldWrapCID {
			inner := &recordlayer.InnerPlaintext{
				Content:  handshakeFragment,
				RealType: protocol.ContentTypeHandshake,
				Zeros:    c.paddingLengthGenerator(uint(len(handshakeFragment))),
			}
			rawInner, err := inner.Marshal() //nolint:govet
			if err != nil {
				return nil, err
			}
			cidHeader := &recordlayer.Header{
				Version:        p.record.Header.Version,
				ContentType:    protocol.ContentTypeConnectionID,
				Epoch:          p.record.Header.Epoch,
				ContentLen:     uint16(len(rawInner)),
				ConnectionID:   c.state.remoteConnectionID,
				SequenceNumber: p.record.Header.SequenceNumber,
			}
			rawPacket, err = cidHeader.Marshal()
			if err != nil {
				return nil, err
			}
			p.record.Header = *cidHeader
			rawPacket = append(rawPacket, rawInner...)
		} else {
			recordlayerHeader := &recordlayer.Header{
				Version:        p.record.Header.Version,
				ContentType:    p.record.Header.ContentType,
				ContentLen:     uint16(len(handshakeFragment)),
				Epoch:          p.record.Header.Epoch,
				SequenceNumber: seq,
			}

			rawPacket, err = recordlayerHeader.Marshal()
			if err != nil {
				return nil, err
			}

			p.record.Header = *recordlayerHeader
			rawPacket = append(rawPacket, handshakeFragment...)
		}

		if p.shouldEncrypt {
			var err error
			rawPacket, err = c.state.cipherSuite.Encrypt(p.record, rawPacket)
			if err != nil {
				return nil, err
			}
		}

		rawPackets = append(rawPackets, rawPacket)
	}

	return rawPackets, nil
}

func (c *Conn) fragmentHandshake(h *handshake.Handshake) ([][]byte, error) {
	content, err := h.Message.Marshal()
	if err != nil {
		return nil, err
	}

	fragmentedHandshakes := make([][]byte, 0)

	contentFragments := splitBytes(content, c.maximumTransmissionUnit)
	if len(contentFragments) == 0 {
		contentFragments = [][]byte{
			{},
		}
	}

	offset := 0
	for _, contentFragment := range contentFragments {
		contentFragmentLen := len(contentFragment)

		headerFragment := &handshake.Header{
			Type:            h.Header.Type,
			Length:          h.Header.Length,
			MessageSequence: h.Header.MessageSequence,
			FragmentOffset:  uint32(offset),
			FragmentLength:  uint32(contentFragmentLen),
		}

		offset += contentFragmentLen

		fragmentedHandshake, err := headerFragment.Marshal()
		if err != nil {
			return nil, err
		}

		fragmentedHandshake = append(fragmentedHandshake, contentFragment...)
		fragmentedHandshakes = append(fragmentedHandshakes, fragmentedHandshake)
	}

	return fragmentedHandshakes, nil
}

var poolReadBuffer = sync.Pool{ //nolint:gochecknoglobals
	New: func() interface{} {
		b := make([]byte, inboundBufferSize)
		return &b
	},
}

func (c *Conn) readAndBuffer(ctx context.Context) error {
	bufptr, ok := poolReadBuffer.Get().(*[]byte)
	if !ok {
		return errFailedToAccessPoolReadBuffer
	}
	defer poolReadBuffer.Put(bufptr)

	b := *bufptr
	i, rAddr, err := c.nextConn.ReadFromContext(ctx, b)
	if err != nil {
		return netError(err)
	}

	pkts, err := recordlayer.ContentAwareUnpackDatagram(b[:i], len(c.state.localConnectionID))
	if err != nil {
		return err
	}

	var hasHandshake bool
	for _, p := range pkts {
		hs, alert, err := c.handleIncomingPacket(ctx, p, rAddr, true)
		if alert != nil {
			if alertErr := c.notify(ctx, alert.Level, alert.Description); alertErr != nil {
				if err == nil {
					err = alertErr
				}
			}
		}

		var e *alertError
		if errors.As(err, &e) && e.IsFatalOrCloseNotify() {
			return e
		}
		if err != nil {
			return err
		}
		if hs {
			hasHandshake = true
		}
	}
	if hasHandshake {
		done := make(chan struct{})
		select {
		case c.handshakeRecv <- done:
			// If the other party may retransmit the flight,
			// we should respond even if it not a new message.
			<-done
		case <-c.fsm.Done():
		}
	}
	return nil
}

func (c *Conn) handleQueuedPackets(ctx context.Context) error {
	pkts := c.encryptedPackets
	c.encryptedPackets = nil

	for _, p := range pkts {
		_, alert, err := c.handleIncomingPacket(ctx, p.data, p.rAddr, false) // don't re-enqueue
		if alert != nil {
			if alertErr := c.notify(ctx, alert.Level, alert.Description); alertErr != nil {
				if err == nil {
					err = alertErr
				}
			}
		}
		var e *alertError
		if errors.As(err, &e) && e.IsFatalOrCloseNotify() {
			return e
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) enqueueEncryptedPackets(packet addrPkt) bool {
	if len(c.encryptedPackets) < maxAppDataPacketQueueSize {
		c.encryptedPackets = append(c.encryptedPackets, packet)
		return true
	}
	return false
}

func (c *Conn) handleIncomingPacket(ctx context.Context, buf []byte, rAddr net.Addr, enqueue bool) (bool, *alert.Alert, error) { //nolint:gocognit
	h := &recordlayer.Header{}
	// Set connection ID size so that records of content type tls12_cid will
	// be parsed correctly.
	if len(c.state.localConnectionID) > 0 {
		h.ConnectionID = make([]byte, len(c.state.localConnectionID))
	}
	if err := h.Unmarshal(buf); err != nil {
		// Decode error must be silently discarded
		// [RFC6347 Section-4.1.2.7]
		c.log.Debugf("discarded broken packet: %v", err)
		return false, nil, nil
	}
	// Validate epoch
	remoteEpoch := c.state.getRemoteEpoch()
	if h.Epoch > remoteEpoch {
		if h.Epoch > remoteEpoch+1 {
			c.log.Debugf("discarded future packet (epoch: %d, seq: %d)",
				h.Epoch, h.SequenceNumber,
			)
			return false, nil, nil
		}
		if enqueue {
			if ok := c.enqueueEncryptedPackets(addrPkt{rAddr, buf}); ok {
				c.log.Debug("received packet of next epoch, queuing packet")
			}
		}
		return false, nil, nil
	}

	// Anti-replay protection
	for len(c.state.replayDetector) <= int(h.Epoch) {
		c.state.replayDetector = append(c.state.replayDetector,
			replaydetector.New(c.replayProtectionWindow, recordlayer.MaxSequenceNumber),
		)
	}
	markPacketAsValid, ok := c.state.replayDetector[int(h.Epoch)].Check(h.SequenceNumber)
	if !ok {
		c.log.Debugf("discarded duplicated packet (epoch: %d, seq: %d)",
			h.Epoch, h.SequenceNumber,
		)
		return false, nil, nil
	}

	// originalCID indicates whether the original record had content type
	// Connection ID.
	originalCID := false

	// Decrypt
	if h.Epoch != 0 {
		if c.state.cipherSuite == nil || !c.state.cipherSuite.IsInitialized() {
			if enqueue {
				if ok := c.enqueueEncryptedPackets(addrPkt{rAddr, buf}); ok {
					c.log.Debug("handshake not finished, queuing packet")
				}
			}
			return false, nil, nil
		}

		// If a connection identifier had been negotiated and encryption is
		// enabled, the connection identifier MUST be sent.
		if len(c.state.localConnectionID) > 0 && h.ContentType != protocol.ContentTypeConnectionID {
			c.log.Debug("discarded packet missing connection ID after value negotiated")
			return false, nil, nil
		}

		var err error
		var hdr recordlayer.Header
		if h.ContentType == protocol.ContentTypeConnectionID {
			hdr.ConnectionID = make([]byte, len(c.state.localConnectionID))
		}
		buf, err = c.state.cipherSuite.Decrypt(hdr, buf)
		if err != nil {
			c.log.Debugf("%s: decrypt failed: %s", srvCliStr(c.state.isClient), err)
			return false, nil, nil
		}
		// If this is a connection ID record, make it look like a normal record for
		// further processing.
		if h.ContentType == protocol.ContentTypeConnectionID {
			originalCID = true
			ip := &recordlayer.InnerPlaintext{}
			if err := ip.Unmarshal(buf[h.Size():]); err != nil { //nolint:govet
				c.log.Debugf("unpacking inner plaintext failed: %s", err)
				return false, nil, nil
			}
			unpacked := &recordlayer.Header{
				ContentType:    ip.RealType,
				ContentLen:     uint16(len(ip.Content)),
				Version:        h.Version,
				Epoch:          h.Epoch,
				SequenceNumber: h.SequenceNumber,
			}
			buf, err = unpacked.Marshal()
			if err != nil {
				c.log.Debugf("converting CID record to inner plaintext failed: %s", err)
				return false, nil, nil
			}
			buf = append(buf, ip.Content...)
		}

		// If connection ID does not match discard the packet.
		if !bytes.Equal(c.state.localConnectionID, h.ConnectionID) {
			c.log.Debug("unexpected connection ID")
			return false, nil, nil
		}
	}

	isHandshake, err := c.fragmentBuffer.push(append([]byte{}, buf...))
	if err != nil {
		// Decode error must be silently discarded
		// [RFC6347 Section-4.1.2.7]
		c.log.Debugf("defragment failed: %s", err)
		return false, nil, nil
	} else if isHandshake {
		markPacketAsValid()
		for out, epoch := c.fragmentBuffer.pop(); out != nil; out, epoch = c.fragmentBuffer.pop() {
			header := &handshake.Header{}
			if err := header.Unmarshal(out); err != nil {
				c.log.Debugf("%s: handshake parse failed: %s", srvCliStr(c.state.isClient), err)
				continue
			}
			c.handshakeCache.push(out, epoch, header.MessageSequence, header.Type, !c.state.isClient)
		}

		return true, nil, nil
	}

	r := &recordlayer.RecordLayer{}
	if err := r.Unmarshal(buf); err != nil {
		return false, &alert.Alert{Level: alert.Fatal, Description: alert.DecodeError}, err
	}

	isLatestSeqNum := false
	switch content := r.Content.(type) {
	case *alert.Alert:
		c.log.Tracef("%s: <- %s", srvCliStr(c.state.isClient), content.String())
		var a *alert.Alert
		if content.Description == alert.CloseNotify {
			// Respond with a close_notify [RFC5246 Section 7.2.1]
			a = &alert.Alert{Level: alert.Warning, Description: alert.CloseNotify}
		}
		_ = markPacketAsValid()
		return false, a, &alertError{content}
	case *protocol.ChangeCipherSpec:
		if c.state.cipherSuite == nil || !c.state.cipherSuite.IsInitialized() {
			if enqueue {
				if ok := c.enqueueEncryptedPackets(addrPkt{rAddr, buf}); ok {
					c.log.Debugf("CipherSuite not initialized, queuing packet")
				}
			}
			return false, nil, nil
		}

		newRemoteEpoch := h.Epoch + 1
		c.log.Tracef("%s: <- ChangeCipherSpec (epoch: %d)", srvCliStr(c.state.isClient), newRemoteEpoch)

		if c.state.getRemoteEpoch()+1 == newRemoteEpoch {
			c.setRemoteEpoch(newRemoteEpoch)
			isLatestSeqNum = markPacketAsValid()
		}
	case *protocol.ApplicationData:
		if h.Epoch == 0 {
			return false, &alert.Alert{Level: alert.Fatal, Description: alert.UnexpectedMessage}, errApplicationDataEpochZero
		}

		isLatestSeqNum = markPacketAsValid()

		select {
		case c.decrypted <- content.Data:
		case <-c.closed.Done():
		case <-ctx.Done():
		}

	default:
		return false, &alert.Alert{Level: alert.Fatal, Description: alert.UnexpectedMessage}, fmt.Errorf("%w: %d", errUnhandledContextType, content.ContentType())
	}

	// Any valid connection ID record is a candidate for updating the remote
	// address if it is the latest record received.
	// https://datatracker.ietf.org/doc/html/rfc9146#peer-address-update
	if originalCID && isLatestSeqNum {
		if rAddr != c.RemoteAddr() {
			c.lock.Lock()
			c.rAddr = rAddr
			c.lock.Unlock()
		}
	}

	return false, nil, nil
}

func (c *Conn) recvHandshake() <-chan chan struct{} {
	return c.handshakeRecv
}

func (c *Conn) notify(ctx context.Context, level alert.Level, desc alert.Description) error {
	if level == alert.Fatal && len(c.state.SessionID) > 0 {
		// According to the RFC, we need to delete the stored session.
		// https://datatracker.ietf.org/doc/html/rfc5246#section-7.2
		if ss := c.fsm.cfg.sessionStore; ss != nil {
			c.log.Tracef("clean invalid session: %s", c.state.SessionID)
			if err := ss.Del(c.sessionKey()); err != nil {
				return err
			}
		}
	}
	return c.writePackets(ctx, []*packet{
		{
			record: &recordlayer.RecordLayer{
				Header: recordlayer.Header{
					Epoch:   c.state.getLocalEpoch(),
					Version: protocol.Version1_2,
				},
				Content: &alert.Alert{
					Level:       level,
					Description: desc,
				},
			},
			shouldWrapCID: len(c.state.remoteConnectionID) > 0,
			shouldEncrypt: c.isHandshakeCompletedSuccessfully(),
		},
	})
}

func (c *Conn) setHandshakeCompletedSuccessfully() {
	c.handshakeCompletedSuccessfully.Store(struct{ bool }{true})
}

func (c *Conn) isHandshakeCompletedSuccessfully() bool {
	boolean, _ := c.handshakeCompletedSuccessfully.Load().(struct{ bool })
	return boolean.bool
}

func (c *Conn) handshake(ctx context.Context, cfg *handshakeConfig, initialFlight flightVal, initialState handshakeState) error { //nolint:gocognit,contextcheck
	c.fsm = newHandshakeFSM(&c.state, c.handshakeCache, cfg, initialFlight)

	done := make(chan struct{})
	ctxRead, cancelRead := context.WithCancel(context.Background())
	c.cancelHandshakeReader = cancelRead
	cfg.onFlightState = func(_ flightVal, s handshakeState) {
		if s == handshakeFinished && !c.isHandshakeCompletedSuccessfully() {
			c.setHandshakeCompletedSuccessfully()
			close(done)
		}
	}

	ctxHs, cancel := context.WithCancel(context.Background())
	c.cancelHandshaker = cancel

	firstErr := make(chan error, 1)

	c.handshakeLoopsFinished.Add(2)

	// Handshake routine should be live until close.
	// The other party may request retransmission of the last flight to cope with packet drop.
	go func() {
		defer c.handshakeLoopsFinished.Done()
		err := c.fsm.Run(ctxHs, c, initialState)
		if !errors.Is(err, context.Canceled) {
			select {
			case firstErr <- err:
			default:
			}
		}
	}()
	go func() {
		defer func() {
			// Escaping read loop.
			// It's safe to close decrypted channnel now.
			close(c.decrypted)

			// Force stop handshaker when the underlying connection is closed.
			cancel()
		}()
		defer c.handshakeLoopsFinished.Done()
		for {
			if err := c.readAndBuffer(ctxRead); err != nil {
				var e *alertError
				if errors.As(err, &e) {
					if !e.IsFatalOrCloseNotify() {
						if c.isHandshakeCompletedSuccessfully() {
							// Pass the error to Read()
							select {
							case c.decrypted <- err:
							case <-c.closed.Done():
							case <-ctxRead.Done():
							}
						}
						continue // non-fatal alert must not stop read loop
					}
				} else {
					switch {
					case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
					case errors.Is(err, recordlayer.ErrInvalidPacketLength):
						// Decode error must be silently discarded
						// [RFC6347 Section-4.1.2.7]
						continue
					default:
						if c.isHandshakeCompletedSuccessfully() {
							// Keep read loop and pass the read error to Read()
							select {
							case c.decrypted <- err:
							case <-c.closed.Done():
							case <-ctxRead.Done():
							}
							continue // non-fatal alert must not stop read loop
						}
					}
				}

				select {
				case firstErr <- err:
				default:
				}

				if e != nil {
					if e.IsFatalOrCloseNotify() {
						_ = c.close(false) //nolint:contextcheck
					}
				}
				if !c.isConnectionClosed() && errors.Is(err, context.Canceled) {
					c.log.Trace("handshake timeouts - closing underline connection")
					_ = c.close(false) //nolint:contextcheck
				}
				return
			}
		}
	}()

	select {
	case err := <-firstErr:
		cancelRead()
		cancel()
		c.handshakeLoopsFinished.Wait()
		return c.translateHandshakeCtxError(err)
	case <-ctx.Done():
		cancelRead()
		cancel()
		c.handshakeLoopsFinished.Wait()
		return c.translateHandshakeCtxError(ctx.Err())
	case <-done:
		return nil
	}
}

func (c *Conn) translateHandshakeCtxError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) && c.isHandshakeCompletedSuccessfully() {
		return nil
	}
	return &HandshakeError{Err: err}
}

func (c *Conn) close(byUser bool) error {
	c.cancelHandshaker()
	c.cancelHandshakeReader()

	if c.isHandshakeCompletedSuccessfully() && byUser {
		// Discard error from notify() to return non-error on the first user call of Close()
		// even if the underlying connection is already closed.
		_ = c.notify(context.Background(), alert.Warning, alert.CloseNotify)
	}

	c.closeLock.Lock()
	// Don't return ErrConnClosed at the first time of the call from user.
	closedByUser := c.connectionClosedByUser
	if byUser {
		c.connectionClosedByUser = true
	}
	isClosed := c.isConnectionClosed()
	c.closed.Close()
	c.closeLock.Unlock()

	if closedByUser {
		return ErrConnClosed
	}

	if isClosed {
		return nil
	}

	return c.nextConn.Close()
}

func (c *Conn) isConnectionClosed() bool {
	select {
	case <-c.closed.Done():
		return true
	default:
		return false
	}
}

func (c *Conn) setLocalEpoch(epoch uint16) {
	c.state.localEpoch.Store(epoch)
}

func (c *Conn) setRemoteEpoch(epoch uint16) {
	c.state.remoteEpoch.Store(epoch)
}

// LocalAddr implements net.Conn.LocalAddr
func (c *Conn) LocalAddr() net.Addr {
	return c.nextConn.LocalAddr()
}

// RemoteAddr implements net.Conn.RemoteAddr
func (c *Conn) RemoteAddr() net.Addr {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.rAddr
}

func (c *Conn) sessionKey() []byte {
	if c.state.isClient {
		// As ServerName can be like 0.example.com, it's better to add
		// delimiter character which is not allowed to be in
		// neither address or domain name.
		return []byte(c.rAddr.String() + "_" + c.fsm.cfg.serverName)
	}
	return c.state.SessionID
}

// SetDeadline implements net.Conn.SetDeadline
func (c *Conn) SetDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn.SetReadDeadline
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	// Read deadline is fully managed by this layer.
	// Don't set read deadline to underlying connection.
	return nil
}

// SetWriteDeadline implements net.Conn.SetWriteDeadline
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Set(t)
	// Write deadline is also fully managed by this layer.
	return nil
}

func (c *Conn) GetHandshakeLog() *tls.ServerHandshake {
	hsLog := &tls.ServerHandshake{}
	s := c.fsm
	_, serverMsgs, ok := s.cache.fullPullMap(1, s.state.cipherSuite,
		handshakeCachePullRule{handshake.TypeServerHello, s.cfg.initialEpoch, false, true},
		handshakeCachePullRule{handshake.TypeCertificate, s.cfg.initialEpoch, false, true},
		handshakeCachePullRule{handshake.TypeServerKeyExchange, s.cfg.initialEpoch, false, true},
		handshakeCachePullRule{handshake.TypeCertificateRequest, s.cfg.initialEpoch, false, true},
		handshakeCachePullRule{handshake.TypeServerHelloDone, s.cfg.initialEpoch, false, true},
		handshakeCachePullRule{handshake.TypeFinished, s.cfg.initialEpoch + 1, false, true},
	)
	if !ok {
		return nil
	}
	_, clientMsgs, ok := s.cache.fullPullMap(1, s.state.cipherSuite,
		handshakeCachePullRule{handshake.TypeClientHello, s.cfg.initialEpoch, true, true},
		handshakeCachePullRule{handshake.TypeCertificate, s.cfg.initialEpoch, true, true},
		handshakeCachePullRule{handshake.TypeClientKeyExchange, s.cfg.initialEpoch, true, true},
		handshakeCachePullRule{handshake.TypeCertificateVerify, s.cfg.initialEpoch, true, true},
		handshakeCachePullRule{handshake.TypeFinished, s.cfg.initialEpoch + 1, true, true},
	)
	if !ok {
		return nil
	}

	for _, v := range clientMsgs {
		switch m := v.(type) {
		case *handshake.MessageClientHello:
			hsLog.ClientHello = m.MakeLog()
		case *handshake.MessageClientKeyExchange:
			hsLog.ClientKeyExchange = m.MakeLog()
		case *handshake.MessageFinished:
			hsLog.ClientFinished = m.MakeLog()
		case *handshake.MessageCertificateVerify: // unimplemented
		case *handshake.MessageCertificate: // unimplmented
		default:
			panic("Unexpected/Unknown message type: " + fmt.Sprintf("%T", v))
		}
	}
	for _, v := range serverMsgs {
		switch m := v.(type) {
		case *handshake.MessageServerHello:
			hsLog.ServerHello = m.MakeLog()
		case *handshake.MessageCertificate:
			hsLog.ServerCertificates = m.MakeLog()
		case *handshake.MessageServerKeyExchange:
			hsLog.ServerKeyExchange = m.MakeLog()
		case *handshake.MessageFinished:
			hsLog.ServerFinished = m.MakeLog()
		case *handshake.MessageServerHelloDone: // Not needed
		case *handshake.MessageCertificateRequest: // unimplemented
		default:
			panic("Unexpected/Unknown message type: " + fmt.Sprintf("%T", v))
		}
	}

	hsLog.KeyMaterial = &tls.KeyMaterial{
		MasterSecret: &tls.MasterSecret{
			Value:  c.state.masterSecret,
			Length: len(c.state.masterSecret),
		},
		PreMasterSecret: &tls.PreMasterSecret{
			Value:  c.state.preMasterSecret,
			Length: len(c.state.preMasterSecret),
		},
	}

	hsLog.SessionTicket = nil // > TLSv1.3 only
	return hsLog
}
