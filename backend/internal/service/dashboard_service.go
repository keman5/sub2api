package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	defaultDashboardStatsFreshTTL       = 15 * time.Second
	defaultDashboardStatsCacheTTL       = 30 * time.Second
	defaultDashboardStatsRefreshTimeout = 30 * time.Second
)

// ErrDashboardStatsCacheMiss 标记仪表盘缓存未命中。
var ErrDashboardStatsCacheMiss = errors.New("仪表盘缓存未命中")

// DashboardStatsCache 定义仪表盘统计缓存接口。
type DashboardStatsCache interface {
	GetDashboardStats(ctx context.Context) (string, error)
	SetDashboardStats(ctx context.Context, data string, ttl time.Duration) error
	DeleteDashboardStats(ctx context.Context) error
}

type dashboardStatsRangeFetcher interface {
	GetDashboardStatsWithRange(ctx context.Context, start, end time.Time) (*usagestats.DashboardStats, error)
}

type dashboardStatsWithViewRepository interface {
	GetDashboardStatsForView(ctx context.Context, usePresentation bool) (*usagestats.DashboardStats, error)
}

type usageTrendWithViewRepository interface {
	GetUsageTrendWithFiltersForView(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, usePresentation bool) ([]usagestats.TrendDataPoint, error)
}

type modelStatsWithViewRepository interface {
	GetModelStatsWithFiltersBySourceForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, source string, usePresentation bool) ([]usagestats.ModelStat, error)
}

type groupStatsWithViewRepository interface {
	GetGroupStatsWithFiltersForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, usePresentation bool) ([]usagestats.GroupStat, error)
}

type groupUsageSummaryWithViewRepository interface {
	GetAllGroupUsageSummaryForView(ctx context.Context, todayStart time.Time, usePresentation bool) ([]usagestats.GroupUsageSummary, error)
}

type userBreakdownWithViewRepository interface {
	GetUserBreakdownStatsForView(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int, usePresentation bool) ([]usagestats.UserBreakdownItem, error)
}

type rankingWithViewRepository interface {
	GetUserSpendingRankingForView(ctx context.Context, startTime, endTime time.Time, limit int, usePresentation bool) (*usagestats.UserSpendingRankingResponse, error)
}

type batchUserUsageWithViewRepository interface {
	GetBatchUserUsageStatsForView(ctx context.Context, userIDs []int64, startTime, endTime time.Time, usePresentation bool) (map[int64]*usagestats.BatchUserUsageStats, error)
}

type batchAPIKeyUsageWithViewRepository interface {
	GetBatchAPIKeyUsageStatsForView(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time, usePresentation bool) (map[int64]*usagestats.BatchAPIKeyUsageStats, error)
}

type entityTrendWithViewRepository interface {
	GetAPIKeyUsageTrendForView(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, usePresentation bool) ([]usagestats.APIKeyUsageTrendPoint, error)
	GetUserUsageTrendForView(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, usePresentation bool) ([]usagestats.UserUsageTrendPoint, error)
}

type dashboardStatsCacheEntry struct {
	Stats     *usagestats.DashboardStats `json:"stats"`
	UpdatedAt int64                      `json:"updated_at"`
}

// DashboardService 提供管理员仪表盘统计服务。
type DashboardService struct {
	usageRepo      UsageLogRepository
	aggRepo        DashboardAggregationRepository
	cache          DashboardStatsCache
	cacheFreshTTL  time.Duration
	cacheTTL       time.Duration
	refreshTimeout time.Duration
	refreshing     int32
	aggEnabled     bool
	aggInterval    time.Duration
	aggLookback    time.Duration
	aggUsageDays   int
}

