package posix_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/blobstore"
	helpers "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/testhelpers"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	tusd "github.com/tus/tusd/v2/pkg/handler"
)

func conditionalEnv(t *testing.T, backend string, disableVersioning bool) *helpers.TestEnv {
	t.Helper()
	e, err := helpers.NewTestEnv(map[string]interface{}{"metadata_backend": backend, "disable_versioning": disableVersioning})
	require.NoError(t, err)
	t.Cleanup(e.Cleanup)
	e.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything).Return(&provider.ResourcePermissions{
		Stat: true, InitiateFileUpload: true, Delete: true, Move: true, CreateContainer: true, ListFileVersions: true,
	}, nil)
	bs, err := blobstore.New(e.Root)
	require.NoError(t, err)
	e.Blobstore.On("Upload", mock.Anything, mock.Anything, mock.Anything).Return(func(n *node.Node, source, target string) error {
		return bs.Upload(n, source, target)
	})
	return e
}

func fileRef(e *helpers.TestEnv, name string) *provider.Reference {
	return &provider.Reference{ResourceId: e.SpaceRootRes, Path: name}
}

func simpleUpload(t *testing.T, e *helpers.TestEnv, name, content string, metadata map[string]string) (*provider.ResourceInfo, error) {
	t.Helper()
	ids, err := e.Fs.InitiateUpload(e.Ctx, fileRef(e, name), int64(len(content)), metadata)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return e.Fs.GetMD(e.Ctx, fileRef(e, name), nil, nil)
	}
	return e.Fs.Upload(e.Ctx, storage.UploadRequest{Ref: &provider.Reference{Path: ids["simple"]}, Body: io.NopCloser(strings.NewReader(content)), Length: int64(len(content))}, nil)
}

func contentAt(t *testing.T, e *helpers.TestEnv, name string) string {
	t.Helper()
	n, err := e.Lookup.NodeFromResource(e.Ctx, fileRef(e, name))
	require.NoError(t, err)
	b, err := os.ReadFile(n.InternalPath())
	require.NoError(t, err)
	return string(b)
}

func tusHandler(t *testing.T, e *helpers.TestEnv) http.Handler {
	t.Helper()
	c := tusd.NewStoreComposer()
	e.Fs.UseIn(c)
	h, err := tusd.NewHandler(tusd.Config{BasePath: "/tus/", StoreComposer: c})
	require.NoError(t, err)
	return http.StripPrefix("/tus/", h)
}

func patchUpload(h http.Handler, id, content string, offset int) int {
	r := httptest.NewRequest(http.MethodPatch, "/tus/"+id, strings.NewReader(content))
	r.Header.Set("Tus-Resumable", "1.0.0")
	r.Header.Set("Content-Type", "application/offset+octet-stream")
	r.Header.Set("Upload-Offset", fmt.Sprint(offset))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestConditionalPosixUploads(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		t.Run(backend, func(t *testing.T) {
			e := conditionalEnv(t, backend, false)
			for i, tt := range []struct {
				name, match, none string
				exists, pass      bool
			}{
				{"missing match", `"stale"`, "", false, false},
				{"missing wildcard", "*", "", false, false},
				{"exclusive existing", "", "*", true, false},
				{"exclusive missing", "", "*", false, true},
				{"wildcard existing", "*", "", true, true},
				{"stale existing", `"stale"`, "", true, false},
			} {
				for _, body := range []string{"", "replacement"} {
					t.Run(tt.name+fmt.Sprint(len(body)), func(t *testing.T) {
						name := fmt.Sprintf("conditional-%d-%d", i, len(body))
						if tt.exists {
							_, err := simpleUpload(t, e, name, "original", nil)
							require.NoError(t, err)
						}
						_, err := simpleUpload(t, e, name, body, (preconditions.Conditions{IfMatch: tt.match, IfNoneMatch: tt.none}).Metadata())
						if tt.pass {
							require.NoError(t, err)
							require.Equal(t, body, contentAt(t, e, name))
						} else {
							require.Error(t, err)
							if tt.exists {
								require.Equal(t, "original", contentAt(t, e, name))
							} else {
								_, err := os.Stat(e.Lookup.InternalSpaceRoot(e.SpaceRootRes.SpaceId) + "/" + name)
								require.True(t, os.IsNotExist(err))
							}
						}
					})
				}
			}
		})
	}
}

