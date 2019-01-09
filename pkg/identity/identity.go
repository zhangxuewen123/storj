// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package identity

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zeebo/errs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"storj.io/storj/pkg/peertls"
	"storj.io/storj/pkg/storj"
	"storj.io/storj/pkg/utils"
)

// PeerIdentity represents another peer on the network.
type PeerIdentity struct {
	RestChain []*x509.Certificate
	// CA represents the peer's self-signed CA
	CA *x509.Certificate
	// Leaf represents the leaf they're currently using. The leaf should be
	// signed by the CA. The leaf is what is used for communication.
	Leaf *x509.Certificate
	// The ID taken from the CA public key
	ID storj.NodeID
}

// FullIdentity represents you on the network. In addition to a PeerIdentity,
// a FullIdentity also has a Key, which a PeerIdentity doesn't have.
type FullIdentity struct {
	RestChain []*x509.Certificate
	// CA represents the peer's self-signed CA. The ID is taken from this cert.
	CA *x509.Certificate
	// Leaf represents the leaf they're currently using. The leaf should be
	// signed by the CA. The leaf is what is used for communication.
	Leaf *x509.Certificate
	// The ID taken from the CA public key
	ID storj.NodeID
	// Key is the key this identity uses with the leaf for communication.
	Key crypto.PrivateKey
}

// SetupConfig allows you to run a set of Responsibilities with the given
// identity. You can also just load an Identity from disk.
type SetupConfig struct {
	CertPath  string `help:"path to the certificate chain for this identity" default:"$CONFDIR/identity.cert"`
	KeyPath   string `help:"path to the private key for this identity" default:"$CONFDIR/identity.key"`
	Overwrite bool   `help:"if true, existing identity certs AND keys will overwritten for" default:"false"`
	Version   string `help:"semantic version of identity storage format" default:"0"`
}

// Config allows you to run a set of Responsibilities with the given
// identity. You can also just load an Identity from disk.
type Config struct {
	CertPath string `help:"path to the certificate chain for this identity" default:"$CONFDIR/identity.cert"`
	KeyPath  string `help:"path to the private key for this identity" default:"$CONFDIR/identity.key"`
}

// FullIdentityFromPEM loads a FullIdentity from a certificate chain and
// private key PEM-encoded bytes
func FullIdentityFromPEM(chainPEM, keyPEM []byte) (*FullIdentity, error) {
	chain, err := DecodeAndParseChainPEM(chainPEM)
	if err != nil {
		return nil, errs.Wrap(err)
	}
	if len(chain) < peertls.CAIndex+1 {
		return nil, ErrChainLength.New("identity chain does not contain a CA certificate")
	}
	keysBytes, err := decodePEM(keyPEM)
	if err != nil {
		return nil, errs.Wrap(err)
	}
	// NB: there shouldn't be multiple keys in the key file but if there
	// are, this uses the first one
	key, err := x509.ParseECPrivateKey(keysBytes[0])
	if err != nil {
		return nil, errs.New("unable to parse EC private key: %v", err)
	}
	nodeID, err := NodeIDFromKey(chain[peertls.CAIndex].PublicKey)
	if err != nil {
		return nil, err
	}

	return &FullIdentity{
		RestChain: chain[peertls.CAIndex+1:],
		CA:        chain[peertls.CAIndex],
		Leaf:      chain[peertls.LeafIndex],
		Key:       key,
		ID:        nodeID,
	}, nil
}

// ParseCertChain converts a chain of certificate bytes into x509 certs
func ParseCertChain(chain [][]byte) ([]*x509.Certificate, error) {
	c := make([]*x509.Certificate, len(chain))
	for i, ct := range chain {
		cp, err := x509.ParseCertificate(ct)
		if err != nil {
			return nil, errs.Wrap(err)
		}
		c[i] = cp
	}
	return c, nil
}

