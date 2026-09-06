package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminUsageRepoCapture struct {
	service.UsageLogRepository
	listParams   pagination.PaginationParams
	listFilters  usagestats.UsageLogFilters
	statsFilters usagestats.UsageLogFilters
	statsCalls   int
	listLogs     []service.UsageLog
	stats        *usagestats.UsageStats
}

func (s *adminUsageRepoCapture) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	s.listParams = params
	s.listFilters = filters
	return s.listLogs, &pagination.PaginationResult{
		Total:    int64(len(s.listLogs)),
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
	}, nil
}

func (s *adminUsageRepoCapture) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	s.statsCalls++
	s.statsFilters = filters
	if s.stats != nil {
		return s.stats, nil
	}
	return &usagestats.UsageStats{}, nil
}

func newAdminUsageRequestTypeTestRouter(repo *adminUsageRepoCapture) *gin.Engine {
	return newAdminUsageRequestTypeTestRouterWithRole(repo, "")
}

func newAdminUsageRequestTypeTestRouterWithRole(repo *adminUsageRepoCapture, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil, nil, nil)
	router := gin.New()
	if role != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUserRole), role)
			c.Next()
		})
	}
	router.GET("/admin/usage", handler.List)
	router.GET("/admin/usage/stats", handler.Stats)
	return router
}

type adminUsageListResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []struct {
			InputTokens    int     `json:"input_tokens"`
			OutputTokens   int     `json:"output_tokens"`
			TotalCost      float64 `json:"total_cost"`
			ActualCost     float64 `json:"actual_cost"`
			RateMultiplier float64 `json:"rate_multiplier"`
		} `json:"items"`
	} `json:"data"`
}

type adminUsageStatsResponse struct {
	Code int `json:"code"`
	Data struct {
		TotalAccountCost *float64 `json:"total_account_cost,omitempty"`
	} `json:"data"`
}

func TestAdminUsageListRequestTypePriority(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?request_type=ws_v2&stream=false", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.listFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.listFilters.RequestType)
	require.Nil(t, repo.listFilters.Stream)
}

func TestAdminUsageListUsesRequestedModelForDisplayModelFilter(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?model=grok-imagine-video-1.5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "grok-imagine-video-1.5", repo.listFilters.Model)
	require.Equal(t, usagestats.ModelSourceRequested, repo.listFilters.ModelFilterSource)
}

func TestAdminUsageListInvalidRequestType(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?request_type=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageListInvalidStream(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageListNativeCompactionFilter(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?request_type=stream&native_compaction_v2=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.listFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeStream), *repo.listFilters.RequestType)
	require.NotNil(t, repo.listFilters.NativeCompactionV2)
	require.True(t, *repo.listFilters.NativeCompactionV2)
}

func TestAdminUsageListInvalidNativeCompactionFilter(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?native_compaction_v2=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageListExactTotalTrue(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?exact_total=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.listFilters.ExactTotal)
}

func TestAdminUsageListRequestIDFilter(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?request_id=req-0123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "req-0123", repo.listFilters.RequestID)
}

func TestAdminUsageListInvalidExactTotal(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage?exact_total=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageStatsRequestTypePriority(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?request_type=stream&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.statsFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeStream), *repo.statsFilters.RequestType)
	require.Nil(t, repo.statsFilters.Stream)
}

func TestAdminUsageStatsNativeCompactionFilter(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?native_compaction_v2=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.statsFilters.NativeCompactionV2)
	require.True(t, *repo.statsFilters.NativeCompactionV2)
}

func TestAdminUsageStatsUsesRequestedModelForDisplayModelFilter(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?model=grok-imagine-video-1.5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "grok-imagine-video-1.5", repo.statsFilters.Model)
	require.Equal(t, usagestats.ModelSourceRequested, repo.statsFilters.ModelFilterSource)
}

func TestAdminUsageStatsInvalidRequestType(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?request_type=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageStatsInvalidStream(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?stream=oops", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminUsageStatsRequestsPresentationViewForAdmin(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouterWithRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?start_date=2026-06-01&end_date=2026-06-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.statsFilters.UsePresentationMultiplier)
}

func TestAdminUsageStatsRequestsRawViewForSuperAdmin(t *testing.T) {
	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouterWithRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?start_date=2026-06-01&end_date=2026-06-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, repo.statsFilters.UsePresentationMultiplier)
}

