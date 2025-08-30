module github.com/censys-oss/dtls/v2

require (
	github.com/pion/dtls/v2 v2.2.11
	github.com/pion/logging v0.2.2
	github.com/pion/transport/v3 v3.0.2
	github.com/zmap/zcrypto v0.0.0-20250830192831-dcac38cad4c0
	golang.org/x/crypto v0.41.0
	golang.org/x/net v0.43.0
)

require (
	github.com/weppos/publicsuffix-go v0.50.1-0.20250829105427-5340293a34a1 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
)

go 1.23.0

toolchain go1.24.6
