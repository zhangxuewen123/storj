package noise

import (
	"context"
	"net"

	"github.com/bifurcation/mint"
	"github.com/mimoo/NoiseGo/noise"
	"google.golang.org/grpc/credentials"
)

// Credentials is the credentials required for authenticating a connection using mint.TLS.
type Credentials struct {
	// noise TLS configuration
	Config     *noise.Config
	ServerName string
}

// NewCredentials uses c to construct a TransportCredentials based on TLS.
func NewCredentials(c *noise.Config) credentials.TransportCredentials {
	return &Credentials{CloneConfig(c), "storj"}
}

func NewConfig(publicKey, privateKey [32]byte, caPublicKey [32]byte, remoteKey [32]byte) *noise.Config {
	return &noise.Config{
		HandshakePattern: noise.Noise_KX,
		// the current peer's keyPair
		KeyPair: &noise.KeyPair{
			PrivateKey: [32]byte{},
			PublicKey:  [32]byte{},
		},
		RemoteKey: remoteKey[:],
		Prologue:  nil,
		// if the chosen handshake pattern requires the current peer to send a static
		// public key as part of the handshake, this proof over the key is mandatory
		// in order for the other peer to verify the current peer's key
		StaticPublicKeyProof: []byte{},
		// if the chosen handshake pattern requires the remote peer to send an unknown
		// static public key as part of the handshake, this callback is mandatory in
		// order to validate it
		PublicKeyVerifier: func(publicKey, proof []byte) bool { return true },
		// a pre-shared key for handshake patterns including a `psk` token
		PreSharedKey: []byte{},
		HalfDuplex:   false,
	}
}

func CloneConfig(config *noise.Config) *noise.Config {
	t := *config
	tkeypair := *t.KeyPair
	t.KeyPair = &tkeypair
	t.Prologue = cloneBytes(t.Prologue)
	t.StaticPublicKeyProof = cloneBytes(t.StaticPublicKeyProof)
	t.PreSharedKey = cloneBytes(t.PreSharedKey)
	return &t
}

func cloneBytes(xs []byte) []byte { return append([]byte{}, xs...) }

// ClientHandshake does the authentication handshake specified by the corresponding
// authentication protocol on rawConn for clients. It returns the authenticated
// connection and the corresponding auth information about the connection.
// Implementations must use the provided context to implement timely cancellation.
// gRPC will try to reconnect if the error returned is a temporary error
// (io.EOF, context.DeadlineExceeded or err.Temporary() == true).
// If the returned error is a wrapper error, implementations should make sure that
// the error implements Temporary() to have the correct retry behaviors.
//
// If the returned net.Conn is closed, it MUST close the net.Conn provided.
func (c *Credentials) ClientHandshake(ctx context.Context, authority string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	// use local conf to avoid clobbering ServerName if using multiple endpoints
	conf := CloneConfig(c.Config)
	conn := noise.Client(rawConn, conf)

	errChannel := make(chan error, 1)
	go func() {
		errChannel <- conn.Handshake()
	}()

	select {
	case err := <-errChannel:
		if err != nil && err != mint.AlertNoAlert {
			return nil, nil, err
		}
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	return conn, Info{}, nil
}

// ServerHandshake does the authentication handshake for servers. It returns
// the authenticated connection and the corresponding auth information about
// the connection.
//
// If the returned net.Conn is closed, it MUST close the net.Conn provided.
func (c *Credentials) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn := noise.Server(rawConn, c.Config)
	if err := conn.Handshake(); err != mint.AlertNoAlert {
		return nil, nil, err
	}

	ca, err := conn.StaticKey()
	if err != nil {
		return conn, Info{}, err
	}

	return conn, Info{
		PeerPublic: c.Config.RemoteKey,
		PeerCA:     ca,
	}, nil
}

// Info provides the ProtocolInfo of this TransportCredentials.
func (c Credentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{
		SecurityProtocol: "noise",
		SecurityVersion:  "0.1",
		ServerName:       "storj", //TODO:
	}
}

// Clone makes a copy of this TransportCredentials.
func (c *Credentials) Clone() credentials.TransportCredentials {
	return NewCredentials(c.Config)
}

// OverrideServerName overrides the server name used to verify the hostname on the returned certificates from the server.
// gRPC internals also use it to override the virtual hosting name if it is set.
// It must be called before dialing. Currently, this is only used by grpclb.
func (c *Credentials) OverrideServerName(serverNameOverride string) error {
	c.ServerName = serverNameOverride
	return nil
}

// Info contains the auth information for a TLS authenticated connection.
// It implements the AuthInfo interface.
type Info struct {
	PeerPublic []byte
	PeerCA     []byte
}

// AuthInfo defines the common interface for the auth information the users are interested in.
type AuthInfo interface {
	AuthType() string
}

// AuthType returns the type of Info as a string.
func (t Info) AuthType() string {
	return "noise"
}
