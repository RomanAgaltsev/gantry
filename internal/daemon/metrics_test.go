package daemon

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func TestPrometheusObserver_RecordsReconcileAndDrift(t *testing.T) {
	obs, handler := NewPrometheusObserver("test")

	obs.ReconcileDone("test", engine.SyncResult{Deployed: true}, nil, 120*time.Millisecond)

	obs.ReconcileDone("test", engine.SyncResult{VerifyFailed: true, AutoRolledBack: true, RolledBackTo: "abc"},

		errors.New("verify failed"), 90*time.Millisecond)

	obs.ReconcileDone("test", engine.SyncResult{}, nil, 10*time.Millisecond) // no change

	obs.DriftObserved("test", 3600)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()

	require.Contains(t, body, `gantry_reconcile_total{env="test",result="deployed"} 1`)

	require.Contains(t, body, `gantry_reconcile_total{env="test",result="failed"} 1`)

	require.Contains(t, body, `gantry_reconcile_total{env="test",result="nochange"} 1`)

	require.Contains(t, body, `gantry_deploys_total{env="test"} 1`)

	require.Contains(t, body, `gantry_verify_failures_total{env="test"} 1`)

	require.Contains(t, body, `gantry_rollbacks_total{env="test",kind="auto"} 1`)

	require.Contains(t, body, `gantry_drift_age_seconds{env="test"} 3600`)

	require.Contains(t, body, `gantry_reconcile_duration_seconds`)

	require.Contains(t, body, `gantry_build_info{version="test"} 1`)
}
