package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/lib/pq"
)

func (r *usageLogRepository) GetDashboardStatsForView(ctx context.Context, usePresentation bool) (*DashboardStats, error) {
	if !usePresentation {
		return r.GetDashboardStats(ctx)
	}

	stats := &DashboardStats{}
	now := timezone.Now()
	todayStart := timezone.Today()
	if err := r.fillDashboardEntityStats(ctx, stats, todayStart, now); err != nil {
		return nil, err
	}
	if err := r.fillDashboardUsageStatsFromUsageLogsForView(ctx, stats, time.Time{}.UTC(), now.UTC(), todayStart, now, true); err != nil {
		return nil, err
	}

	rpm, tpm, err := r.getPerformanceStatsForView(ctx, 0, true)
	if err != nil {
		return nil, err
	}
	stats.Rpm = rpm
	stats.Tpm = tpm
	return stats, nil
}

func (r *usageLogRepository) GetAccountTodayStatsForView(ctx context.Context, accountID int64, usePresentation bool) (*usagestats.AccountStats, error) {
	return r.GetAccountWindowStatsForView(ctx, accountID, timezone.Today(), usePresentation)
}

func (r *usageLogRepository) GetAccountWindowStatsForView(ctx context.Context, accountID int64, startTime time.Time, usePresentation bool) (*usagestats.AccountStats, error) {
	if !usePresentation {
		return r.GetAccountWindowStats(ctx, accountID, startTime)
	}
	presentationFactor := usagePresentationFactorSQL("", true)
	totalTokensExpr := usagePresentationTotalTokensSQL("", presentationFactor)
	accountCostExpr := usagePresentationCostSQL("COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)", presentationFactor)
	standardCostExpr := usagePresentationCostSQL("total_cost", presentationFactor)
	userCostExpr := usagePresentationCostSQL("actual_cost", presentationFactor)

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as requests,
			COALESCE(SUM(%s), 0) as tokens,
			COALESCE(SUM(%s), 0) as cost,
			COALESCE(SUM(%s), 0) as standard_cost,
			COALESCE(SUM(%s), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
	`, totalTokensExpr, accountCostExpr, standardCostExpr, userCostExpr)

	stats := &usagestats.AccountStats{}
	if err := scanSingleRow(ctx, r.sql, query, []any{accountID, startTime}, &stats.Requests, &stats.Tokens, &stats.Cost, &stats.StandardCost, &stats.UserCost); err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *usageLogRepository) GetAccountWindowStatsBatchForView(ctx context.Context, accountIDs []int64, startTime time.Time, usePresentation bool) (map[int64]*usagestats.AccountStats, error) {
	if !usePresentation {
		return r.GetAccountWindowStatsBatch(ctx, accountIDs, startTime)
	}
	result := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}

	presentationFactor := usagePresentationFactorSQL("", true)
	totalTokensExpr := usagePresentationTotalTokensSQL("", presentationFactor)
	accountCostExpr := usagePresentationCostSQL("COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)", presentationFactor)
	standardCostExpr := usagePresentationCostSQL("total_cost", presentationFactor)
	userCostExpr := usagePresentationCostSQL("actual_cost", presentationFactor)

	query := fmt.Sprintf(`
		SELECT
			account_id,
			COUNT(*) as requests,
			COALESCE(SUM(%s), 0) as tokens,
			COALESCE(SUM(%s), 0) as cost,
			COALESCE(SUM(%s), 0) as standard_cost,
			COALESCE(SUM(%s), 0) as user_cost
		FROM usage_logs
		WHERE account_id = ANY($1) AND created_at >= $2
		GROUP BY account_id
	`, totalTokensExpr, accountCostExpr, standardCostExpr, userCostExpr)
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var accountID int64
		stats := &usagestats.AccountStats{}
		if err := rows.Scan(&accountID, &stats.Requests, &stats.Tokens, &stats.Cost, &stats.StandardCost, &stats.UserCost); err != nil {
			return nil, err
		}
		result[accountID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, accountID := range accountIDs {
		if _, ok := result[accountID]; !ok {
			result[accountID] = &usagestats.AccountStats{}
		}
	}
	return result, nil
}

func (r *usageLogRepository) GetBatchUserUsageStatsForView(ctx context.Context, userIDs []int64, startTime, endTime time.Time, usePresentation bool) (map[int64]*BatchUserUsageStats, error) {
	if !usePresentation {
		return r.GetBatchUserUsageStats(ctx, userIDs, startTime, endTime)
	}
	result := make(map[int64]*BatchUserUsageStats)
	normalizedUserIDs := normalizePositiveInt64IDs(userIDs)
	if len(normalizedUserIDs) == 0 {
		return result, nil
	}
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	for _, id := range normalizedUserIDs {
		result[id] = &BatchUserUsageStats{UserID: id}
	}

	presentationFactor := usagePresentationFactorSQL("ul.", true)
	actualCostExpr := usagePresentationCostSQL("ul.actual_cost", presentationFactor)
	query := `
		SELECT
			ul.user_id,
			` + usageLogEffectivePlatformExpr + ` as platform,
			COALESCE(SUM(` + actualCostExpr + `) FILTER (WHERE ul.created_at >= $2 AND ul.created_at < $3), 0) as total_cost,
			COALESCE(SUM(` + actualCostExpr + `) FILTER (WHERE ul.created_at >= $4), 0) as today_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.user_id = ANY($1)
		  AND ul.created_at >= LEAST($2, $4)
		  AND ` + usageLogSuccessFilterUL + `
		GROUP BY ul.user_id, ` + usageLogEffectivePlatformExpr + `
	`
	today := timezone.Today()
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(normalizedUserIDs), startTime, endTime, today)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var userID int64
		var platform sql.NullString
		var total float64
		var todayTotal float64
		if err := rows.Scan(&userID, &platform, &total, &todayTotal); err != nil {
			_ = rows.Close()
			return nil, err
		}
		stats, ok := result[userID]
		if !ok {
			continue
		}
		stats.TotalActualCost += total
		stats.TodayActualCost += todayTotal
		if platform.Valid && platform.String != "" {
			stats.ByPlatform = append(stats.ByPlatform, PlatformUsage{
				Platform:        platform.String,
				TotalActualCost: total,
				TodayActualCost: todayTotal,
			})
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *usageLogRepository) GetBatchAPIKeyUsageStatsForView(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time, usePresentation bool) (map[int64]*BatchAPIKeyUsageStats, error) {
	if !usePresentation {
		return r.GetBatchAPIKeyUsageStats(ctx, apiKeyIDs, startTime, endTime)
	}
	result := make(map[int64]*BatchAPIKeyUsageStats)
	normalizedAPIKeyIDs := normalizePositiveInt64IDs(apiKeyIDs)
	if len(normalizedAPIKeyIDs) == 0 {
		return result, nil
	}
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	for _, id := range normalizedAPIKeyIDs {
		result[id] = &BatchAPIKeyUsageStats{APIKeyID: id}
	}

	presentationFactor := usagePresentationFactorSQL("", true)
	actualCostExpr := usagePresentationCostSQL("actual_cost", presentationFactor)
	query := `
		SELECT
			api_key_id,
			COALESCE(SUM(` + actualCostExpr + `) FILTER (WHERE created_at >= $2 AND created_at < $3), 0) as total_cost,
			COALESCE(SUM(` + actualCostExpr + `) FILTER (WHERE created_at >= $4), 0) as today_cost
		FROM usage_logs
		WHERE api_key_id = ANY($1)
		  AND created_at >= LEAST($2, $4)
		GROUP BY api_key_id
	`
	today := timezone.Today()
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(normalizedAPIKeyIDs), startTime, endTime, today)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var apiKeyID int64
		var total float64
		var todayTotal float64
		if err := rows.Scan(&apiKeyID, &total, &todayTotal); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if stats, ok := result[apiKeyID]; ok {
			stats.TotalActualCost = total
			stats.TodayActualCost = todayTotal
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *usageLogRepository) GetUsageTrendWithFiltersForView(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, usePresentation bool) (results []TrendDataPoint, err error) {
	if !usePresentation {
		return r.GetUsageTrendWithFilters(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, requestType, stream, billingType)
	}
	return r.getUsageTrendWithFiltersForView(ctx, startTime, endTime, granularity, userID, apiKeyID, accountID, groupID, model, "", requestType, stream, billingType, "", true)
}

func (r *usageLogRepository) getUsageTrendWithFiltersForView(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, modelSource string, requestType *int16, stream *bool, billingType *int8, billingMode string, usePresentation bool) (results []TrendDataPoint, err error) {
	dateFormat := safeDateFormat(granularity)
	presentationFactor := usagePresentationFactorSQL("", usePresentation)
	inputTokensExpr := usagePresentationTokenSQL("input_tokens", presentationFactor)
	outputTokensExpr := usagePresentationOutputTokensSQL("", presentationFactor)
	cacheCreationTokensExpr := usagePresentationTokenSQL("cache_creation_tokens", presentationFactor)
	cacheReadTokensExpr := usagePresentationTokenSQL("cache_read_tokens", presentationFactor)
	totalTokensExpr := usagePresentationTotalTokensSQL("", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("total_cost", presentationFactor)
	actualCostExpr := usagePresentationCostSQL("actual_cost", presentationFactor)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(`+inputTokensExpr+`), 0) as input_tokens,
			COALESCE(SUM(`+outputTokensExpr+`), 0) as output_tokens,
			COALESCE(SUM(`+cacheCreationTokensExpr+`), 0) as cache_creation_tokens,
			COALESCE(SUM(`+cacheReadTokensExpr+`), 0) as cache_read_tokens,
			COALESCE(SUM(`+totalTokensExpr+`), 0) as total_tokens,
			COALESCE(SUM(`+totalCostExpr+`), 0) as cost,
			COALESCE(SUM(`+actualCostExpr+`), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, dateFormat)
	args := []any{startTime, endTime}
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
	}
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
	}
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
	}
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
	}
	query, args = appendUsageLogModelQueryFilter(query, args, model, modelSource)
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
	}
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "")
	query += " GROUP BY date ORDER BY date ASC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results, err = scanTrendRows(rows)
	return results, err
}

func (r *usageLogRepository) GetAPIKeyUsageTrendForView(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, usePresentation bool) (results []APIKeyUsageTrendPoint, err error) {
	if !usePresentation {
		return r.GetAPIKeyUsageTrend(ctx, startTime, endTime, granularity, limit)
	}
	dateFormat := safeDateFormat(granularity)
	presentationFactor := usagePresentationFactorSQL("u.", true)
	totalTokensExpr := usagePresentationTotalTokensSQL("u.", presentationFactor)
	topTokensExpr := usagePresentationTotalTokensSQL("", usagePresentationFactorSQL("", true))

	query := fmt.Sprintf(`
		WITH top_keys AS (
			SELECT api_key_id
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY api_key_id
			ORDER BY SUM(%s) DESC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.api_key_id,
			COALESCE(k.name, '') as key_name,
			COUNT(*) as requests,
			COALESCE(SUM(%s), 0) as tokens
		FROM usage_logs u
		LEFT JOIN api_keys k ON u.api_key_id = k.id
		WHERE u.api_key_id IN (SELECT api_key_id FROM top_keys)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.api_key_id, k.name
		ORDER BY date ASC, tokens DESC
	`, topTokensExpr, dateFormat, totalTokensExpr)
	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]APIKeyUsageTrendPoint, 0)
	for rows.Next() {
		var row APIKeyUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.APIKeyID, &row.KeyName, &row.Requests, &row.Tokens); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (r *usageLogRepository) GetUserUsageTrendForView(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, usePresentation bool) (results []UserUsageTrendPoint, err error) {
	if !usePresentation {
		return r.GetUserUsageTrend(ctx, startTime, endTime, granularity, limit)
	}
	dateFormat := safeDateFormat(granularity)
	presentationFactor := usagePresentationFactorSQL("u.", true)
	totalTokensExpr := usagePresentationTotalTokensSQL("u.", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("u.total_cost", presentationFactor)
	actualCostExpr := usagePresentationCostSQL("u.actual_cost", presentationFactor)
	topTokensExpr := usagePresentationTotalTokensSQL("", usagePresentationFactorSQL("", true))

	query := fmt.Sprintf(`
		WITH top_users AS (
			SELECT user_id
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY user_id
			ORDER BY SUM(%s) DESC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.user_id,
			COALESCE(us.email, '') as email,
			COALESCE(us.username, '') as username,
			COUNT(*) as requests,
			COALESCE(SUM(%s), 0) as tokens,
			COALESCE(SUM(%s), 0) as cost,
			COALESCE(SUM(%s), 0) as actual_cost
		FROM usage_logs u
		LEFT JOIN users us ON u.user_id = us.id
		WHERE u.user_id IN (SELECT user_id FROM top_users)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.user_id, us.email, us.username
		ORDER BY date ASC, tokens DESC
	`, topTokensExpr, dateFormat, totalTokensExpr, totalCostExpr, actualCostExpr)
	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]UserUsageTrendPoint, 0)
	for rows.Next() {
		var row UserUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.UserID, &row.Email, &row.Username, &row.Requests, &row.Tokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (r *usageLogRepository) GetModelStatsWithFiltersBySourceForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, source string, usePresentation bool) (results []ModelStat, err error) {
	if !usePresentation {
		return r.GetModelStatsWithFiltersBySource(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType, source)
	}
	return r.getModelStatsWithFiltersBySourceForView(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, "", requestType, stream, billingType, source, "", true)
}

func (r *usageLogRepository) getModelStatsWithFiltersBySourceForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, source string, billingMode string, usePresentation bool) (results []ModelStat, err error) {
	presentationFactor := usagePresentationFactorSQL("", usePresentation)
	inputTokensExpr := usagePresentationTokenSQL("input_tokens", presentationFactor)
	outputTokensExpr := usagePresentationOutputTokensSQL("", presentationFactor)
	cacheCreationTokensExpr := usagePresentationTokenSQL("cache_creation_tokens", presentationFactor)
	cacheReadTokensExpr := usagePresentationTokenSQL("cache_read_tokens", presentationFactor)
	totalTokensExpr := usagePresentationTotalTokensSQL("", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("total_cost", presentationFactor)
	actualCostExpr := fmt.Sprintf("COALESCE(SUM(%s), 0) as actual_cost", usagePresentationCostSQL("actual_cost", presentationFactor))
	if accountID > 0 && userID == 0 && apiKeyID == 0 {
		actualCostExpr = fmt.Sprintf("COALESCE(SUM(%s), 0) as actual_cost", usagePresentationCostSQL("COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)", presentationFactor))
	}
	accountCostExpr := fmt.Sprintf("COALESCE(SUM(%s), 0) as account_cost", usagePresentationCostSQL("COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)", presentationFactor))
	modelExpr := resolveModelDimensionExpression(source)

	query := fmt.Sprintf(`
		SELECT
			%s as model,
			COUNT(*) as requests,
			COALESCE(SUM(%s), 0) as input_tokens,
			COALESCE(SUM(%s), 0) as output_tokens,
			COALESCE(SUM(%s), 0) as cache_creation_tokens,
			COALESCE(SUM(%s), 0) as cache_read_tokens,
			COALESCE(SUM(%s), 0) as total_tokens,
			COALESCE(SUM(%s), 0) as cost,
			%s,
			%s
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, modelExpr, inputTokensExpr, outputTokensExpr, cacheCreationTokensExpr, cacheReadTokensExpr, totalTokensExpr, totalCostExpr, actualCostExpr, accountCostExpr)
	args := []any{startTime, endTime}
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
	}
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
	}
	if accountID > 0 {
		query += fmt.Sprintf(" AND account_id = $%d", len(args)+1)
		args = append(args, accountID)
	}
	if groupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, groupID)
	}
	if strings.TrimSpace(model) != "" {
		query += fmt.Sprintf(" AND %s = $%d", modelExpr, len(args)+1)
		args = append(args, model)
	}
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
	}
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "")
	query += fmt.Sprintf(" GROUP BY %s ORDER BY total_tokens DESC", modelExpr)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results, err = scanModelStatsRows(rows)
	return results, err
}