// PeerIdentityFromCerts loads a PeerIdentity from a pair of leaf and ca x509 certificates
func PeerIdentityFromCerts(leaf, ca *x509.Certificate, rest []*x509.Certificate) (*PeerIdentity, error) {
	i, err := NodeIDFromKey(ca.PublicKey)
	if err != nil {
		return nil, err
	}

	return &PeerIdentity{
		RestChain: rest,
		CA:        ca,
		ID:        i,
		Leaf:      leaf,
	}, nil
}

// PeerIdentityFromPeer loads a PeerIdentity from a peer connection
func PeerIdentityFromPeer(peer *peer.Peer) (*PeerIdentity, error) {
	tlsInfo := peer.AuthInfo.(credentials.TLSInfo)
	c := tlsInfo.State.PeerCertificates
	if len(c) < 2 {
		return nil, Error.New("invalid certificate chain")
	}
	pi, err := PeerIdentityFromCerts(c[peertls.LeafIndex], c[peertls.CAIndex], c[2:])
	if err != nil {
		return nil, err
	}

	return pi, nil
}

// PeerIdentityFromContext loads a PeerIdentity from a ctx TLS credentials
func PeerIdentityFromContext(ctx context.Context) (*PeerIdentity, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, Error.New("unable to get grpc peer from contex")
	}

	return PeerIdentityFromPeer(p)
}

// NodeIDFromKey hashes a public key and creates a node ID from it
func NodeIDFromKey(k crypto.PublicKey) (storj.NodeID, error) {
	if ek, ok := k.(*ecdsa.PublicKey); ok {
		return NodeIDFromECDSAKey(ek)
	}
	return storj.NodeID{}, storj.ErrNodeID.New("invalid key type: %T", k)
}

// NodeIDFromECDSAKey hashes a public key and creates a node ID from it
func NodeIDFromECDSAKey(k *ecdsa.PublicKey) (storj.NodeID, error) {
	// id = sha256(sha256(pkix(k)))
	kb, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return storj.NodeID{}, storj.ErrNodeID.Wrap(err)
	}
	mid := sha256.Sum256(kb)
	end := sha256.Sum256(mid[:])
	return storj.NodeIDFromBytes(end[:])
}

// NewFullIdentity creates a new ID for nodes with difficulty and concurrency params
func NewFullIdentity(ctx context.Context, difficulty uint16, concurrency uint) (*FullIdentity, error) {
	ca, err := NewCA(ctx, NewCAOptions{
		Difficulty:  difficulty,
		Concurrency: concurrency,
	})
	if err != nil {
		return nil, err
	}
	identity, err := ca.NewIdentity()
	if err != nil {
		return nil, err
	}
	return identity, err
}

// Status returns the status of the identity cert/key files for the config
func (is SetupConfig) Status() TLSFilesStatus {
	return statTLSFiles(is.CertPath, is.KeyPath)
}

// Create generates and saves a CA using the config
func (is SetupConfig) Create(ca *FullCertificateAuthority) (*FullIdentity, error) {
	fi, err := ca.NewIdentity()
	if err != nil {
		return nil, err
	}
	fi.CA = ca.Cert
	ic := Config{
		CertPath: is.CertPath,
		KeyPath:  is.KeyPath,
	}
	return fi, ic.Save(fi)
}

// FullConfig converts a `SetupConfig` to `Config`
func (is SetupConfig) FullConfig() Config {
	return Config{
		CertPath: is.CertPath,
		KeyPath:  is.KeyPath,
	}
}

// Load loads a FullIdentity from the config
func (ic Config) Load() (*FullIdentity, error) {
	c, err := ioutil.ReadFile(ic.CertPath)
	if err != nil {
		return nil, peertls.ErrNotExist.Wrap(err)
	}
	k, err := ioutil.ReadFile(ic.KeyPath)
	if err != nil {
		return nil, peertls.ErrNotExist.Wrap(err)
	}
	fi, err := FullIdentityFromPEM(c, k)
	if err != nil {
		return nil, errs.New("failed to load identity %#v, %#v: %v",
			ic.CertPath, ic.KeyPath, err)
	}
	return fi, nil
}