func TestAdminUsageStatsCacheSeparatesPresentationAndRawViews(t *testing.T) {
	usageStatsCache = newSnapshotCache(30 * time.Second)
	repo := &adminUsageRepoCapture{}
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		role := c.GetHeader("X-Test-Role")
		if role == "" {
			role = service.RoleAdmin
		}
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	})
	router.GET("/admin/usage/stats", handler.Stats)

	adminReq := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?start_date=2026-06-01&end_date=2026-06-02", nil)
	adminRec := httptest.NewRecorder()
	router.ServeHTTP(adminRec, adminReq)
	require.Equal(t, http.StatusOK, adminRec.Code)
	require.Equal(t, 1, repo.statsCalls)
	require.True(t, repo.statsFilters.UsePresentationMultiplier)

	superReq := httptest.NewRequest(http.MethodGet, "/admin/usage/stats?start_date=2026-06-01&end_date=2026-06-02", nil)
	superReq.Header.Set("X-Test-Role", service.RoleSuperAdmin)
	superRec := httptest.NewRecorder()
	router.ServeHTTP(superRec, superReq)
	require.Equal(t, http.StatusOK, superRec.Code)
	require.Equal(t, 2, repo.statsCalls, "raw and presentation stats must not share cache")
	require.False(t, repo.statsFilters.UsePresentationMultiplier)
}

func TestAdminUsageStatsHidesAccountCostForAdmin(t *testing.T) {
	accountCost := 12.5
	repo := &adminUsageRepoCapture{
		stats: &usagestats.UsageStats{TotalAccountCost: &accountCost},
	}
	router := newAdminUsageRequestTypeTestRouterWithRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got adminUsageStatsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Nil(t, got.Data.TotalAccountCost)
}

func TestAdminUsageStatsKeepsAccountCostForSuperAdmin(t *testing.T) {
	accountCost := 12.5
	repo := &adminUsageRepoCapture{
		stats: &usagestats.UsageStats{TotalAccountCost: &accountCost},
	}
	router := newAdminUsageRequestTypeTestRouterWithRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got adminUsageStatsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.NotNil(t, got.Data.TotalAccountCost)
	require.Equal(t, 12.5, *got.Data.TotalAccountCost)
}

func TestAdminUsageListUsesPresentationViewForAdmin(t *testing.T) {
	repo := &adminUsageRepoCapture{listLogs: []service.UsageLog{{
		RequestID:              "req_admin_presentation",
		Model:                  "claude-sonnet-4",
		InputTokens:            600,
		OutputTokens:           500,
		TotalCost:              0.02,
		ActualCost:             0.04,
		RateMultiplier:         1.5,
		PresentationMultiplier: 2,
	}}}
	router := newAdminUsageRequestTypeTestRouterWithRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got adminUsageListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data.Items, 1)
	require.Equal(t, 1200, got.Data.Items[0].InputTokens)
	require.Equal(t, 1000, got.Data.Items[0].OutputTokens)
	require.InDelta(t, 0.04, got.Data.Items[0].TotalCost, 1e-12)
	require.InDelta(t, 0.08, got.Data.Items[0].ActualCost, 1e-12)
	require.InDelta(t, 1.0, got.Data.Items[0].RateMultiplier, 1e-12)
}

func TestAdminUsageListUsesRawViewForSuperAdmin(t *testing.T) {
	repo := &adminUsageRepoCapture{listLogs: []service.UsageLog{{
		RequestID:              "req_super_admin_raw",
		Model:                  "claude-sonnet-4",
		InputTokens:            600,
		OutputTokens:           500,
		TotalCost:              0.02,
		ActualCost:             0.04,
		RateMultiplier:         1.5,
		PresentationMultiplier: 2,
	}}}
	router := newAdminUsageRequestTypeTestRouterWithRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got adminUsageListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data.Items, 1)
	require.Equal(t, 600, got.Data.Items[0].InputTokens)
	require.Equal(t, 500, got.Data.Items[0].OutputTokens)
	require.InDelta(t, 0.02, got.Data.Items[0].TotalCost, 1e-12)
	require.InDelta(t, 0.04, got.Data.Items[0].ActualCost, 1e-12)
	require.InDelta(t, 1.5, got.Data.Items[0].RateMultiplier, 1e-12)
}
