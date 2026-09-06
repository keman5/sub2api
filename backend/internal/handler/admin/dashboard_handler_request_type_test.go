package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardUsageRepoCapture struct {
	service.UsageLogRepository
	trendRequestType         *int16
	trendStream              *bool
	trendNativeCompaction    *bool
	modelRequestType         *int16
	modelStream              *bool
	modelNativeCompaction    *bool
	groupNativeCompaction    *bool
	trendMismatch            *bool
	modelMismatch            *bool
	groupMismatch            *bool
	rankingLimit             int
	ranking                  []usagestats.UserSpendingRankingItem
	rankingTotal             float64
	trendViewCalled          bool
	trendPresentation        bool
	modelViewCalled          bool
	modelPresentation        bool
	rawStatsCalled           bool
	statsViewCalled          bool
	statsPresentation        bool
	groupViewCalled          bool
	groupPresentation        bool
	rankingViewCalled        bool
	rankingPresentation      bool
	batchUsersViewCalled     bool
	batchUsersPresentation   bool
	batchAPIKeysViewCalled   bool
	batchAPIKeysPresentation bool
	apiKeysTrendViewCalled   bool
	apiKeysTrendPresentation bool
	usersTrendViewCalled     bool
	usersTrendPresentation   bool
}

func (s *dashboardUsageRepoCapture) GetUsageTrendWithUsageFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	filters usagestats.UsageLogFilters,
) ([]usagestats.TrendDataPoint, error) {
	s.trendRequestType = filters.RequestType
	s.trendStream = filters.Stream
	s.trendNativeCompaction = filters.NativeCompactionV2
	s.trendMismatch = filters.UpstreamModelMismatch
	return []usagestats.TrendDataPoint{}, nil
}

func (s *dashboardUsageRepoCapture) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	s.trendRequestType = requestType
	s.trendStream = stream
	return []usagestats.TrendDataPoint{}, nil
}

func (s *dashboardUsageRepoCapture) GetUsageTrendWithFiltersForView(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
	usePresentation bool,
) ([]usagestats.TrendDataPoint, error) {
	s.trendViewCalled = true
	s.trendPresentation = usePresentation
	return s.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
}

func (s *dashboardUsageRepoCapture) GetDashboardStats(context.Context) (*usagestats.DashboardStats, error) {
	s.rawStatsCalled = true
	return &usagestats.DashboardStats{TotalRequests: 1, TotalInputTokens: 10, TotalTokens: 10, TotalAccountCost: 12.5, TodayAccountCost: 3.25}, nil
}

func (s *dashboardUsageRepoCapture) GetDashboardStatsForView(ctx context.Context, usePresentation bool) (*usagestats.DashboardStats, error) {
	s.statsViewCalled = true
	s.statsPresentation = usePresentation
	return &usagestats.DashboardStats{TotalRequests: 1, TotalInputTokens: 10, TotalTokens: 10, TotalAccountCost: 12.5, TodayAccountCost: 3.25}, nil
}

func (s *dashboardUsageRepoCapture) GetModelStatsWithUsageFiltersBySource(
	ctx context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
	source string,
) ([]usagestats.ModelStat, error) {
	s.modelRequestType = filters.RequestType
	s.modelStream = filters.Stream
	s.modelNativeCompaction = filters.NativeCompactionV2
	s.modelMismatch = filters.UpstreamModelMismatch
	return []usagestats.ModelStat{}, nil
}

func (s *dashboardUsageRepoCapture) GetGroupStatsWithUsageFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
) ([]usagestats.GroupStat, error) {
	s.groupNativeCompaction = filters.NativeCompactionV2
	s.groupMismatch = filters.UpstreamModelMismatch
	return []usagestats.GroupStat{}, nil
}

func (s *dashboardUsageRepoCapture) GetModelStatsWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.ModelStat, error) {
	s.modelRequestType = requestType
	s.modelStream = stream
	return []usagestats.ModelStat{}, nil
}

func (s *dashboardUsageRepoCapture) GetModelStatsWithFiltersBySourceForView(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	requestType *int16,
	stream *bool,
	billingType *int8,
	source string,
	usePresentation bool,
) ([]usagestats.ModelStat, error) {
	s.modelViewCalled = true
	s.modelPresentation = usePresentation
	return s.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
}

