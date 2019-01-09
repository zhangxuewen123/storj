// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"bytes"
	"crypto/x509"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zeebo/errs"

	"storj.io/storj/pkg/cfgstruct"
	"storj.io/storj/pkg/identity"
)

type verifyConfig struct {
	CA       identity.FullCAConfig
	Identity identity.Config
	Signer   identity.FullCAConfig
}

var (
	errVerify = errs.Class("Verify error")
	verifyCmd = &cobra.Command{
		Use:   "verify",
		Short: "Verify identity and CA certificate chains are valid",
		Long: `Verify identity and CA certificate chains are valid.

To be valid, an identity certificate chain must contain its CA's certificate chain, and both chains must consist of certificates signed by their respective parents, ending in a self-signed root.`,
		RunE: cmdVerify,
	}

	verifyCfg verifyConfig
)

type checkOpts struct {
	verifyConfig
	ca       *identity.FullCertificateAuthority
	ident    *identity.FullIdentity
	errGroup *errs.Group
}

func init() {
	rootCmd.AddCommand(verifyCmd)
	cfgstruct.Bind(verifyCmd.Flags(), &verifyCfg, cfgstruct.ConfDir(defaultConfDir))
}

func cmdVerify(cmd *cobra.Command, args []string) error {
	ca, err := verifyCfg.CA.Load()
	if err != nil {
		return err
	}

	ident, err := verifyCfg.Identity.Load()
	if err != nil {
		return err
	}

	opts := checkOpts{
		verifyConfig: verifyCfg,
		ca:           ca,
		ident:        ident,
		errGroup:     new(errs.Group),
	}
	checks := []struct {
		errFmt string
		run    func(checkOpts, string)
	}{
		{
			errFmt: "identity chain must contain CA chain: %s",
			run:    checkIdentContainsCA,
		},
		{
			errFmt: "identity chain must be valid: %s",
			run:    checkIdentChain,
		},
		{
			errFmt: "CA chain must be valid: %s",
			run:    checkCAChain,
		},
	}

	for _, check := range checks {
		check.run(opts, check.errFmt)
	}

	return opts.errGroup.Err()
}

func checkIdentChain(opts checkOpts, errFmt string) {
	identChain := append([]*x509.Certificate{
		opts.ident.Leaf,
		opts.ident.CA,
	}, opts.ident.RestChain...)

	verifyChain(identChain, errFmt, opts.errGroup)
}

func checkCAChain(opts checkOpts, errFmt string) {
	caChain := append([]*x509.Certificate{
		opts.ca.Cert,
	}, opts.ca.RestChain...)

	verifyChain(caChain, errFmt, opts.errGroup)
}

func checkIdentContainsCA(opts checkOpts, errFmt string) {
	identChainBytes := append([][]byte{
		opts.ident.Leaf.Raw,
		opts.ident.CA.Raw,
	}, opts.ca.RestChainRaw()...)
	caChainBytes := append([][]byte{
		opts.ca.Cert.Raw,
	}, opts.ca.RestChainRaw()...)

	for i, caCert := range caChainBytes {
		j := i + 1
		if len(identChainBytes) == j {
			opts.errGroup.Add(errVerify.New(errFmt, "ident chain should be longer than ca chain"))
			break
		}
		if bytes.Compare(caCert, identChainBytes[j]) != 0 {
			opts.errGroup.Add(errVerify.New(errFmt,
				fmt.Sprintf("ident and ca chains don't match at indicies %d and %d, respectively", j, i),
			))
		}
	}
}

func verifyChain(chain []*x509.Certificate, errFormat string, errGroup *errs.Group) {
	for i, cert := range chain {
		if i+1 == len(chain) {
			break
		}
		parent := chain[i+1]

		if err := cert.CheckSignatureFrom(parent); err != nil {
			errGroup.Add(errs.New(errFormat, err))
			break

		}
	}
}
