// Copyright 2026 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
)

func (f *FS) mutationContext(ctx context.Context, method, spaceID string) (context.Context, UnHook, error) {
	var unhooks []UnHook
	undo := func() error {
		var first error
		for i := len(unhooks) - 1; i >= 0; i-- {
			if err := unhooks[i](); first == nil {
				first = err
			}
		}
		return first
	}
	for _, hook := range f.hooks {
		next, unhook, err := hook(method, ctx, spaceID)
		if err != nil {
			_ = undo()
			return nil, nil, err
		}
		ctx = next
		if unhook != nil {
			unhooks = append(unhooks, unhook)
		}
	}
	return ctx, undo, nil
}

func (f *FS) DeleteConditional(ctx context.Context, ref *provider.Reference, c preconditions.Conditions) (err error) {
	next, ok := f.next.(storage.ConditionalFS)
	if !ok {
		return errtypes.NotSupported("conditional delete is not supported by this storage")
	}
	ctx, undo, err := f.mutationContext(ctx, "Delete", ref.GetResourceId().GetSpaceId())
	if err != nil {
		return err
	}
	defer func() {
		if e := undo(); err == nil {
			err = e
		}
	}()
	return next.DeleteConditional(ctx, ref, c)
}

func (f *FS) MoveConditional(ctx context.Context, source, target *provider.Reference, options storage.MoveOptions) (result *storage.MoveResult, err error) {
	next, ok := f.next.(storage.ConditionalFS)
	if !ok {
		return nil, errtypes.NotSupported("conditional move is not supported by this storage")
	}
	ctx, undo, err := f.mutationContext(ctx, "Move", source.GetResourceId().GetSpaceId())
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := undo(); err == nil {
			err = e
		}
	}()
	return next.MoveConditional(ctx, source, target, options)
}
