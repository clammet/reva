package ocdav

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/status"
	"github.com/opencloud-eu/reva/v2/pkg/storage"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/preconditions"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	"github.com/opencloud-eu/reva/v2/tests/cs3mocks/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestConditionalMoveDoesNotPredeleteDestination(t *testing.T) {
	ctx := context.Background()
	root := &provider.ResourceId{SpaceId: "space", OpaqueId: "space"}
	source := &provider.Reference{ResourceId: root, Path: "source"}
	target := &provider.Reference{ResourceId: root, Path: "target"}
	client := &mocks.GatewayAPIClient{}
	for _, name := range []string{"source", "target"} {
		client.On("Stat", mock.Anything, mock.MatchedBy(func(req *provider.StatRequest) bool { return req.Ref.Path == name })).Return(&provider.StatResponse{
			Status: status.NewOK(ctx), Info: &provider.ResourceInfo{Id: &provider.ResourceId{SpaceId: "space", OpaqueId: name}, Etag: `"current"`},
		}, nil).Once()
	}
	client.On("Move", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(1).(*provider.MoveRequest)
		require.Equal(t, preconditions.Conditions{IfMatch: `"current"`}, preconditions.FromOpaque(req.Opaque))
		require.Equal(t, "true", utils.ReadPlainFromOpaque(req.Opaque, storage.MoveOverwriteKey))
	}).Return(&provider.MoveResponse{Status: status.NewAborted(ctx, errors.New("changed after Stat"), "changed")}, nil).Once()
	s := &svc{gatewaySelector: gatewayStub{client}}
	r := httptest.NewRequest("MOVE", "/source", nil)
	r.Header.Set("If-Match", `"current"`)
	w := httptest.NewRecorder()
	s.handleMove(ctx, w, r, source, target, zerolog.Nop())
	require.Equal(t, http.StatusPreconditionFailed, w.Code)
	client.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestConditionalDeleteFailure(t *testing.T) {
	ctx := context.Background()
	client := &mocks.GatewayAPIClient{}
	client.On("Delete", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		require.Equal(t, preconditions.Conditions{IfMatch: `"stale"`}, preconditions.FromOpaque(args.Get(1).(*provider.DeleteRequest).Opaque))
	}).Return(&provider.DeleteResponse{Status: status.NewAborted(ctx, errors.New("stale"), "stale")}, nil).Once()
	s := &svc{gatewaySelector: gatewayStub{client}}
	r := httptest.NewRequest(http.MethodDelete, "/source", nil)
	r.Header.Set("If-Match", `"stale"`)
	code, _ := s.handleDelete(ctx, httptest.NewRecorder(), r, &provider.Reference{Path: "source"})
	require.Equal(t, http.StatusPreconditionFailed, code)
	client.AssertExpectations(t)
}