func NewDashboardService(usageRepo UsageLogRepository, aggRepo DashboardAggregationRepository, cache DashboardStatsCache, cfg *config.Config) *DashboardService {
	freshTTL := defaultDashboardStatsFreshTTL
	cacheTTL := defaultDashboardStatsCacheTTL
	refreshTimeout := defaultDashboardStatsRefreshTimeout
	aggEnabled := true
	aggInterval := time.Minute
	aggLookback := 2 * time.Minute
	aggUsageDays := 90
	if cfg != nil {
		if !cfg.Dashboard.Enabled {
			cache = nil
		}
		if cfg.Dashboard.StatsFreshTTLSeconds > 0 {
			freshTTL = time.Duration(cfg.Dashboard.StatsFreshTTLSeconds) * time.Second
		}
		if cfg.Dashboard.StatsTTLSeconds > 0 {
			cacheTTL = time.Duration(cfg.Dashboard.StatsTTLSeconds) * time.Second
		}
		if cfg.Dashboard.StatsRefreshTimeoutSeconds > 0 {
			refreshTimeout = time.Duration(cfg.Dashboard.StatsRefreshTimeoutSeconds) * time.Second
		}
		aggEnabled = cfg.DashboardAgg.Enabled
		if cfg.DashboardAgg.IntervalSeconds > 0 {
			aggInterval = time.Duration(cfg.DashboardAgg.IntervalSeconds) * time.Second
		}
		if cfg.DashboardAgg.LookbackSeconds > 0 {
			aggLookback = time.Duration(cfg.DashboardAgg.LookbackSeconds) * time.Second
		}
		if cfg.DashboardAgg.Retention.UsageLogsDays > 0 {
			aggUsageDays = cfg.DashboardAgg.Retention.UsageLogsDays
		}
	}
	if aggRepo == nil {
		aggEnabled = false
	}
	return &DashboardService{
		usageRepo:      usageRepo,
		aggRepo:        aggRepo,
		cache:          cache,
		cacheFreshTTL:  freshTTL,
		cacheTTL:       cacheTTL,
		refreshTimeout: refreshTimeout,
		aggEnabled:     aggEnabled,
		aggInterval:    aggInterval,
		aggLookback:    aggLookback,
		aggUsageDays:   aggUsageDays,
	}
}

func (s *DashboardService) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	if s.cache != nil {
		cached, fresh, err := s.getCachedDashboardStats(ctx)
		if err == nil && cached != nil {
			s.refreshAggregationStaleness(cached)
			if !fresh {
				s.refreshDashboardStatsAsync()
			}
			return cached, nil
		}
		if err != nil && !errors.Is(err, ErrDashboardStatsCacheMiss) {
			logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存读取失败: %v", err)
		}
	}

	stats, err := s.refreshDashboardStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get dashboard stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetDashboardStatsForView(ctx context.Context, usePresentation bool) (*usagestats.DashboardStats, error) {
	if !usePresentation {
		return s.GetDashboardStats(ctx)
	}
	if repo, ok := s.usageRepo.(dashboardStatsWithViewRepository); ok {
		stats, err := repo.GetDashboardStatsForView(ctx, true)
		if err != nil {
			return nil, fmt.Errorf("get dashboard stats: %w", err)
		}
		s.applyAggregationStatus(ctx, stats)
		return dashboardStatsWithoutAccountCost(stats), nil
	}
	stats, err := s.GetDashboardStats(ctx)
	if err != nil {
		return nil, err
	}
	return dashboardStatsWithoutAccountCost(stats), nil
}

func (s *DashboardService) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	trend, err := s.usageRepo.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
	if err != nil {
		return nil, fmt.Errorf("get usage trend with filters: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetUsageTrendWithFiltersForView(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, usePresentation bool) ([]usagestats.TrendDataPoint, error) {
	if repo, ok := s.usageRepo.(usageTrendWithViewRepository); ok {
		trend, err := repo.GetUsageTrendWithFiltersForView(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get usage trend with filters: %w", err)
		}
		return trend, nil
	}
	return s.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
}

func (s *DashboardService) GetUsageTrendWithUsageFilters(ctx context.Context, startTime, endTime time.Time, granularity string, filters usagestats.UsageLogFilters) ([]usagestats.TrendDataPoint, error) {
	type usageTrendWithFiltersRepo interface {
		GetUsageTrendWithUsageFilters(context.Context, time.Time, time.Time, string, usagestats.UsageLogFilters) ([]usagestats.TrendDataPoint, error)
	}
	// The legacy view repository carries the raw/presentation contract. Keep it
	// for the normal admin view; the newer usage-filter path is required only
	// when upstream model mismatch filtering is present because the legacy view
	// API cannot carry that predicate.
	if filters.UpstreamModelMismatch == nil && filters.NativeCompactionV2 == nil {
		if _, ok := s.usageRepo.(usageTrendWithViewRepository); ok {
			return s.GetUsageTrendWithFiltersForView(ctx, startTime, endTime, granularity, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.RequestType, filters.Stream, filters.BillingType, filters.UsePresentationMultiplier)
		}
	}
	if repo, ok := s.usageRepo.(usageTrendWithFiltersRepo); ok {
		trend, err := repo.GetUsageTrendWithUsageFilters(ctx, startTime, endTime, granularity, filters)
		if err != nil {
			return nil, fmt.Errorf("get usage trend with usage filters: %w", err)
		}
		return trend, nil
	}
	if filters.UsePresentationMultiplier {
		return s.GetUsageTrendWithFiltersForView(ctx, startTime, endTime, granularity, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.RequestType, filters.Stream, filters.BillingType, true)
	}
	return s.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.Model, filters.RequestType, filters.Stream, filters.BillingType)
}