func TestConditionalTusFinalization(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		for _, noVersions := range []bool{false, true} {
			for _, concurrent := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/noVersions=%v/concurrent=%v", backend, noVersions, concurrent), func(t *testing.T) {
					e := conditionalEnv(t, backend, noVersions)
					initial, err := simpleUpload(t, e, "document", "0000000000\n", map[string]string{"mtime": "1700000000"})
					require.NoError(t, err)
					md := map[string]string{"mtime": "1700000000", "if-match": initial.Etag}
					a, err := e.Fs.InitiateUpload(e.Ctx, fileRef(e, "document"), 11, md)
					require.NoError(t, err)
					b, err := e.Fs.InitiateUpload(e.Ctx, fileRef(e, "document"), 11, md)
					require.NoError(t, err)
					h := tusHandler(t, e)
					require.Equal(t, 204, patchUpload(h, a["tus"], "AAAAA", 0))
					require.Equal(t, 204, patchUpload(h, b["tus"], "BBBBB", 0))
					codes := [2]int{}
					if concurrent {
						start := make(chan struct{})
						var wg sync.WaitGroup
						for i, id := range []string{a["tus"], b["tus"]} {
							wg.Add(1)
							go func() { defer wg.Done(); <-start; codes[i] = patchUpload(h, id, []string{"AAAAA\n", "BBBBB\n"}[i], 5) }()
						}
						close(start)
						wg.Wait()
					} else {
						codes[0] = patchUpload(h, a["tus"], "AAAAA\n", 5)
						codes[1] = patchUpload(h, b["tus"], "BBBBB\n", 5)
					}
					require.ElementsMatch(t, []int{204, 412}, codes[:])
					winner := "AAAAAAAAAA\n"
					if codes[1] == 204 {
						winner = "BBBBBBBBBB\n"
					}
					require.Equal(t, winner, contentAt(t, e, "document"))
					current, err := e.Fs.GetMD(e.Ctx, fileRef(e, "document"), nil, nil)
					require.NoError(t, err)
					require.NotEqual(t, initial.Etag, current.Etag)
				})
			}
		}
	}
}

func TestExclusiveTusCreation(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		t.Run(backend, func(t *testing.T) {
			e := conditionalEnv(t, backend, false)
			a, err := e.Fs.InitiateUpload(e.Ctx, fileRef(e, "document"), 1, map[string]string{"if-none-match": "*"})
			require.NoError(t, err)
			b, err := e.Fs.InitiateUpload(e.Ctx, fileRef(e, "document"), 1, map[string]string{"if-none-match": "*"})
			require.NoError(t, err)
			h := tusHandler(t, e)
			require.Equal(t, 204, patchUpload(h, a["tus"], "A", 0))
			require.Equal(t, 412, patchUpload(h, b["tus"], "B", 0))
			require.Equal(t, "A", contentAt(t, e, "document"))
		})
	}
}

func TestTusTargetReplacedDuringUpload(t *testing.T) {
	for _, backend := range []string{"hybrid", "xattrs"} {
		for _, recreate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/recreate=%v", backend, recreate), func(t *testing.T) {
				e := conditionalEnv(t, backend, false)
				initial, err := simpleUpload(t, e, "document", "original", nil)
				require.NoError(t, err)
				ids, err := e.Fs.InitiateUpload(e.Ctx, fileRef(e, "document"), 1, map[string]string{"if-match": initial.Etag})
				require.NoError(t, err)
				require.NoError(t, e.Fs.Delete(e.Ctx, fileRef(e, "document")))
				if recreate {
					_, err := simpleUpload(t, e, "document", "replacement", nil)
					require.NoError(t, err)
				}
				require.Equal(t, 412, patchUpload(tusHandler(t, e), ids["tus"], "X", 0))
				if recreate {
					require.Equal(t, "replacement", contentAt(t, e, "document"))
				} else {
					n, err := e.Lookup.NodeFromResource(e.Ctx, fileRef(e, "document"))
					require.NoError(t, err)
					require.False(t, n.Exists)
				}
			})
		}
	}
}
