package node

import "errors"

// Standard domain errors for Node Gateway, mTLS PKI, and Hypervisor Node management.
var (
	ErrNodeNotFound            = errors.New("node not found")
	ErrNodeAlreadyExists       = errors.New("node with this name or fqdn already exists")
	ErrNodeRevoked             = errors.New("node certificate has been revoked")
	ErrNodeOffline             = errors.New("node is currently offline")
	ErrNodeInMaintenance       = errors.New("node is in maintenance mode")
	ErrEnrollmentTokenInvalid  = errors.New("invalid or unrecognised enrollment token")
	ErrEnrollmentTokenExpired  = errors.New("enrollment token has expired")
	ErrEnrollmentTokenUsed     = errors.New("enrollment token has already been consumed")
	ErrInvalidCSR              = errors.New("invalid or unparseable pkcs#10 certificate signing request")
	ErrCertMismatch            = errors.New("client certificate fingerprint does not match registered node")
	ErrStreamClosed            = errors.New("grpc stream tunnel has closed")
	ErrCommandTimeout          = errors.New("timeout waiting for node command response")
	ErrCommandRejected         = errors.New("command rejected by node agent")
)