func (s *DashboardService) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	stats, err := s.usageRepo.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	if err != nil {
		return nil, fmt.Errorf("get model stats with filters: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetModelStatsWithFiltersBySource(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, modelSource string) ([]usagestats.ModelStat, error) {
	normalizedSource := usagestats.NormalizeModelSource(modelSource)
	if normalizedSource == usagestats.ModelSourceRequested {
		return s.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	}

	type modelStatsBySourceRepo interface {
		GetModelStatsWithFiltersBySource(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, source string) ([]usagestats.ModelStat, error)
	}

	if sourceRepo, ok := s.usageRepo.(modelStatsBySourceRepo); ok {
		stats, err := sourceRepo.GetModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType, normalizedSource)
		if err != nil {
			return nil, fmt.Errorf("get model stats with filters by source: %w", err)
		}
		return stats, nil
	}

	return s.GetModelStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
}

func (s *DashboardService) GetModelStatsWithFiltersBySourceForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, modelSource string, usePresentation bool) ([]usagestats.ModelStat, error) {
	normalizedSource := usagestats.NormalizeModelSource(modelSource)
	if repo, ok := s.usageRepo.(modelStatsWithViewRepository); ok {
		stats, err := repo.GetModelStatsWithFiltersBySourceForView(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType, normalizedSource, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get model stats with filters by source: %w", err)
		}
		return modelStatsForPresentation(stats, usePresentation), nil
	}
	stats, err := s.GetModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType, normalizedSource)
	if err != nil {
		return nil, err
	}
	return modelStatsForPresentation(stats, usePresentation), nil
}

func (s *DashboardService) GetModelStatsWithUsageFiltersBySource(ctx context.Context, startTime, endTime time.Time, filters usagestats.UsageLogFilters, modelSource string) ([]usagestats.ModelStat, error) {
	normalizedSource := usagestats.NormalizeModelSource(modelSource)
	type modelStatsWithFiltersRepo interface {
		GetModelStatsWithUsageFiltersBySource(context.Context, time.Time, time.Time, usagestats.UsageLogFilters, string) ([]usagestats.ModelStat, error)
	}
	if filters.UpstreamModelMismatch == nil && filters.NativeCompactionV2 == nil {
		if _, ok := s.usageRepo.(modelStatsWithViewRepository); ok {
			return s.GetModelStatsWithFiltersBySourceForView(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType, normalizedSource, filters.UsePresentationMultiplier)
		}
	}
	if repo, ok := s.usageRepo.(modelStatsWithFiltersRepo); ok {
		stats, err := repo.GetModelStatsWithUsageFiltersBySource(ctx, startTime, endTime, filters, normalizedSource)
		if err != nil {
			return nil, fmt.Errorf("get model stats with usage filters by source: %w", err)
		}
		return modelStatsForPresentation(stats, filters.UsePresentationMultiplier), nil
	}
	if filters.UsePresentationMultiplier {
		return s.GetModelStatsWithFiltersBySourceForView(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType, normalizedSource, true)
	}
	return s.GetModelStatsWithFiltersBySource(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType, normalizedSource)
}