func (r *usageLogRepository) GetGroupStatsWithFiltersForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8, usePresentation bool) (results []usagestats.GroupStat, err error) {
	if !usePresentation {
		return r.GetGroupStatsWithFilters(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, requestType, stream, billingType)
	}
	return r.getGroupStatsWithFiltersForView(ctx, startTime, endTime, userID, apiKeyID, accountID, groupID, "", requestType, stream, billingType, "", true)
}

func (r *usageLogRepository) getGroupStatsWithFiltersForView(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8, billingMode string, usePresentation bool) (results []usagestats.GroupStat, err error) {
	presentationFactor := usagePresentationFactorSQL("ul.", usePresentation)
	totalTokensExpr := usagePresentationTotalTokensSQL("ul.", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("ul.total_cost", presentationFactor)
	actualCostExpr := usagePresentationCostSQL("ul.actual_cost", presentationFactor)
	accountCostExpr := usagePresentationCostSQL("COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)", presentationFactor)
	query := `
		SELECT
			COALESCE(ul.group_id, 0) as group_id,
			COALESCE(g.name, '') as group_name,
			COUNT(*) as requests,
			COALESCE(SUM(` + totalTokensExpr + `), 0) as total_tokens,
			COALESCE(SUM(` + totalCostExpr + `), 0) as cost,
			COALESCE(SUM(` + actualCostExpr + `), 0) as actual_cost,
			COALESCE(SUM(` + accountCostExpr + `), 0) as account_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`
	args := []any{startTime, endTime}
	if userID > 0 {
		query += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, userID)
	}
	if apiKeyID > 0 {
		query += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, apiKeyID)
	}
	if accountID > 0 {
		query += fmt.Sprintf(" AND ul.account_id = $%d", len(args)+1)
		args = append(args, accountID)
	}
	if groupID > 0 {
		query += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, groupID)
	}
	if strings.TrimSpace(model) != "" {
		modelExpr := resolveModelDimensionExpressionWithAlias(usagestats.ModelSourceRequested, "ul")
		query += fmt.Sprintf(" AND %s = $%d", modelExpr, len(args)+1)
		args = append(args, model)
	}
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, requestType, stream)
	if billingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, int16(*billingType))
	}
	query, args = appendUsageLogBillingModeQueryFilter(query, args, billingMode, "ul")
	query += " GROUP BY ul.group_id, g.name ORDER BY total_tokens DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]usagestats.GroupStat, 0)
	for rows.Next() {
		var row usagestats.GroupStat
		if err := rows.Scan(&row.GroupID, &row.GroupName, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost, &row.AccountCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (r *usageLogRepository) GetUserBreakdownStatsForView(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int, usePresentation bool) (results []usagestats.UserBreakdownItem, err error) {
	if !usePresentation {
		return r.GetUserBreakdownStats(ctx, startTime, endTime, dim, limit)
	}
	presentationFactor := usagePresentationFactorSQL("ul.", true)
	inputTokensExpr := usagePresentationTokenSQL("ul.input_tokens", presentationFactor)
	outputTokensExpr := usagePresentationOutputTokensSQL("ul.", presentationFactor)
	cacheTokensExpr := usagePresentationTokenSQL("ul.cache_creation_tokens", presentationFactor) + " + " +
		usagePresentationTokenSQL("ul.cache_read_tokens", presentationFactor)
	totalTokensExpr := usagePresentationTotalTokensSQL("ul.", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("ul.total_cost", presentationFactor)
	actualCostExpr := usagePresentationCostSQL("ul.actual_cost", presentationFactor)
	accountCostExpr := usagePresentationCostSQL("COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)", presentationFactor)
	query := `
		SELECT
			COALESCE(ul.user_id, 0) as user_id,
			COALESCE(u.email, '') as email,
			COALESCE(u.notes, '') as notes,
			COUNT(*) as requests,
			COALESCE(SUM(` + inputTokensExpr + `), 0) as input_tokens,
			COALESCE(SUM(` + outputTokensExpr + `), 0) as output_tokens,
			COALESCE(SUM(` + cacheTokensExpr + `), 0) as cache_tokens,
			COALESCE(SUM(` + totalTokensExpr + `), 0) as total_tokens,
			COALESCE(SUM(` + totalCostExpr + `), 0) as cost,
			COALESCE(SUM(` + actualCostExpr + `), 0) as actual_cost,
			COALESCE(SUM(` + accountCostExpr + `), 0) as account_cost
		FROM usage_logs ul
		LEFT JOIN users u ON u.id = ul.user_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`
	args := []any{startTime, endTime}
	if dim.GroupID > 0 {
		query += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, dim.GroupID)
	}
	if dim.Model != "" {
		query += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpression(dim.ModelType), len(args)+1)
		args = append(args, dim.Model)
	}
	if dim.Endpoint != "" {
		query += fmt.Sprintf(" AND %s = $%d", resolveEndpointColumn(dim.EndpointType), len(args)+1)
		args = append(args, dim.Endpoint)
	}
	if dim.UserID > 0 {
		query += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, dim.UserID)
	}
	if dim.APIKeyID > 0 {
		query += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, dim.APIKeyID)
	}
	if dim.AccountID > 0 {
		query += fmt.Sprintf(" AND ul.account_id = $%d", len(args)+1)
		args = append(args, dim.AccountID)
	}
	if dim.RequestType != nil {
		condition, conditionArgs := buildRequestTypeFilterConditionWithAlias(len(args)+1, *dim.RequestType, "ul")
		query += " AND " + condition
		args = append(args, conditionArgs...)
	}
	if dim.Stream != nil {
		query += fmt.Sprintf(" AND ul.stream = $%d", len(args)+1)
		args = append(args, *dim.Stream)
	}
	if dim.BillingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, *dim.BillingType)
	}
	orderBy := "actual_cost"
	switch dim.SortBy {
	case "total_tokens", "input_tokens", "output_tokens", "cache_tokens", "requests", "cost", "actual_cost":
		orderBy = dim.SortBy
	}
	query += " GROUP BY ul.user_id, u.email, u.notes ORDER BY " + orderBy + " DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]usagestats.UserBreakdownItem, 0)
	for rows.Next() {
		var row usagestats.UserBreakdownItem
		if err := rows.Scan(
			&row.UserID,
			&row.Email,
			&row.Notes,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheTokens,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
			&row.AccountCost,
		); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (r *usageLogRepository) GetAllGroupUsageSummaryForView(ctx context.Context, todayStart time.Time, usePresentation bool) ([]usagestats.GroupUsageSummary, error) {
	if !usePresentation {
		return r.GetAllGroupUsageSummary(ctx, todayStart)
	}
	presentationFactor := usagePresentationFactorSQL("ul.", true)
	actualCostExpr := usagePresentationCostSQL("ul.actual_cost", presentationFactor)
	query := fmt.Sprintf(`
		SELECT
			g.id AS group_id,
			COALESCE(SUM(%s), 0) AS total_cost,
			COALESCE(SUM(CASE WHEN ul.created_at >= $1 THEN %s ELSE 0 END), 0) AS today_cost
		FROM groups g
		LEFT JOIN usage_logs ul ON ul.group_id = g.id
		GROUP BY g.id
	`, actualCostExpr, actualCostExpr)
	rows, err := r.sql.QueryContext(ctx, query, todayStart)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var results []usagestats.GroupUsageSummary
	for rows.Next() {
		var row usagestats.GroupUsageSummary
		if err := rows.Scan(&row.GroupID, &row.TotalCost, &row.TodayCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (r *usageLogRepository) GetUserSpendingRankingForView(ctx context.Context, startTime, endTime time.Time, limit int, usePresentation bool) (result *UserSpendingRankingResponse, err error) {
	if !usePresentation {
		return r.GetUserSpendingRanking(ctx, startTime, endTime, limit)
	}
	if limit <= 0 {
		limit = 12
	}
	presentationFactor := usagePresentationFactorSQL("u.", true)
	tokensExpr := usagePresentationTotalTokensSQL("u.", presentationFactor)
	actualCostExpr := usagePresentationCostSQL("u.actual_cost", presentationFactor)
	query := fmt.Sprintf(`
		WITH user_spend AS (
			SELECT
				u.user_id,
				COALESCE(us.email, '') as email,
				COALESCE(SUM(%s), 0) as actual_cost,
				COUNT(*) as requests,
				COALESCE(SUM(%s), 0) as tokens
			FROM usage_logs u
			LEFT JOIN users us ON u.user_id = us.id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.user_id, us.email
		),
		ranked AS (
			SELECT
				user_id,
				email,
				actual_cost,
				requests,
				tokens,
				COALESCE(SUM(actual_cost) OVER (), 0) as total_actual_cost,
				COALESCE(SUM(requests) OVER (), 0) as total_requests,
				COALESCE(SUM(tokens) OVER (), 0) as total_tokens
			FROM user_spend
			ORDER BY actual_cost DESC, tokens DESC, user_id ASC
			LIMIT $3
		)
		SELECT user_id, email, actual_cost, requests, tokens, total_actual_cost, total_requests, total_tokens
		FROM ranked
		ORDER BY actual_cost DESC, tokens DESC, user_id ASC
	`, actualCostExpr, tokensExpr)
	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()
	ranking := make([]UserSpendingRankingItem, 0)
	totalActualCost := 0.0
	totalRequests := int64(0)
	totalTokens := int64(0)
	for rows.Next() {
		var row UserSpendingRankingItem
		if err = rows.Scan(&row.UserID, &row.Email, &row.ActualCost, &row.Requests, &row.Tokens, &totalActualCost, &totalRequests, &totalTokens); err != nil {
			return nil, err
		}
		ranking = append(ranking, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return &UserSpendingRankingResponse{Ranking: ranking, TotalActualCost: totalActualCost, TotalRequests: totalRequests, TotalTokens: totalTokens}, nil
}

func (r *usageLogRepository) GetAccountUsageStatsForView(ctx context.Context, accountID int64, startTime, endTime time.Time, usePresentation bool) (resp *AccountUsageStatsResponse, err error) {
	if !usePresentation {
		return r.GetAccountUsageStats(ctx, accountID, startTime, endTime)
	}
	daysCount := int(endTime.Sub(startTime).Hours()/24) + 1
	if daysCount <= 0 {
		daysCount = 30
	}
	presentationFactor := usagePresentationFactorSQL("", true)
	totalTokensExpr := usagePresentationTotalTokensSQL("", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("total_cost", presentationFactor)
	accountCostExpr := usagePresentationCostSQL("COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)", presentationFactor)
	userCostExpr := usagePresentationCostSQL("actual_cost", presentationFactor)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, 'YYYY-MM-DD') as date,
			COUNT(*) as requests,
			COALESCE(SUM(%s), 0) as tokens,
			COALESCE(SUM(%s), 0) as cost,
			COALESCE(SUM(%s), 0) as actual_cost,
			COALESCE(SUM(%s), 0) as user_cost
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY date
		ORDER BY date ASC
	`, totalTokensExpr, totalCostExpr, accountCostExpr, userCostExpr)
	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			resp = nil
		}
	}()
	history := make([]AccountUsageHistory, 0)
	for rows.Next() {
		var date string
		var requests int64
		var tokens int64
		var cost float64
		var actualCost float64
		var userCost float64
		if err = rows.Scan(&date, &requests, &tokens, &cost, &actualCost, &userCost); err != nil {
			return nil, err
		}
		t, _ := time.Parse("2006-01-02", date)
		history = append(history, AccountUsageHistory{Date: date, Label: t.Format("01/02"), Requests: requests, Tokens: tokens, Cost: cost, ActualCost: actualCost, UserCost: userCost})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	resp, err = r.buildAccountUsageStatsResponse(ctx, accountID, startTime, endTime, daysCount, history, true)
	return resp, err
}

func (r *usageLogRepository) buildAccountUsageStatsResponse(ctx context.Context, accountID int64, startTime, endTime time.Time, daysCount int, history []AccountUsageHistory, usePresentation bool) (*AccountUsageStatsResponse, error) {
	var totalAccountCost, totalUserCost, totalStandardCost float64
	var totalRequests, totalTokens int64
	var highestCostDay, highestRequestDay *AccountUsageHistory
	for i := range history {
		h := &history[i]
		totalAccountCost += h.ActualCost
		totalUserCost += h.UserCost
		totalStandardCost += h.Cost
		totalRequests += h.Requests
		totalTokens += h.Tokens
		if highestCostDay == nil || h.ActualCost > highestCostDay.ActualCost {
			highestCostDay = h
		}
		if highestRequestDay == nil || h.Requests > highestRequestDay.Requests {
			highestRequestDay = h
		}
	}
	actualDaysUsed := len(history)
	if actualDaysUsed == 0 {
		actualDaysUsed = 1
	}
	avgQuery := "SELECT COALESCE(AVG(duration_ms), 0) as avg_duration_ms FROM usage_logs WHERE account_id = $1 AND created_at >= $2 AND created_at < $3"
	var avgDuration float64
	if err := scanSingleRow(ctx, r.sql, avgQuery, []any{accountID, startTime, endTime}, &avgDuration); err != nil {
		return nil, err
	}
	summary := AccountUsageSummary{
		Days:              daysCount,
		ActualDaysUsed:    actualDaysUsed,
		TotalCost:         totalAccountCost,
		TotalUserCost:     totalUserCost,
		TotalStandardCost: totalStandardCost,
		TotalRequests:     totalRequests,
		TotalTokens:       totalTokens,
		AvgDailyCost:      totalAccountCost / float64(actualDaysUsed),
		AvgDailyUserCost:  totalUserCost / float64(actualDaysUsed),
		AvgDailyRequests:  float64(totalRequests) / float64(actualDaysUsed),
		AvgDailyTokens:    float64(totalTokens) / float64(actualDaysUsed),
		AvgDurationMs:     avgDuration,
	}
	todayStr := timezone.Now().Format("2006-01-02")
	for i := range history {
		if history[i].Date == todayStr {
			summary.Today = &struct {
				Date     string  `json:"date"`
				Cost     float64 `json:"cost"`
				UserCost float64 `json:"user_cost"`
				Requests int64   `json:"requests"`
				Tokens   int64   `json:"tokens"`
			}{Date: history[i].Date, Cost: history[i].ActualCost, UserCost: history[i].UserCost, Requests: history[i].Requests, Tokens: history[i].Tokens}
			break
		}
	}
	if highestCostDay != nil {
		summary.HighestCostDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
			Requests int64   `json:"requests"`
		}{Date: highestCostDay.Date, Label: highestCostDay.Label, Cost: highestCostDay.ActualCost, UserCost: highestCostDay.UserCost, Requests: highestCostDay.Requests}
	}
	if highestRequestDay != nil {
		summary.HighestRequestDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Requests int64   `json:"requests"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
		}{Date: highestRequestDay.Date, Label: highestRequestDay.Label, Requests: highestRequestDay.Requests, Cost: highestRequestDay.ActualCost, UserCost: highestRequestDay.UserCost}
	}
	models, err := r.GetModelStatsWithFiltersBySourceForView(ctx, startTime, endTime, 0, 0, accountID, 0, nil, nil, nil, usagestats.ModelSourceRequested, usePresentation)
	if err != nil {
		models = []ModelStat{}
	}
	endpoints, endpointErr := r.getEndpointStatsByColumnWithFilters(ctx, "inbound_endpoint", startTime, endTime, 0, 0, accountID, 0, "", "", nil, nil, nil, "")
	if endpointErr != nil {
		logger.LegacyPrintf("repository.usage_log", "GetEndpointStatsWithFilters failed in GetAccountUsageStats: %v", endpointErr)
		endpoints = []EndpointStat{}
	}
	upstreamEndpoints, upstreamEndpointErr := r.getEndpointStatsByColumnWithFilters(ctx, "upstream_endpoint", startTime, endTime, 0, 0, accountID, 0, "", "", nil, nil, nil, "")
	if upstreamEndpointErr != nil {
		logger.LegacyPrintf("repository.usage_log", "GetUpstreamEndpointStatsWithFilters failed in GetAccountUsageStats: %v", upstreamEndpointErr)
		upstreamEndpoints = []EndpointStat{}
	}
	return &AccountUsageStatsResponse{Summary: summary, History: history, Models: models, Endpoints: endpoints, UpstreamEndpoints: upstreamEndpoints}, nil
}

func (r *usageLogRepository) getPerformanceStatsForView(ctx context.Context, userID int64, usePresentation bool) (rpm, tpm int64, err error) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	presentationFactor := usagePresentationFactorSQL("", usePresentation)
	inputTokensExpr := usagePresentationTokenSQL("input_tokens", presentationFactor)
	outputTokensExpr := usagePresentationOutputTokensSQL("", presentationFactor)
	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as request_count,
			COALESCE(SUM(%s + %s), 0) as token_count
		FROM usage_logs
		WHERE created_at >= $1`, inputTokensExpr, outputTokensExpr)
	args := []any{fiveMinutesAgo}
	if userID > 0 {
		query += " AND user_id = $2"
		args = append(args, userID)
	}
	var requestCount int64
	var tokenCount int64
	if err := scanSingleRow(ctx, r.sql, query, args, &requestCount, &tokenCount); err != nil {
		return 0, 0, err
	}
	return requestCount / 5, tokenCount / 5, nil
}

func (r *usageLogRepository) getPerformanceStatsByAPIKeyForView(ctx context.Context, apiKeyID int64, usePresentation bool) (rpm, tpm int64, err error) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	presentationFactor := usagePresentationFactorSQL("", usePresentation)
	totalTokensExpr := usagePresentationTotalTokensSQL("", presentationFactor)
	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as request_count,
			COALESCE(SUM(%s), 0) as token_count
		FROM usage_logs
		WHERE created_at >= $1 AND api_key_id = $2`, totalTokensExpr)
	var requestCount int64
	var tokenCount int64
	if err := scanSingleRow(ctx, r.sql, query, []any{fiveMinutesAgo, apiKeyID}, &requestCount, &tokenCount); err != nil {
		return 0, 0, err
	}
	return requestCount / 5, tokenCount / 5, nil
}

func (r *usageLogRepository) fillDashboardUsageStatsFromUsageLogsForView(ctx context.Context, stats *DashboardStats, startUTC, endUTC, todayUTC, now time.Time, usePresentation bool) error {
	todayEnd := todayUTC.Add(24 * time.Hour)
	presentationFactor := usagePresentationFactorSQL("", usePresentation)
	inputTokensExpr := usagePresentationTokenSQL("input_tokens", presentationFactor)
	outputTokensExpr := usagePresentationOutputTokensSQL("", presentationFactor)
	cacheCreationTokensExpr := usagePresentationTokenSQL("cache_creation_tokens", presentationFactor)
	cacheReadTokensExpr := usagePresentationTokenSQL("cache_read_tokens", presentationFactor)
	totalCostExpr := usagePresentationCostSQL("total_cost", presentationFactor)
	actualCostExpr := usagePresentationCostSQL("actual_cost", presentationFactor)
	accountCostExpr := usagePresentationCostSQL("COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)", presentationFactor)
	combinedStatsQuery := fmt.Sprintf(`
		WITH scoped AS (
			SELECT
				created_at,
				%s AS input_tokens,
				%s AS output_tokens,
				%s AS cache_creation_tokens,
				%s AS cache_read_tokens,
				%s AS total_cost,
				%s AS actual_cost,
				%s AS account_cost,
				COALESCE(duration_ms, 0) AS duration_ms
			FROM usage_logs
			WHERE created_at >= LEAST($1::timestamptz, $3::timestamptz)
				AND created_at < GREATEST($2::timestamptz, $4::timestamptz)
		)
		SELECT
			COUNT(*) FILTER (WHERE created_at >= $1 AND created_at < $2) AS total_requests,
			COALESCE(SUM(input_tokens) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_input_tokens,
			COALESCE(SUM(output_tokens) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_output_tokens,
			COALESCE(SUM(cache_creation_tokens) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_cache_read_tokens,
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_actual_cost,
			COALESCE(SUM(account_cost) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_account_cost,
			COALESCE(SUM(duration_ms) FILTER (WHERE created_at >= $1 AND created_at < $2), 0) AS total_duration_ms,
			COUNT(*) FILTER (WHERE created_at >= $3 AND created_at < $4) AS today_requests,
			COALESCE(SUM(input_tokens) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_input_tokens,
			COALESCE(SUM(output_tokens) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_output_tokens,
			COALESCE(SUM(cache_creation_tokens) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_cache_read_tokens,
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_cost,
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_actual_cost,
			COALESCE(SUM(account_cost) FILTER (WHERE created_at >= $3 AND created_at < $4), 0) AS today_account_cost
		FROM scoped
	`, inputTokensExpr, outputTokensExpr, cacheCreationTokensExpr, cacheReadTokensExpr, totalCostExpr, actualCostExpr, accountCostExpr)
	hourStart := now.In(timezone.Location()).Truncate(time.Hour).UTC()
	hourEnd := hourStart.Add(time.Hour)
	var totalDurationMs int64
	if err := scanSingleRow(ctx, r.sql, combinedStatsQuery, []any{startUTC, endUTC, todayUTC, todayEnd},
		&stats.TotalRequests, &stats.TotalInputTokens, &stats.TotalOutputTokens, &stats.TotalCacheCreationTokens, &stats.TotalCacheReadTokens,
		&stats.TotalCost, &stats.TotalActualCost, &stats.TotalAccountCost, &totalDurationMs,
		&stats.TodayRequests, &stats.TodayInputTokens, &stats.TodayOutputTokens, &stats.TodayCacheCreationTokens, &stats.TodayCacheReadTokens,
		&stats.TodayCost, &stats.TodayActualCost, &stats.TodayAccountCost,
	); err != nil {
		return err
	}
	activeUsersQuery := `
		SELECT
			COUNT(DISTINCT CASE WHEN created_at >= $1 AND created_at < $2 THEN user_id END) AS active_users,
			COUNT(DISTINCT CASE WHEN created_at >= $3 AND created_at < $4 THEN user_id END) AS hourly_active_users
		FROM usage_logs
	`
	if err := scanSingleRow(ctx, r.sql, activeUsersQuery, []any{todayUTC, todayEnd, hourStart, hourEnd}, &stats.ActiveUsers, &stats.HourlyActiveUsers); err != nil {
		return err
	}
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs = float64(totalDurationMs) / float64(stats.TotalRequests)
	}
	return nil
}

func usagePresentationFactorSQL(prefix string, usePresentation bool) string {
	if !usePresentation {
		return "1"
	}
	return fmt.Sprintf("COALESCE(NULLIF(%spresentation_multiplier, 0), 1)", prefix)
}

func usagePresentationTokenSQL(column, factor string) string {
	if factor == "1" {
		return column
	}
	return fmt.Sprintf("FLOOR(%s * %s)::BIGINT", column, factor)
}

func usagePresentationOutputTokensSQL(prefix, factor string) string {
	return usagePresentationTokenSQL(fmt.Sprintf("GREATEST(%soutput_tokens, %simage_output_tokens)", prefix, prefix), factor)
}

func usagePresentationCostSQL(expr, factor string) string {
	if factor == "1" {
		return expr
	}
	return fmt.Sprintf("%s * %s", expr, factor)
}

func usagePresentationTotalTokensSQL(prefix, factor string) string {
	return strings.Join([]string{
		usagePresentationTokenSQL(prefix+"input_tokens", factor),
		usagePresentationOutputTokensSQL(prefix, factor),
		usagePresentationTokenSQL(prefix+"cache_creation_tokens", factor),
		usagePresentationTokenSQL(prefix+"cache_read_tokens", factor),
	}, " + ")
}
