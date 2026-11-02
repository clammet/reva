package posix_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/stretchr/testify/require"
)

func TestConditionalPosixMutations(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		for _, method := range []string{"MOVE", "DELETE"} {
			t.Run(backend+method, func(t *testing.T) {
				e := conditionalEnv(t, backend, false)
				original, err := simpleUpload(t, e, "source", "original", map[string]string{"mtime": "1700000000"})
				require.NoError(t, err)
				current, err := simpleUpload(t, e, "source", "updated", map[string]string{"mtime": "1700000000"})
				require.NoError(t, err)
				_, err = simpleUpload(t, e, "target", "occupied", nil)
				require.NoError(t, err)
				stale := preconditions.Conditions{IfMatch: original.Etag}
				if method == "MOVE" {
					_, err = e.Fs.MoveConditional(e.Ctx, fileRef(e, "source"), fileRef(e, "target"), storage.MoveOptions{Conditions: stale, Overwrite: true})
				} else {
					err = e.Fs.DeleteConditional(e.Ctx, fileRef(e, "source"), stale)
				}
				require.ErrorAs(t, err, new(errtypes.IsAborted))
				require.Equal(t, "updated", contentAt(t, e, "source"))
				require.Equal(t, "occupied", contentAt(t, e, "target"))
				valid := preconditions.Conditions{IfMatch: current.Etag}
				if method == "MOVE" {
					_, err = e.Fs.MoveConditional(e.Ctx, fileRef(e, "source"), fileRef(e, "target"), storage.MoveOptions{Conditions: valid, Overwrite: true})
					require.NoError(t, err)
					require.Equal(t, "updated", contentAt(t, e, "target"))
				} else {
					require.NoError(t, e.Fs.DeleteConditional(e.Ctx, fileRef(e, "source"), valid))
				}
				n, err := e.Lookup.NodeFromResource(e.Ctx, fileRef(e, "source"))
				require.NoError(t, err)
				require.False(t, n.Exists)
			})
		}
	}
}

func TestMutationWaitsForUploadCommit(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		for _, method := range []string{"MOVE", "DELETE"} {
			t.Run(backend+method, func(t *testing.T) {
				e := conditionalEnv(t, backend, false)
				original, err := simpleUpload(t, e, "source", "original", nil)
				require.NoError(t, err)
				// An upload owns the shared namespace lock while it publishes a revision.
				unlock, err := node.LockNamespace(e.Ctx, e.Lookup, e.SpaceRootRes.SpaceId, false)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(e.Ctx, 30*time.Millisecond)
				defer cancel()
				c := preconditions.Conditions{IfMatch: original.Etag}
				if method == "MOVE" {
					_, err = e.Fs.MoveConditional(ctx, fileRef(e, "source"), fileRef(e, "target"), storage.MoveOptions{Conditions: c})
				} else {
					err = e.Fs.DeleteConditional(ctx, fileRef(e, "source"), c)
				}
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.NoError(t, unlock())
				require.Equal(t, "original", contentAt(t, e, "source"))
				// Once that upload commits, the old validator must fail rather than mutate it.
				_, err = simpleUpload(t, e, "source", "updated", nil)
				require.NoError(t, err)
				if method == "MOVE" {
					_, err = e.Fs.MoveConditional(e.Ctx, fileRef(e, "source"), fileRef(e, "target"), storage.MoveOptions{Conditions: c})
				} else {
					err = e.Fs.DeleteConditional(e.Ctx, fileRef(e, "source"), c)
				}
				require.ErrorAs(t, err, new(errtypes.IsAborted))
				require.Equal(t, "updated", contentAt(t, e, "source"))
			})
		}
	}
}

func TestConditionalMoveWithoutOverwrite(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		t.Run(fmt.Sprint(backend), func(t *testing.T) {
			e := conditionalEnv(t, backend, false)
			initial, err := simpleUpload(t, e, "source", "source", nil)
			require.NoError(t, err)
			_, err = simpleUpload(t, e, "target", "target", nil)
			require.NoError(t, err)
			_, err = e.Fs.MoveConditional(e.Ctx, fileRef(e, "source"), fileRef(e, "target"), storage.MoveOptions{Conditions: preconditions.Conditions{IfMatch: initial.Etag}})
			require.ErrorAs(t, err, new(errtypes.IsAlreadyExists))
			require.Equal(t, "source", contentAt(t, e, "source"))
			require.Equal(t, "target", contentAt(t, e, "target"))
		})
	}
}
