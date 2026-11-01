package storageprovider

import (
	"context"
	"net/url"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/stretchr/testify/require"
)

type conditionalUploadStorage struct {
	storage.FS
	info     *provider.ResourceInfo
	metadata map[string]string
	err      error
}

func (s *conditionalUploadStorage) GetMD(context.Context, *provider.Reference, []string, []string) (*provider.ResourceInfo, error) {
	if s.info == nil {
		return nil, errtypes.NotFound("missing")
	}
	return s.info, nil
}

func (s *conditionalUploadStorage) InitiateUpload(_ context.Context, _ *provider.Reference, _ int64, metadata map[string]string) (map[string]string, error) {
	s.metadata = metadata
	return map[string]string{"tus": "upload"}, s.err
}

func TestUploadConditions(t *testing.T) {
	for _, tt := range []struct {
		name       string
		exists     bool
		conditions preconditions.Conditions
		code       rpc.Code
	}{
		{"missing match", false, preconditions.Conditions{IfMatch: `"a"`}, rpc.Code_CODE_ABORTED},
		{"missing wildcard", false, preconditions.Conditions{IfMatch: "*"}, rpc.Code_CODE_ABORTED},
		{"existing wildcard", true, preconditions.Conditions{IfMatch: "*"}, rpc.Code_CODE_OK},
		{"exclusive existing", true, preconditions.Conditions{IfNoneMatch: "*"}, rpc.Code_CODE_ABORTED},
		{"exclusive missing", false, preconditions.Conditions{IfNoneMatch: "*"}, rpc.Code_CODE_OK},
		{"both", true, preconditions.Conditions{IfMatch: `"a"`, IfNoneMatch: `"b"`}, rpc.Code_CODE_OK},
		{"weak none", true, preconditions.Conditions{IfNoneMatch: `W/"a"`}, rpc.Code_CODE_ABORTED},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := &conditionalUploadStorage{}
			if tt.exists {
				fs.info = &provider.ResourceInfo{Etag: `"a"`}
			}
			s := &Service{Storage: fs, conf: &config{}, dataServerURL: &url.URL{Scheme: "http", Host: "localhost"}}
			res, err := s.InitiateFileUpload(context.Background(), &provider.InitiateFileUploadRequest{Ref: &provider.Reference{Path: "file"}, Opaque: tt.conditions.Opaque(nil)})
			require.NoError(t, err)
			require.Equal(t, tt.code, res.Status.Code)
			if tt.code == rpc.Code_CODE_OK {
				require.Equal(t, tt.conditions, preconditions.FromMetadata(fs.metadata))
			} else {
				require.Nil(t, fs.metadata)
			}
		})
	}
}

func TestUploadFinalizationPreconditionStatus(t *testing.T) {
	fs := &conditionalUploadStorage{err: errtypes.Aborted("changed during upload")}
	s := &Service{Storage: fs, conf: &config{}}
	res, err := s.InitiateFileUpload(context.Background(), &provider.InitiateFileUploadRequest{Ref: &provider.Reference{Path: "file"}})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_ABORTED, res.Status.Code)
}
