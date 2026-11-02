// Copyright 2026 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"context"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/metadata"
)

// LockNamespace excludes renames/deletions while an upload resolves or commits its
// node. Existing-file uploads share the lock; namespace changes take it exclusively.
// The lock file must survive individual node deletion and rename.
func LockNamespace(ctx context.Context, lu PathLookup, spaceID string, exclusive bool) (metadata.UnlockFunc, error) {
	root := lu.InternalSpaceRoot(spaceID)
	if root == "" {
		return nil, errtypes.NotFound(spaceID)
	}
	f := flock.New(filepath.Join(root, ".oc-mutations.mlock"), flock.SetPermissions(0600))
	var ok bool
	var err error
	if exclusive {
		ok, err = f.TryLockContext(ctx, 10*time.Millisecond)
	} else {
		ok, err = f.TryRLockContext(ctx, 10*time.Millisecond)
	}
	if err != nil || !ok {
		_ = f.Close()
		if err == nil {
			err = ctx.Err()
		}
		return nil, err
	}
	return f.Close, nil
}
