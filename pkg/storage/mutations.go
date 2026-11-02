// Copyright 2026 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
)

// MoveOverwriteKey carries the overwrite policy with conditional CS3 Move requests.
const MoveOverwriteKey = "overwrite"

type MoveOptions struct {
	Conditions preconditions.Conditions
	Overwrite  bool
}

// ConditionalFS evaluates conditions atomically with the requested mutation.
type ConditionalFS interface {
	DeleteConditional(context.Context, *provider.Reference, preconditions.Conditions) error
	MoveConditional(context.Context, *provider.Reference, *provider.Reference, MoveOptions) (*MoveResult, error)
}
