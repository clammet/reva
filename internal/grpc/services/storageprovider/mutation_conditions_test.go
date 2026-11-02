package storageprovider

import (
	"context"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	"github.com/stretchr/testify/require"
)

type conditionalMutationStorage struct {
	storage.FS
	conditions preconditions.Conditions
	overwrite  bool
}

func (s *conditionalMutationStorage) GetMD(context.Context, *provider.Reference, []string, []string) (*provider.ResourceInfo, error) {
	return &provider.ResourceInfo{Id: &provider.ResourceId{OpaqueId: "file"}}, nil
}

func (s *conditionalMutationStorage) DeleteConditional(_ context.Context, _ *provider.Reference, c preconditions.Conditions) error {
	s.conditions = c
	return errtypes.Aborted("stale")
}

func (s *conditionalMutationStorage) MoveConditional(_ context.Context, _, _ *provider.Reference, options storage.MoveOptions) (*storage.MoveResult, error) {
	s.conditions, s.overwrite = options.Conditions, options.Overwrite
	return nil, errtypes.Aborted("stale")
}

func TestStorageMutationConditions(t *testing.T) {
	for _, method := range []string{"MOVE", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			fs := &conditionalMutationStorage{}
			s := &Service{Storage: fs}
			c := preconditions.Conditions{IfMatch: `"current"`, IfNoneMatch: `"other"`}
			o := utils.AppendPlainToOpaque(c.Opaque(nil), storage.MoveOverwriteKey, "true")
			ref := &provider.Reference{Path: "file"}
			if method == "MOVE" {
				r, err := s.Move(context.Background(), &provider.MoveRequest{Source: ref, Destination: ref, Opaque: o})
				require.NoError(t, err)
				require.Equal(t, rpc.Code_CODE_ABORTED, r.Status.Code)
				require.True(t, fs.overwrite)
			} else {
				r, err := s.Delete(context.Background(), &provider.DeleteRequest{Ref: ref, Opaque: o})
				require.NoError(t, err)
				require.Equal(t, rpc.Code_CODE_ABORTED, r.Status.Code)
			}
			require.Equal(t, c, fs.conditions)
		})
	}
}

func TestConditionalMoveRejectsUnsupportedStorage(t *testing.T) {
	s := &Service{Storage: moveStorage{}}
	r, err := s.Move(context.Background(), &provider.MoveRequest{Opaque: (preconditions.Conditions{IfMatch: "*"}).Opaque(nil)})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_UNIMPLEMENTED, r.Status.Code)
}
