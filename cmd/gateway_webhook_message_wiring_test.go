package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Non-nil store stubs: an unauthenticated request is rejected by the auth
// middleware before any store method is called.
type stubWebhookStore struct{ store.WebhookStore }
type stubWebhookCallStore struct{ store.WebhookCallStore }
type stubChannelInstanceStore struct{ store.ChannelInstanceStore }

// Regression: wireHTTPHandlersOnServer runs before runGateway creates channelMgr,
// so the message handler used to be silently skipped and POST /v1/webhooks/message
// returned 404. Mirror the gateway order: wire HTTP handlers, assign channelMgr,
// run the deferred webhook-message wiring, then build the mux.
func TestWebhookMessageRouteMountedAfterChannelMgr(t *testing.T) {
	t.Setenv("GOCLAW_ENCRYPTION_KEY", "test-encryption-key-0123456789abcdef")
	t.Setenv("GOCLAW_DATA_DIR", t.TempDir()) // keep package-runtime state out of the real data dir

	cfg := &config.Config{}
	msgBus := bus.New()
	deps := &gatewayDeps{
		cfg:    cfg,
		server: gateway.NewServer(cfg, msgBus, nil, nil),
		msgBus: msgBus,
		pgStores: &store.Stores{
			Webhooks:         stubWebhookStore{},
			WebhookCalls:     stubWebhookCallStore{},
			ChannelInstances: stubChannelInstanceStore{},
		},
	}

	deps.wireHTTPHandlersOnServer(httpHandlers{}, nil, nil, nil, nil)

	// runGateway: channelMgr is created after wireHTTPHandlersOnServer.
	deps.channelMgr = channels.NewManager(msgBus)
	deps.server.SetChannelManager(deps.channelMgr)
	if deps.wireWebhookMessage != nil {
		deps.wireWebhookMessage()
	}

	mux := deps.server.BuildMux()
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/message", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /v1/webhooks/message without auth = %d, want 401 (404 means route not mounted)", rec.Code)
	}
}
