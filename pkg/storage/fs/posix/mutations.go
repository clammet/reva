// Copyright 2026 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
)

var _ storage.ConditionalFS = (*posixFS)(nil)

func (fs *posixFS) DeleteConditional(ctx context.Context, ref *provider.Reference, c preconditions.Conditions) error {
	return fs.FS.(storage.ConditionalFS).DeleteConditional(ctx, ref, c)
}

func (fs *posixFS) MoveConditional(ctx context.Context, source, target *provider.Reference, options storage.MoveOptions) (*storage.MoveResult, error) {
	return fs.FS.(storage.ConditionalFS).MoveConditional(ctx, source, target, options)
}
