// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/zeebo/errs"

	"storj.io/storj/pkg/certificates"
	"storj.io/storj/pkg/cfgstruct"
	"storj.io/storj/pkg/utils"
)

var (
	authCmd = &cobra.Command{
		Use:   "auth",
		Short: "CSR authorization management",
	}

	authCreateCmd = &cobra.Command{
		Use:   "create <auth_increment_count> [<email>, ...]",
		Short: "Create authorizations from a list of emails",
		Args:  cobra.MinimumNArgs(1),
		RunE:  cmdCreateAuth,
	}

	authInfoCmd = &cobra.Command{
		Use:   "info [<email>, ...]",
		Short: "Get authorization(s) info from CSR authorization DB",
		RunE:  cmdInfoAuth,
	}

	authExportCmd = &cobra.Command{
		Use:   "export [<email>, ...]",
		Short: "Export authorization(s) from CSR authorization DB to a CSV file (or stdout)",
		RunE:  cmdExportAuth,
	}

	authCreateCfg struct {
		certificates.CertServerConfig
		batchCfg
	}

	authInfoCfg struct {
		ShowTokens bool `help:"if true, token strings will be printed" default:"false"`
		certificates.CertServerConfig
		batchCfg
	}

	authExportCfg struct {
		All bool   `help:"export all authorizations" default:"false"`
		Out string `help:"output file path; if \"-\", will use STDOUT" default:"$CONFDIR/authorizations.csv"`
		certificates.CertServerConfig
		batchCfg
	}
)

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authCreateCmd)
	cfgstruct.Bind(authCreateCmd.Flags(), &authCreateCfg, cfgstruct.ConfDir(defaultConfDir))
	authCmd.AddCommand(authInfoCmd)
	cfgstruct.Bind(authInfoCmd.Flags(), &authInfoCfg, cfgstruct.ConfDir(defaultConfDir))
	authCmd.AddCommand(authExportCmd)
	cfgstruct.Bind(authExportCmd.Flags(), &authExportCfg, cfgstruct.ConfDir(defaultConfDir))
}

func cmdCreateAuth(cmd *cobra.Command, args []string) error {
	count, err := strconv.Atoi(args[0])
	if err != nil {
		return errs.New("Count couldn't be parsed: %s", args[0])
	}
	authDB, err := authCreateCfg.NewAuthDB()
	if err != nil {
		return err
	}

	var emails []string
	if len(args) > 1 {
		if authCreateCfg.EmailsPath != "" {
			return errs.New("Either use `--emails-path` or positional args, not both.")
		}
		emails = args[1:]
	} else {
		list, err := ioutil.ReadFile(authCreateCfg.EmailsPath)
		if err != nil {
			return errs.Wrap(err)
		}
		emails = strings.Split(string(list), authCreateCfg.Delimiter)
	}

	var incErrs utils.ErrorGroup
	for _, email := range emails {
		if _, err := authDB.Create(email, count); err != nil {
			incErrs.Add(err)
		}
	}
	return incErrs.Finish()
}

func cmdInfoAuth(cmd *cobra.Command, args []string) error {
	authDB, err := authInfoCfg.NewAuthDB()
	if err != nil {
		return err
	}

	var emails []string
	if len(args) > 0 {
		if authInfoCfg.EmailsPath != "" {
			return errs.New("Either use `--emails-path` or positional args, not both.")
		}
		emails = args
	} else if _, err := os.Stat(authInfoCfg.EmailsPath); err != nil {
		return errs.New("Emails path error: %s", err)
	} else {
		list, err := ioutil.ReadFile(authInfoCfg.EmailsPath)
		if err != nil {
			return errs.Wrap(err)
		}
		emails = strings.Split(string(list), authInfoCfg.Delimiter)
	}

	var emailErrs, printErrs utils.ErrorGroup
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "Email\tClaimed\tAvail.\t"); err != nil {
		return err
	}

	for _, email := range emails {
		if err := writeAuthInfo(authDB, email, w); err != nil {
			emailErrs.Add(err)
			continue
		}
	}

	if err := w.Flush(); err != nil {
		return errs.Wrap(err)
	}
	return utils.CombineErrors(emailErrs.Finish(), printErrs.Finish())
}

func writeAuthInfo(authDB *certificates.AuthorizationDB, email string, w io.Writer) error {
	auths, err := authDB.Get(email)
	if err != nil {
		return err
	}
	if len(auths) < 1 {
		return nil
	}

	claimed, open := auths.Group()
	if _, err := fmt.Fprintf(w,
		"%s\t%d\t%d\t\n",
		email,
		len(claimed),
		len(open),
	); err != nil {
		return err
	}

	if authInfoCfg.ShowTokens {
		if err := writeTokenInfo(claimed, open, w); err != nil {
			return err
		}
	}
	return nil
}

func writeTokenInfo(claimed, open certificates.Authorizations, w io.Writer) error {
	groups := map[string]certificates.Authorizations{
		"Claimed": claimed,
		"Open":    open,
	}
	for label, group := range groups {
		if _, err := fmt.Fprintf(w, "\t%s:\n", label); err != nil {
			return err
		}
		if len(group) > 0 {
			for _, auth := range group {
				if _, err := fmt.Fprintf(w, "\t\t%s\n", auth.Token.String()); err != nil {
					return err
				}
			}
		} else {
			if _, err := fmt.Fprintln(w, "\t\tnone"); err != nil {
				return err
			}
		}
	}
	return nil
}

func cmdExportAuth(cmd *cobra.Command, args []string) error {
	authDB, err := authExportCfg.NewAuthDB()
	if err != nil {
		return err
	}

	var emails []string
	if len(args) > 0 && !authExportCfg.All {
		if authExportCfg.EmailsPath != "" {
			return errs.New("Either use `--emails-path` or positional args, not both.")
		}
		emails = args
	} else if len(args) == 0 || authExportCfg.All {
		emails, err = authDB.UserIDs()
		if err != nil {
			return err
		}
	} else {
		list, err := ioutil.ReadFile(authExportCfg.EmailsPath)
		if err != nil {
			return errs.Wrap(err)
		}
		emails = strings.Split(string(list), authExportCfg.Delimiter)
	}

	var (
		emailErrs, csvErrs utils.ErrorGroup
		output             io.Writer
	)
	switch authExportCfg.Out {
	case "-":
		output = os.Stdout
	default:
		if err := os.MkdirAll(filepath.Dir(authExportCfg.Out), 0600); err != nil {
			return errs.Wrap(err)
		}
		output, err = os.OpenFile(authExportCfg.Out, os.O_CREATE, 0600)
		if err != nil {
			return errs.Wrap(err)
		}
	}
	csvWriter := csv.NewWriter(output)

	for _, email := range emails {
		if err := writeAuthExport(authDB, email, csvWriter); err != nil {
			emailErrs.Add(err)
		}
	}

	csvWriter.Flush()
	return utils.CombineErrors(emailErrs.Finish(), csvErrs.Finish())
}

func writeAuthExport(authDB *certificates.AuthorizationDB, email string, w *csv.Writer) error {
	auths, err := authDB.Get(email)
	if err != nil {
		return err
	}
	if len(auths) < 1 {
		return nil
	}

	var authErrs utils.ErrorGroup
	for _, auth := range auths {
		if err := w.Write([]string{email, auth.Token.String()}); err != nil {
			authErrs.Add(err)
		}
	}
	return authErrs.Finish()
}