func (s *dashboardUsageRepoCapture) GetGroupStatsWithFiltersForView(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	requestType *int16,
	stream *bool,
	billingType *int8,
	usePresentation bool,
) ([]usagestats.GroupStat, error) {
	s.groupViewCalled = true
	s.groupPresentation = usePresentation
	return []usagestats.GroupStat{}, nil
}

func (s *dashboardUsageRepoCapture) GetUserSpendingRanking(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) (*usagestats.UserSpendingRankingResponse, error) {
	s.rankingLimit = limit
	return &usagestats.UserSpendingRankingResponse{
		Ranking:         s.ranking,
		TotalActualCost: s.rankingTotal,
		TotalRequests:   44,
		TotalTokens:     1234,
	}, nil
}

func (s *dashboardUsageRepoCapture) GetUserSpendingRankingForView(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
	usePresentation bool,
) (*usagestats.UserSpendingRankingResponse, error) {
	s.rankingViewCalled = true
	s.rankingPresentation = usePresentation
	return s.GetUserSpendingRanking(ctx, startTime, endTime, limit)
}

func (s *dashboardUsageRepoCapture) GetBatchUserUsageStatsForView(
	ctx context.Context,
	userIDs []int64,
	startTime, endTime time.Time,
	usePresentation bool,
) (map[int64]*usagestats.BatchUserUsageStats, error) {
	s.batchUsersViewCalled = true
	s.batchUsersPresentation = usePresentation
	return map[int64]*usagestats.BatchUserUsageStats{}, nil
}

func (s *dashboardUsageRepoCapture) GetBatchAPIKeyUsageStatsForView(
	ctx context.Context,
	apiKeyIDs []int64,
	startTime, endTime time.Time,
	usePresentation bool,
) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	s.batchAPIKeysViewCalled = true
	s.batchAPIKeysPresentation = usePresentation
	return map[int64]*usagestats.BatchAPIKeyUsageStats{}, nil
}

func (s *dashboardUsageRepoCapture) GetAPIKeyUsageTrendForView(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	limit int,
	usePresentation bool,
) ([]usagestats.APIKeyUsageTrendPoint, error) {
	s.apiKeysTrendViewCalled = true
	s.apiKeysTrendPresentation = usePresentation
	return []usagestats.APIKeyUsageTrendPoint{}, nil
}

func (s *dashboardUsageRepoCapture) GetUserUsageTrendForView(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	limit int,
	usePresentation bool,
) ([]usagestats.UserUsageTrendPoint, error) {
	s.usersTrendViewCalled = true
	s.usersTrendPresentation = usePresentation
	return []usagestats.UserUsageTrendPoint{}, nil
}

func newDashboardRequestTypeTestRouter(repo *dashboardUsageRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)
	router.GET("/admin/dashboard/stats", handler.GetStats)
	router.GET("/admin/dashboard/models", handler.GetModelStats)
	router.GET("/admin/dashboard/groups", handler.GetGroupStats)
	router.GET("/admin/dashboard/api-keys-trend", handler.GetAPIKeyUsageTrend)
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)
	router.GET("/admin/dashboard/users-ranking", handler.GetUserSpendingRanking)
	router.POST("/admin/dashboard/users-usage", handler.GetBatchUsersUsage)
	router.POST("/admin/dashboard/api-keys-usage", handler.GetBatchAPIKeysUsage)
	return router
}

func newDashboardRequestTypeTestRouterForRole(repo *dashboardUsageRepoCapture, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUserRole), role)
		c.Next()
	})
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)
	router.GET("/admin/dashboard/stats", handler.GetStats)
	router.GET("/admin/dashboard/models", handler.GetModelStats)
	router.GET("/admin/dashboard/groups", handler.GetGroupStats)
	router.GET("/admin/dashboard/api-keys-trend", handler.GetAPIKeyUsageTrend)
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)
	router.GET("/admin/dashboard/users-ranking", handler.GetUserSpendingRanking)
	router.POST("/admin/dashboard/users-usage", handler.GetBatchUsersUsage)
	router.POST("/admin/dashboard/api-keys-usage", handler.GetBatchAPIKeysUsage)
	return router
}