func (s *DashboardService) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error) {
	stats, err := s.usageRepo.GetGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	if err != nil {
		return nil, fmt.Errorf("get group stats with filters: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetGroupStatsWithFiltersForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, usePresentation bool) ([]usagestats.GroupStat, error) {
	if repo, ok := s.usageRepo.(groupStatsWithViewRepository); ok {
		stats, err := repo.GetGroupStatsWithFiltersForView(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get group stats with filters: %w", err)
		}
		return groupStatsForPresentation(stats, usePresentation), nil
	}
	stats, err := s.GetGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	if err != nil {
		return nil, err
	}
	return groupStatsForPresentation(stats, usePresentation), nil
}

func (s *DashboardService) GetGroupStatsWithUsageFilters(ctx context.Context, startTime, endTime time.Time, filters usagestats.UsageLogFilters) ([]usagestats.GroupStat, error) {
	type groupStatsWithFiltersRepo interface {
		GetGroupStatsWithUsageFilters(context.Context, time.Time, time.Time, usagestats.UsageLogFilters) ([]usagestats.GroupStat, error)
	}
	if filters.UpstreamModelMismatch == nil && filters.NativeCompactionV2 == nil {
		if _, ok := s.usageRepo.(groupStatsWithViewRepository); ok {
			return s.GetGroupStatsWithFiltersForView(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType, filters.UsePresentationMultiplier)
		}
	}
	if repo, ok := s.usageRepo.(groupStatsWithFiltersRepo); ok {
		stats, err := repo.GetGroupStatsWithUsageFilters(ctx, startTime, endTime, filters)
		if err != nil {
			return nil, fmt.Errorf("get group stats with usage filters: %w", err)
		}
		return groupStatsForPresentation(stats, filters.UsePresentationMultiplier), nil
	}
	if filters.UsePresentationMultiplier {
		return s.GetGroupStatsWithFiltersForView(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType, true)
	}
	return s.GetGroupStatsWithFilters(ctx, startTime, endTime, filters.UserID, filters.APIKeyID, filters.AccountID, filters.GroupID, filters.RequestType, filters.Stream, filters.BillingType)
}

// GetGroupUsageSummary returns today's, yesterday's, and cumulative cost for all groups.
func (s *DashboardService) GetGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error) {
	results, err := s.usageRepo.GetAllGroupUsageSummary(ctx, todayStart)
	if err != nil {
		return nil, fmt.Errorf("get group usage summary: %w", err)
	}
	return results, nil
}

// GetGroupUsageSummaryForView returns today's and cumulative group cost for the requested usage view.
func (s *DashboardService) GetGroupUsageSummaryForView(ctx context.Context, todayStart time.Time, viewMode UsageViewMode) ([]usagestats.GroupUsageSummary, error) {
	usePresentation := viewMode == UsageViewPresentation
	if repo, ok := s.usageRepo.(groupUsageSummaryWithViewRepository); ok {
		results, err := repo.GetAllGroupUsageSummaryForView(ctx, todayStart, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get group usage summary: %w", err)
		}
		return results, nil
	}
	return s.GetGroupUsageSummary(ctx, todayStart)
}

func (s *DashboardService) getCachedDashboardStats(ctx context.Context) (*usagestats.DashboardStats, bool, error) {
	data, err := s.cache.GetDashboardStats(ctx)
	if err != nil {
		return nil, false, err
	}

	var entry dashboardStatsCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		s.evictDashboardStatsCache(err)
		return nil, false, ErrDashboardStatsCacheMiss
	}
	if entry.Stats == nil {
		s.evictDashboardStatsCache(errors.New("仪表盘缓存缺少统计数据"))
		return nil, false, ErrDashboardStatsCacheMiss
	}

	age := time.Since(time.Unix(entry.UpdatedAt, 0))
	return entry.Stats, age <= s.cacheFreshTTL, nil
}

func (s *DashboardService) refreshDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	stats, err := s.fetchDashboardStats(ctx)
	if err != nil {
		return nil, err
	}
	s.applyAggregationStatus(ctx, stats)
	cacheCtx, cancel := s.cacheOperationContext()
	defer cancel()
	s.saveDashboardStatsCache(cacheCtx, stats)
	return stats, nil
}

func (s *DashboardService) refreshDashboardStatsAsync() {
	if s.cache == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&s.refreshing, 0, 1) {
		return
	}

	go func() {
		defer atomic.StoreInt32(&s.refreshing, 0)

		ctx, cancel := context.WithTimeout(context.Background(), s.refreshTimeout)
		defer cancel()

		stats, err := s.fetchDashboardStats(ctx)
		if err != nil {
			logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存异步刷新失败: %v", err)
			return
		}
		s.applyAggregationStatus(ctx, stats)
		cacheCtx, cancel := s.cacheOperationContext()
		defer cancel()
		s.saveDashboardStatsCache(cacheCtx, stats)
	}()
}

