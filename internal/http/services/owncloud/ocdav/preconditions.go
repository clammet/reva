// Copyright 2026 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

package ocdav

import (
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
)

func setUploadConditions(req *provider.InitiateFileUploadRequest, c preconditions.Conditions) {
	if c.Empty() {
		return
	}
	req.Opaque = c.Opaque(req.Opaque)
	if c.IfMatch != "" {
		req.Options = &provider.InitiateFileUploadRequest_IfMatch{IfMatch: c.IfMatch}
	} else if c.IfNoneMatch == "*" {
		req.Options = &provider.InitiateFileUploadRequest_IfNotExist{IfNotExist: true}
	}
}