func TestDashboardTrendRequestTypePriority(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?request_type=ws_v2&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.trendRequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.trendRequestType)
	require.Nil(t, repo.trendStream)
}

func TestDashboardTrendUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.trendViewCalled)
	require.True(t, repo.trendPresentation)
}

func TestDashboardTrendUsesRawViewForSuperAdmin(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.trendViewCalled)
	require.False(t, repo.trendPresentation)
}

func TestDashboardStatsUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.statsViewCalled)
	require.True(t, repo.statsPresentation)
	require.False(t, repo.rawStatsCalled)

	var body struct {
		Code int `json:"code"`
		Data struct {
			TotalAccountCost float64 `json:"total_account_cost"`
			TodayAccountCost float64 `json:"today_account_cost"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Zero(t, body.Data.TotalAccountCost)
	require.Zero(t, body.Data.TodayAccountCost)
}

func TestDashboardStatsUsesRawViewForSuperAdmin(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.rawStatsCalled)
	require.False(t, repo.statsViewCalled)
	require.False(t, repo.statsPresentation)

	var body struct {
		Code int `json:"code"`
		Data struct {
			TotalAccountCost float64 `json:"total_account_cost"`
			TodayAccountCost float64 `json:"today_account_cost"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 12.5, body.Data.TotalAccountCost)
	require.Equal(t, 3.25, body.Data.TodayAccountCost)
}

func TestDashboardModelStatsUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardModelStatsCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.modelViewCalled)
	require.True(t, repo.modelPresentation)
}

func TestDashboardModelStatsUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardModelStatsCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.modelViewCalled)
	require.False(t, repo.modelPresentation)
}

func TestDashboardGroupStatsUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardGroupStatsCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/groups?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.groupViewCalled)
	require.True(t, repo.groupPresentation)
}

func TestDashboardGroupStatsUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardGroupStatsCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/groups?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.groupViewCalled)
	require.False(t, repo.groupPresentation)
}

func TestDashboardUsersRankingUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardUsersRankingCache = newSnapshotCache(5 * time.Minute)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.rankingViewCalled)
	require.True(t, repo.rankingPresentation)
}

func TestDashboardUsersRankingUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardUsersRankingCache = newSnapshotCache(5 * time.Minute)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.rankingViewCalled)
	require.False(t, repo.rankingPresentation)
}

func TestDashboardBatchUsersUsageUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardBatchUsersUsageCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/users-usage", strings.NewReader(`{"user_ids":[9]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.batchUsersViewCalled)
	require.True(t, repo.batchUsersPresentation)
}

func TestDashboardBatchUsersUsageUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardBatchUsersUsageCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/users-usage", strings.NewReader(`{"user_ids":[9]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.batchUsersViewCalled)
	require.False(t, repo.batchUsersPresentation)
}

func TestDashboardBatchAPIKeysUsageUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardBatchAPIKeysUsageCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/api-keys-usage", strings.NewReader(`{"api_key_ids":[7]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.batchAPIKeysViewCalled)
	require.True(t, repo.batchAPIKeysPresentation)
}

func TestDashboardBatchAPIKeysUsageUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardBatchAPIKeysUsageCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodPost, "/admin/dashboard/api-keys-usage", strings.NewReader(`{"api_key_ids":[7]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.batchAPIKeysViewCalled)
	require.False(t, repo.batchAPIKeysPresentation)
}

func TestDashboardAPIKeysTrendUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/api-keys-trend?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.apiKeysTrendViewCalled)
	require.True(t, repo.apiKeysTrendPresentation)
}

func TestDashboardAPIKeysTrendUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/api-keys-trend?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.apiKeysTrendViewCalled)
	require.False(t, repo.apiKeysTrendPresentation)
}

func TestDashboardUsersTrendUsesPresentationViewForOrdinaryAdmin(t *testing.T) {
	dashboardUsersTrendCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.usersTrendViewCalled)
	require.True(t, repo.usersTrendPresentation)
}

func TestDashboardUsersTrendUsesRawViewForSuperAdmin(t *testing.T) {
	dashboardUsersTrendCache = newSnapshotCache(30 * time.Second)
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouterForRole(repo, service.RoleSuperAdmin)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.usersTrendViewCalled)
	require.False(t, repo.usersTrendPresentation)
}

func TestDashboardTrendInvalidRequestType(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?request_type=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDashboardTrendInvalidStream(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDashboardModelStatsRequestTypePriority(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?request_type=sync&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.modelRequestType)
	require.Equal(t, int16(service.RequestTypeSync), *repo.modelRequestType)
	require.Nil(t, repo.modelStream)
}

func TestDashboardModelStatsInvalidRequestType(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?request_type=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDashboardModelStatsInvalidStream(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDashboardModelStatsInvalidModelSource(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?model_source=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDashboardModelStatsValidModelSource(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/models?model_source=upstream", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestDashboardNativeCompactionFilterPropagatesAlongsideTransport(t *testing.T) {
	resetDashboardReadCachesForTest()
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	for _, path := range []string{
		"/admin/dashboard/trend?request_type=stream&native_compaction_v2=true",
		"/admin/dashboard/models?request_type=stream&native_compaction_v2=true",
		"/admin/dashboard/groups?request_type=stream&native_compaction_v2=true",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
	}

	require.NotNil(t, repo.trendNativeCompaction)
	require.True(t, *repo.trendNativeCompaction)
	require.NotNil(t, repo.modelNativeCompaction)
	require.True(t, *repo.modelNativeCompaction)
	require.NotNil(t, repo.groupNativeCompaction)
	require.True(t, *repo.groupNativeCompaction)
	require.NotNil(t, repo.trendRequestType)
	require.Equal(t, int16(service.RequestTypeStream), *repo.trendRequestType)
}

func TestDashboardNativeCompactionFilterRejectsInvalidBoolean(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	for _, path := range []string{
		"/admin/dashboard/trend?native_compaction_v2=invalid",
		"/admin/dashboard/models?native_compaction_v2=invalid",
		"/admin/dashboard/groups?native_compaction_v2=invalid",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, path)
	}
}

func TestDashboardModelAuditFilterPropagatesToTrendModelAndGroupQueries(t *testing.T) {
	resetDashboardReadCachesForTest()
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	for _, path := range []string{
		"/admin/dashboard/trend?upstream_model_mismatch=true",
		"/admin/dashboard/models?upstream_model_mismatch=true",
		"/admin/dashboard/groups?upstream_model_mismatch=true",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
	}

	require.NotNil(t, repo.trendMismatch)
	require.True(t, *repo.trendMismatch)
	require.NotNil(t, repo.modelMismatch)
	require.True(t, *repo.modelMismatch)
	require.NotNil(t, repo.groupMismatch)
	require.True(t, *repo.groupMismatch)
}

func TestDashboardModelAuditFilterRejectsInvalidBoolean(t *testing.T) {
	repo := &dashboardUsageRepoCapture{}
	router := newDashboardRequestTypeTestRouter(repo)

	for _, path := range []string{
		"/admin/dashboard/trend?upstream_model_mismatch=invalid",
		"/admin/dashboard/models?upstream_model_mismatch=invalid",
		"/admin/dashboard/groups?upstream_model_mismatch=invalid",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, path)
	}
}

func TestDashboardUsersRankingLimitAndCache(t *testing.T) {
	dashboardUsersRankingCache = newSnapshotCache(5 * time.Minute)
	repo := &dashboardUsageRepoCapture{
		ranking: []usagestats.UserSpendingRankingItem{
			{UserID: 7, Email: "rank@example.com", ActualCost: 10.5, Requests: 3, Tokens: 300},
		},
		rankingTotal: 88.8,
	}
	router := newDashboardRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?limit=100&start_date=2025-01-01&end_date=2025-01-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 50, repo.rankingLimit)
	require.Contains(t, rec.Body.String(), "\"total_actual_cost\":88.8")
	require.Contains(t, rec.Body.String(), "\"total_requests\":44")
	require.Contains(t, rec.Body.String(), "\"total_tokens\":1234")
	require.Equal(t, "miss", rec.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-ranking?limit=100&start_date=2025-01-01&end_date=2025-01-02", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
}