func (s *DashboardService) fetchDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	if !s.aggEnabled {
		if fetcher, ok := s.usageRepo.(dashboardStatsRangeFetcher); ok {
			now := time.Now().UTC()
			start := truncateToDayUTC(now.AddDate(0, 0, -s.aggUsageDays))
			return fetcher.GetDashboardStatsWithRange(ctx, start, now)
		}
	}
	return s.usageRepo.GetDashboardStats(ctx)
}

func (s *DashboardService) saveDashboardStatsCache(ctx context.Context, stats *usagestats.DashboardStats) {
	if s.cache == nil || stats == nil {
		return
	}

	entry := dashboardStatsCacheEntry{
		Stats:     stats,
		UpdatedAt: time.Now().Unix(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存序列化失败: %v", err)
		return
	}

	if err := s.cache.SetDashboardStats(ctx, string(data), s.cacheTTL); err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存写入失败: %v", err)
	}
}

func (s *DashboardService) evictDashboardStatsCache(reason error) {
	if s.cache == nil {
		return
	}
	cacheCtx, cancel := s.cacheOperationContext()
	defer cancel()

	if err := s.cache.DeleteDashboardStats(cacheCtx); err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存清理失败: %v", err)
	}
	if reason != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 仪表盘缓存异常，已清理: %v", reason)
	}
}

func (s *DashboardService) cacheOperationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.refreshTimeout)
}

func (s *DashboardService) applyAggregationStatus(ctx context.Context, stats *usagestats.DashboardStats) {
	if stats == nil {
		return
	}
	updatedAt := s.fetchAggregationUpdatedAt(ctx)
	stats.StatsUpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	stats.StatsStale = s.isAggregationStale(updatedAt, time.Now().UTC())
}

func (s *DashboardService) refreshAggregationStaleness(stats *usagestats.DashboardStats) {
	if stats == nil {
		return
	}
	updatedAt := parseStatsUpdatedAt(stats.StatsUpdatedAt)
	stats.StatsStale = s.isAggregationStale(updatedAt, time.Now().UTC())
}

func (s *DashboardService) fetchAggregationUpdatedAt(ctx context.Context) time.Time {
	if s.aggRepo == nil {
		return time.Unix(0, 0).UTC()
	}
	updatedAt, err := s.aggRepo.GetAggregationWatermark(ctx)
	if err != nil {
		logger.LegacyPrintf("service.dashboard", "[Dashboard] 读取聚合水位失败: %v", err)
		return time.Unix(0, 0).UTC()
	}
	if updatedAt.IsZero() {
		return time.Unix(0, 0).UTC()
	}
	return updatedAt.UTC()
}

func (s *DashboardService) isAggregationStale(updatedAt, now time.Time) bool {
	if !s.aggEnabled {
		return true
	}
	epoch := time.Unix(0, 0).UTC()
	if !updatedAt.After(epoch) {
		return true
	}
	threshold := s.aggInterval + s.aggLookback
	return now.Sub(updatedAt) > threshold
}

func parseStatsUpdatedAt(raw string) time.Time {
	if raw == "" {
		return time.Unix(0, 0).UTC()
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return parsed.UTC()
}

func (s *DashboardService) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetAPIKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
	if err != nil {
		return nil, fmt.Errorf("get api key usage trend: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetAPIKeyUsageTrendForView(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, usePresentation bool) ([]usagestats.APIKeyUsageTrendPoint, error) {
	if repo, ok := s.usageRepo.(entityTrendWithViewRepository); ok {
		trend, err := repo.GetAPIKeyUsageTrendForView(ctx, startTime, endTime, granularity, limit, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get api key usage trend: %w", err)
		}
		return trend, nil
	}
	return s.GetAPIKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
}

func (s *DashboardService) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit)
	if err != nil {
		return nil, fmt.Errorf("get user usage trend: %w", err)
	}
	return trend, nil
}

func (s *DashboardService) GetUserUsageTrendForView(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, usePresentation bool) ([]usagestats.UserUsageTrendPoint, error) {
	if repo, ok := s.usageRepo.(entityTrendWithViewRepository); ok {
		trend, err := repo.GetUserUsageTrendForView(ctx, startTime, endTime, granularity, limit, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get user usage trend: %w", err)
		}
		return trend, nil
	}
	return s.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit)
}