// Save saves a FullIdentity according to the config
func (ic Config) Save(fi *FullIdentity) error {
	var (
		certData, keyData                                              bytes.Buffer
		writeChainErr, writeChainDataErr, writeKeyErr, writeKeyDataErr error
	)

	chain := []*x509.Certificate{fi.Leaf, fi.CA}
	chain = append(chain, fi.RestChain...)

	if ic.CertPath != "" {
		writeChainErr = peertls.WriteChain(&certData, chain...)
		writeChainDataErr = writeChainData(ic.CertPath, certData.Bytes())
	}

	if ic.KeyPath != "" {
		writeKeyErr = peertls.WriteKey(&keyData, fi.Key)
		writeKeyDataErr = writeKeyData(ic.KeyPath, keyData.Bytes())
	}

	writeErr := utils.CombineErrors(writeChainErr, writeKeyErr)
	if writeErr != nil {
		return writeErr
	}

	return utils.CombineErrors(
		writeChainDataErr,
		writeKeyDataErr,
	)
}

// SaveBackup saves the certificate of the config with a timestamped filename
func (ic Config) SaveBackup(fi *FullIdentity) error {
	return Config{
		CertPath: backupPath(ic.CertPath),
	}.Save(fi)
}

// RestChainRaw returns the rest (excluding leaf and CA) of the certificate chain as a 2d byte slice
func (fi *FullIdentity) RestChainRaw() [][]byte {
	var chain [][]byte
	for _, cert := range fi.RestChain {
		chain = append(chain, cert.Raw)
	}
	return chain
}

// ServerOption returns a grpc `ServerOption` for incoming connections
// to the node with this full identity
func (fi *FullIdentity) ServerOption(pcvFuncs ...peertls.PeerCertVerificationFunc) (grpc.ServerOption, error) {
	ch := [][]byte{fi.Leaf.Raw, fi.CA.Raw}
	ch = append(ch, fi.RestChainRaw()...)
	c, err := peertls.TLSCert(ch, fi.Leaf, fi.Key)
	if err != nil {
		return nil, err
	}

	pcvFuncs = append(
		[]peertls.PeerCertVerificationFunc{peertls.VerifyPeerCertChains},
		pcvFuncs...,
	)
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{*c},
		InsecureSkipVerify: true,
		ClientAuth:         tls.RequireAnyClientCert,
		VerifyPeerCertificate: peertls.VerifyPeerFunc(
			pcvFuncs...,
		),
	}

	return grpc.Creds(credentials.NewTLS(tlsConfig)), nil
}

// DialOption returns a grpc `DialOption` for making outgoing connections
// to the node with this peer identity
// id is an optional id of the node we are dialing
func (fi *FullIdentity) DialOption(id storj.NodeID) (grpc.DialOption, error) {
	ch := [][]byte{fi.Leaf.Raw, fi.CA.Raw}
	ch = append(ch, fi.RestChainRaw()...)
	c, err := peertls.TLSCert(ch, fi.Leaf, fi.Key)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{*c},
		InsecureSkipVerify: true,
		VerifyPeerCertificate: peertls.VerifyPeerFunc(
			peertls.VerifyPeerCertChains,
			verifyIdentity(id),
		),
	}

	return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
}

func verifyIdentity(id storj.NodeID) peertls.PeerCertVerificationFunc {
	return func(_ [][]byte, parsedChains [][]*x509.Certificate) (err error) {
		defer mon.TaskNamed("verifyIdentity")(nil)(&err)
		if id == (storj.NodeID{}) {
			return nil
		}

		peer, err := PeerIdentityFromCerts(parsedChains[0][0], parsedChains[0][1], parsedChains[0][2:])
		if err != nil {
			return err
		}

		if peer.ID.String() != id.String() {
			return Error.New("peer ID did not match requested ID")
		}

		return nil
	}
}

func backupPath(path string) string {
	pathExt := filepath.Ext(path)
	base := strings.TrimSuffix(path, pathExt)
	return fmt.Sprintf(
		"%s.%s%s",
		base,
		strconv.Itoa(int(time.Now().Unix())),
		pathExt,
	)
}
