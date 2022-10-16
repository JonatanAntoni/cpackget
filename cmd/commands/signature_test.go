/* SPDX-License-Identifier: Apache-2.0 */
/* Copyright Contributors to the cpackget project. */

package commands_test

import (
	"errors"
	"os"
	"testing"

	errs "github.com/open-cmsis-pack/cpackget/cmd/errors"
)

// TODO: Compare actual ErrFileNotFound output
var signatureCreateCmdTests = []TestCase{
	{
		name:        "test different number of parameters",
		args:        []string{"signature-create"},
		expectedErr: errors.New("accepts 1 arg(s), received 0"),
	},
	{
		name:        "test help command",
		args:        []string{"help", "signature-create"},
		expectedErr: nil,
	},
	{
		name:        "test wrong usage of passphrase flag",
		args:        []string{"signature-create", "Pack.1.2.3.pack", "--passphrase", "foo"},
		expectedErr: errs.ErrIncorrectCmdArgs,
		setUpFunc: func(t *TestCase) {
			x, _ := os.Create("Pack.1.2.3.pack")
			x.Close()
		},
		tearDownFunc: func() {
			os.Remove("Pack.1.2.3.pack")
		},
	},
	// TODO: Investigate why cobra does not clear up used flags
	// https://github.com/spf13/cobra/issues/1419
	// Using -k here as it seems to keep the --passphrase from
	// the second test..
	{
		name:        "test creating signature of unexisting pack",
		args:        []string{"signature-create", "DoesNotExist.Pack.1.2.3.pack", "-k", "foo"},
		expectedErr: errs.ErrFileNotFound,
	},
}

var signatureVerifyCmdTests = []TestCase{
	{
		name:        "test different number of parameters",
		args:        []string{"signature-verify"},
		expectedErr: errors.New("accepts 2 arg(s), received 0"),
	},
	{
		name:        "test help command",
		args:        []string{"help", "signature-verify"},
		expectedErr: nil,
	},
	{
		name:        "test signature of unexisting .checksum",
		args:        []string{"signature-verify", "Pack.1.2.3.sha256.checksum", "signature_curve25519.key"},
		expectedErr: errs.ErrFileNotFound,
		setUpFunc: func(t *TestCase) {
			x, _ := os.Create("signature_curve25519.key")
			y, _ := os.Create("Pack.1.2.3.sha256.signature")
			x.Close()
			y.Close()
		},
		tearDownFunc: func() {
			os.Remove("signature_curve25519.key")
			os.Remove("Pack.1.2.3.sha256.signature")
		},
	},
	{
		name:        "test verifying unexisting .signature",
		args:        []string{"signature-verify", "Pack.1.2.3.sha256.checksum", "signature_curve25519.key"},
		expectedErr: errs.ErrFileNotFound,
		setUpFunc: func(t *TestCase) {
			x, _ := os.Create("signature_curve25519.key")
			y, _ := os.Create("Pack.1.2.3.sha256.checksum")
			x.Close()
			y.Close()
		},
		tearDownFunc: func() {
			os.Remove("signature_curve25519.key")
			os.Remove("Pack.1.2.3.sha256.checksum")
		},
	},
}

func TestSignatureCreateCmd(t *testing.T) {
	runTests(t, signatureCreateCmdTests)
}

func TestSignatureVerifyCmd(t *testing.T) {
	runTests(t, signatureVerifyCmdTests)
}