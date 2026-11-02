package middleware

import (
	"context"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/stretchr/testify/require"
)

type scopeKey struct{}
type scopedMutations struct {
	storage.FS
	t *testing.T
}

func (s scopedMutations) DeleteConditional(ctx context.Context, _ *provider.Reference, c preconditions.Conditions) error {
	require.Equal(s.t, "scoped", ctx.Value(scopeKey{}))
	require.Equal(s.t, "*", c.IfMatch)
	return errtypes.Aborted("stale")
}

func (s scopedMutations) MoveConditional(ctx context.Context, _, _ *provider.Reference, options storage.MoveOptions) (*storage.MoveResult, error) {
	return nil, s.DeleteConditional(ctx, nil, options.Conditions)
}

func TestConditionalMutationRestoresScopeOnFailure(t *testing.T) {
	for _, method := range []string{"Move", "Delete"} {
		t.Run(method, func(t *testing.T) {
			entered, restored := false, false
			fs := NewFS(scopedMutations{t: t}, func(name string, ctx context.Context, spaceID string) (context.Context, UnHook, error) {
				require.Equal(t, method, name)
				require.Equal(t, "space", spaceID)
				entered = true
				return context.WithValue(ctx, scopeKey{}, "scoped"), func() error { restored = true; return nil }, nil
			})
			ref := &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "space"}}
			c := preconditions.Conditions{IfMatch: "*"}
			var err error
			if method == "Move" {
				_, err = fs.MoveConditional(context.Background(), ref, ref, storage.MoveOptions{Conditions: c})
			} else {
				err = fs.DeleteConditional(context.Background(), ref, c)
			}
			require.ErrorAs(t, err, new(errtypes.IsAborted))
			require.True(t, entered)
			require.True(t, restored)
		})
	}
}