func (s *DashboardService) GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	ranking, err := s.usageRepo.GetUserSpendingRanking(ctx, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("get user spending ranking: %w", err)
	}
	return ranking, nil
}

func (s *DashboardService) GetUserSpendingRankingForView(ctx context.Context, startTime, endTime time.Time, limit int, usePresentation bool) (*usagestats.UserSpendingRankingResponse, error) {
	if repo, ok := s.usageRepo.(rankingWithViewRepository); ok {
		ranking, err := repo.GetUserSpendingRankingForView(ctx, startTime, endTime, limit, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get user spending ranking: %w", err)
		}
		return ranking, nil
	}
	return s.GetUserSpendingRanking(ctx, startTime, endTime, limit)
}

func (s *DashboardService) GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error) {
	stats, err := s.usageRepo.GetUserBreakdownStats(ctx, startTime, endTime, dim, limit)
	if err != nil {
		return nil, fmt.Errorf("get user breakdown stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetUserBreakdownStatsForView(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int, usePresentation bool) ([]usagestats.UserBreakdownItem, error) {
	if repo, ok := s.usageRepo.(userBreakdownWithViewRepository); ok {
		stats, err := repo.GetUserBreakdownStatsForView(ctx, startTime, endTime, dim, limit, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get user breakdown stats: %w", err)
		}
		return userBreakdownForPresentation(stats, usePresentation), nil
	}
	stats, err := s.GetUserBreakdownStats(ctx, startTime, endTime, dim, limit)
	if err != nil {
		return nil, err
	}
	return userBreakdownForPresentation(stats, usePresentation), nil
}

func dashboardStatsWithoutAccountCost(stats *usagestats.DashboardStats) *usagestats.DashboardStats {
	if stats == nil {
		return nil
	}
	out := *stats
	out.TotalAccountCost = 0
	out.TodayAccountCost = 0
	return &out
}

func modelStatsForPresentation(stats []usagestats.ModelStat, usePresentation bool) []usagestats.ModelStat {
	if !usePresentation {
		return stats
	}
	out := append([]usagestats.ModelStat(nil), stats...)
	for i := range out {
		out[i].AccountCost = 0
	}
	return out
}

func groupStatsForPresentation(stats []usagestats.GroupStat, usePresentation bool) []usagestats.GroupStat {
	if !usePresentation {
		return stats
	}
	out := append([]usagestats.GroupStat(nil), stats...)
	for i := range out {
		out[i].AccountCost = 0
	}
	return out
}

func userBreakdownForPresentation(stats []usagestats.UserBreakdownItem, usePresentation bool) []usagestats.UserBreakdownItem {
	if !usePresentation {
		return stats
	}
	out := append([]usagestats.UserBreakdownItem(nil), stats...)
	for i := range out {
		out[i].AccountCost = 0
	}
	return out
}

func (s *DashboardService) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	stats, err := s.usageRepo.GetBatchUserUsageStats(ctx, userIDs, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get batch user usage stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetBatchUserUsageStatsForView(ctx context.Context, userIDs []int64, startTime, endTime time.Time, usePresentation bool) (map[int64]*usagestats.BatchUserUsageStats, error) {
	if repo, ok := s.usageRepo.(batchUserUsageWithViewRepository); ok {
		stats, err := repo.GetBatchUserUsageStatsForView(ctx, userIDs, startTime, endTime, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get batch user usage stats: %w", err)
		}
		return stats, nil
	}
	return s.GetBatchUserUsageStats(ctx, userIDs, startTime, endTime)
}

func (s *DashboardService) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	stats, err := s.usageRepo.GetBatchAPIKeyUsageStats(ctx, apiKeyIDs, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get batch api key usage stats: %w", err)
	}
	return stats, nil
}

func (s *DashboardService) GetBatchAPIKeyUsageStatsForView(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time, usePresentation bool) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	if repo, ok := s.usageRepo.(batchAPIKeyUsageWithViewRepository); ok {
		stats, err := repo.GetBatchAPIKeyUsageStatsForView(ctx, apiKeyIDs, startTime, endTime, usePresentation)
		if err != nil {
			return nil, fmt.Errorf("get batch api key usage stats: %w", err)
		}
		return stats, nil
	}
	return s.GetBatchAPIKeyUsageStats(ctx, apiKeyIDs, startTime, endTime)
}
