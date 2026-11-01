package ocdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/internal/http/services/owncloud/ocdav/config"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/status"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/opencloud-eu/reva/v2/tests/cs3mocks/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestConditionalEmptyUploads(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodPost} {
		for _, c := range []preconditions.Conditions{{IfMatch: `"stale"`}, {IfMatch: "*"}, {IfNoneMatch: "*"}, {IfMatch: `"a"`, IfNoneMatch: `"b"`}} {
			t.Run(method+c.IfMatch+c.IfNoneMatch, func(t *testing.T) {
				ctx := context.Background()
				client := &mocks.GatewayAPIClient{}
				client.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: status.NewOK(ctx), Info: &provider.ResourceInfo{Type: provider.ResourceType_RESOURCE_TYPE_FILE, Etag: c.IfMatch}}, nil).Maybe()
				client.On("InitiateFileUpload", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					req := args.Get(1).(*provider.InitiateFileUploadRequest)
					require.Equal(t, c, preconditions.FromOpaque(req.Opaque))
				}).Return(&gateway.InitiateFileUploadResponse{Status: status.NewAborted(ctx, nil, "precondition failed")}, nil).Maybe()
				cfg := &config.Config{}
				cfg.Init()
				s := &svc{gatewaySelector: gatewayStub{client}, nameValidators: ValidatorsFromConfig(cfg)}
				r := httptest.NewRequest(method, "/file", nil)
				r.Header.Set("Content-Length", "0")
				r.Header.Set("Upload-Length", "0")
				r.Header.Set("Tus-Resumable", "1.0.0")
				if c.IfMatch != "" {
					r.Header.Set("If-Match", c.IfMatch)
				}
				if c.IfNoneMatch != "" {
					r.Header.Set("If-None-Match", c.IfNoneMatch)
				}
				w := httptest.NewRecorder()
				ref := &provider.Reference{Path: "file"}
				if method == http.MethodPut {
					s.handlePut(ctx, w, r, ref, zerolog.Nop())
				} else {
					s.handleTusPost(ctx, w, r, map[string]string{"filename": "file"}, ref, zerolog.Nop())
				}
				require.Equal(t, http.StatusPreconditionFailed, w.Code)
				client.AssertNotCalled(t, "TouchFile", mock.Anything, mock.Anything)
			})
		}
	}
}
