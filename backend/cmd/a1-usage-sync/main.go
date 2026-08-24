package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const (
	defaultConfigPath = "/opt/a1-user-sync/config.yaml"
	batchSize         = 2000
)

type config struct {
	sourceDB string
	targetDB string
	userIDs  []int64
}

type user struct {
	id    int64
	email string
}

type group struct {
	id       int64
	name     string
	platform string
}

type account struct {
	id        int64
	name      string
	platform  string
	typ       string
	identity  string
	status    string
	createdAt time.Time
}

type apiKey struct {
	id        int64
	userID    int64
	key       string
	name      string
	groupID   sql.NullInt64
	status    string
	createdAt time.Time
	updatedAt time.Time
	deletedAt sql.NullTime
}

type subscription struct {
	id        int64
	userID    int64
	groupID   int64
	startsAt  time.Time
	expiresAt sql.NullTime
}

type accountCandidate struct {
	account  account
	nameFit  bool
	distance time.Duration
}

type stats struct {
	logsInserted  int64
	logsExisting  int64
	logsSkipped   int64
	billingAdded  int64
	billingExists int64
}

func main() {
	var (
		configPath = flag.String("config", defaultConfigPath, "sync config path")
		mapPath    = flag.String("account-map", "/opt/a1-user-sync/usage-account-map.csv", "optional source account to target account overrides")
		dryRun     = flag.Bool("dry-run", false, "validate mappings without writing")
	)
	flag.Parse()

	ctx := context.Background()
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(cfg.userIDs) == 0 {
		log.Println("usage sync skipped: no user_ids configured")
		return
	}

	source, err := sql.Open("postgres", dsn(cfg.sourceDB))
	if err != nil {
		log.Fatal(err)
	}
	defer source.Close()
	target, err := sql.Open("postgres", dsn(cfg.targetDB))
	if err != nil {
		log.Fatal(err)
	}
	defer target.Close()

	if err := source.PingContext(ctx); err != nil {
		log.Fatalf("source database unavailable: %v", err)
	}
	if err := target.PingContext(ctx); err != nil {
		log.Fatalf("target database unavailable: %v", err)
	}

	sourceUsers, err := loadUsers(ctx, source, cfg.userIDs)
	if err != nil {
		log.Fatalf("load source users: %v", err)
	}
	targetUsers, err := loadUsersByEmail(ctx, target)
	if err != nil {
		log.Fatalf("load target users: %v", err)
	}
	userMap, err := mapUsers(sourceUsers, targetUsers)
	if err != nil {
		log.Fatal(err)
	}

	sourceGroups, err := loadGroups(ctx, source)
	if err != nil {
		log.Fatalf("load source groups: %v", err)
	}
	targetGroups, err := loadGroups(ctx, target)
	if err != nil {
		log.Fatalf("load target groups: %v", err)
	}
	groupMap, err := mapGroups(sourceGroups, targetGroups)
	if err != nil {
		log.Fatal(err)
	}

	sourceAccounts, err := loadAccounts(ctx, source)
	if err != nil {
		log.Fatalf("load source accounts: %v", err)
	}
	targetAccounts, err := loadAccounts(ctx, target)
	if err != nil {
		log.Fatalf("load target accounts: %v", err)
	}
	accountOverrides, err := loadAccountOverrides(*mapPath)
	if err != nil {
		log.Fatal(err)
	}
	accountMap, err := mapAccounts(sourceAccounts, targetAccounts, accountOverrides)
	if err != nil {
		log.Fatal(err)
	}

	sourceKeys, err := loadAPIKeys(ctx, source, cfg.userIDs)
	if err != nil {
		log.Fatalf("load source api keys: %v", err)
	}
	targetKeys, err := loadAPIKeys(ctx, target, nil)
	if err != nil {
		log.Fatalf("load target api keys: %v", err)
	}
	keyMap, err := ensureAPIKeys(ctx, target, sourceKeys, targetKeys, userMap, groupMap, *dryRun)
	if err != nil {
		log.Fatal(err)
	}

	sourceSubs, err := loadSubscriptions(ctx, source, cfg.userIDs)
	if err != nil {
		log.Fatalf("load source subscriptions: %v", err)
	}
	targetSubs, err := loadSubscriptions(ctx, target, nil)
	if err != nil {
		log.Fatalf("load target subscriptions: %v", err)
	}
	subMap := mapSubscriptions(sourceSubs, targetSubs, userMap, groupMap)

	if !*dryRun {
		if err := ensureSyncTables(ctx, target); err != nil {
			log.Fatalf("ensure usage sync tables: %v", err)
		}
	}
	logMap, initialState, err := loadLogState(ctx, target, *dryRun)
	if err != nil {
		log.Fatalf("load usage sync state: %v", err)
	}
	result, err := syncUsageLogs(ctx, source, target, cfg.userIDs, userMap, keyMap, accountMap, groupMap, subMap, logMap, initialState, *dryRun)
	if err != nil {
		log.Fatal(err)
	}
	if !*dryRun {
		if err := syncBillingEntries(ctx, source, target, cfg.userIDs, userMap, keyMap, subMap, logMap, result); err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("usage sync finished: users=%d source_accounts=%d source_api_keys=%d source_subscriptions=%d logs_inserted=%d logs_existing=%d logs_skipped=%d billing_added=%d billing_existing=%d dry_run=%t", len(userMap), len(accountMap), len(sourceKeys), len(sourceSubs), result.logsInserted, result.logsExisting, result.logsSkipped, result.billingAdded, result.billingExists, *dryRun)
}

func loadConfig(path string) (config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	text := string(b)
	idsMatch := regexp.MustCompile(`(?m)^user_ids:\s*\[([^\]]*)\]`).FindStringSubmatch(text)
	if len(idsMatch) != 2 {
		return config{}, errors.New("config has no user_ids")
	}
	var ids []int64
	for _, item := range strings.Split(idsMatch[1], ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return config{}, fmt.Errorf("invalid user id %q: %w", item, err)
		}
		ids = append(ids, id)
	}
	return config{
		sourceDB: yamlScalar(text, "source_database", "dbname", "sub2api_ap1"),
		targetDB: yamlScalar(text, "target_database", "dbname", "sub2api"),
		userIDs:  ids,
	}, nil
}

func yamlScalar(text, section, key, fallback string) string {
	sectionRe := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(section) + `:\s*\n(.*?)(?:^\S|\z)`)
	match := sectionRe.FindStringSubmatch(text)
	if len(match) != 2 {
		return fallback
	}
	keyRe := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(key) + `:\s*["']?([^"'\s]+)`)
	value := keyRe.FindStringSubmatch(match[1])
	if len(value) != 2 {
		return fallback
	}
	return value[1]
}

func dsn(database string) string {
	host := envOr("POSTGRES_HOST", "sub2api-postgres")
	port := envOr("POSTGRES_PORT", "5432")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	return (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
		Path:   "/" + database,
		RawQuery: url.Values{
			"sslmode":  {"disable"},
			"TimeZone": {"Asia/Shanghai"},
		}.Encode(),
	}).String()
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadUsers(ctx context.Context, db *sql.DB, ids []int64) (map[int64]user, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, email FROM users WHERE id = ANY($1) ORDER BY id`, pqArray(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]user)
	for rows.Next() {
		var item user
		if err := rows.Scan(&item.id, &item.email); err != nil {
			return nil, err
		}
		result[item.id] = item
	}
	return result, rows.Err()
}

func loadUsersByEmail(ctx context.Context, db *sql.DB) (map[string][]user, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, email FROM users WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]user)
	for rows.Next() {
		var item user
		if err := rows.Scan(&item.id, &item.email); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(item.email))
		result[key] = append(result[key], item)
	}
	return result, rows.Err()
}

func mapUsers(source map[int64]user, target map[string][]user) (map[int64]int64, error) {
	result := make(map[int64]int64, len(source))
	for sourceID, item := range source {
		matches := target[strings.ToLower(strings.TrimSpace(item.email))]
		if len(matches) != 1 {
			return nil, fmt.Errorf("source user %d (%q) maps to %d target users", sourceID, item.email, len(matches))
		}
		result[sourceID] = matches[0].id
	}
	return result, nil
}

func loadGroups(ctx context.Context, db *sql.DB) (map[string]group, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, platform FROM groups WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]group)
	for rows.Next() {
		var item group
		if err := rows.Scan(&item.id, &item.name, &item.platform); err != nil {
			return nil, err
		}
		key := groupKey(item.name, item.platform)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate target group %q", key)
		}
		result[key] = item
	}
	return result, rows.Err()
}

func mapGroups(source, target map[string]group) (map[int64]int64, error) {
	result := make(map[int64]int64, len(source))
	for key, item := range source {
		mapped, ok := target[key]
		if !ok {
			// Usage rows can still be attributed to their user and upstream
			// account when a newly-created A1 group has not reached primary yet.
			// Keep the group column NULL instead of dropping the whole sync round.
			log.Printf("usage sync warning: source group %d (%q/%q) has no target mapping; usage group_id will be NULL", item.id, item.name, item.platform)
			continue
		}
		result[item.id] = mapped.id
	}
	return result, nil
}

func groupKey(name, platform string) string {
	return strings.ToLower(strings.TrimSpace(platform)) + "\x00" + name
}

func loadAccounts(ctx context.Context, db *sql.DB) ([]account, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, platform, type, status, created_at,
		       COALESCE(NULLIF(credentials->>'chatgpt_account_id', ''),
		                NULLIF(credentials->>'account_id', ''),
		                NULLIF(credentials->>'email', ''),
		                NULLIF(credentials->>'api_key', ''), '')
		FROM accounts
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []account
	for rows.Next() {
		var item account
		if err := rows.Scan(&item.id, &item.name, &item.platform, &item.typ, &item.status, &item.createdAt, &item.identity); err != nil {
			return nil, err
		}
		item.identity = stableIdentity(item.identity)
		result = append(result, item)
	}
	return result, rows.Err()
}

func stableIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func loadAccountOverrides(path string) (map[int64]int64, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[int64]int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make(map[int64]int64)
	for lineNo, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d must contain source_id,target_id", path, lineNo+1)
		}
		sourceID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s:%d invalid source id: %w", path, lineNo+1, err)
		}
		targetID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s:%d invalid target id: %w", path, lineNo+1, err)
		}
		result[sourceID] = targetID
	}
	return result, nil
}

func mapAccounts(source, target []account, overrides map[int64]int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(source))
	for _, sourceItem := range source {
		if targetID, ok := overrides[sourceItem.id]; ok {
			result[sourceItem.id] = targetID
			continue
		}
		var candidates []accountCandidate
		for _, targetItem := range target {
			if sourceItem.platform != targetItem.platform || sourceItem.typ != targetItem.typ {
				continue
			}
			if sourceItem.identity != "" && sourceItem.identity != targetItem.identity {
				continue
			}
			if sourceItem.identity == "" && sourceItem.name != targetItem.name {
				continue
			}
			candidates = append(candidates, accountCandidate{
				account:  targetItem,
				nameFit:  sourceItem.name == targetItem.name,
				distance: absDuration(sourceItem.createdAt.Sub(targetItem.createdAt)),
			})
		}
		if len(candidates) == 0 {
			// Soft-deleted accounts may have changed credential metadata while the
			// historical usage rows still reference them. Fall back to the stable
			// account identity used by the operator: platform, type, name and
			// closest creation time.
			for _, targetItem := range target {
				if sourceItem.platform != targetItem.platform || sourceItem.typ != targetItem.typ || sourceItem.name != targetItem.name {
					continue
				}
				candidates = append(candidates, accountCandidate{
					account:  targetItem,
					nameFit:  true,
					distance: absDuration(sourceItem.createdAt.Sub(targetItem.createdAt)),
				})
			}
		}
		if len(candidates) == 0 {
			// The account may be unused by the configured users. If it appears in
			// a usage row, remapUsageParams will fail explicitly instead of
			// silently attributing that usage to another account.
			log.Printf("usage sync warning: source account %d (%q) has no target mapping; it cannot be used by synced usage rows", sourceItem.id, sourceItem.name)
			continue
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].nameFit != candidates[j].nameFit {
				return candidates[i].nameFit
			}
			if candidates[i].distance != candidates[j].distance {
				return candidates[i].distance < candidates[j].distance
			}
			return candidates[i].account.id < candidates[j].account.id
		})
		result[sourceItem.id] = candidates[0].account.id
	}
	return result, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func loadAPIKeys(ctx context.Context, db *sql.DB, userIDs []int64) (map[int64]apiKey, error) {
	query := `SELECT id, user_id, key, name, group_id, status, created_at, updated_at, deleted_at FROM api_keys`
	args := []any{}
	if len(userIDs) > 0 {
		query += ` WHERE user_id = ANY($1)`
		args = append(args, pqArray(userIDs))
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]apiKey)
	for rows.Next() {
		var item apiKey
		if err := rows.Scan(&item.id, &item.userID, &item.key, &item.name, &item.groupID, &item.status, &item.createdAt, &item.updatedAt, &item.deletedAt); err != nil {
			return nil, err
		}
		result[item.id] = item
	}
	return result, rows.Err()
}

func ensureAPIKeys(ctx context.Context, target *sql.DB, source, targetKeys map[int64]apiKey, userMap, groupMap map[int64]int64, dryRun bool) (map[int64]int64, error) {
	byKey := make(map[string]apiKey, len(targetKeys))
	for _, item := range targetKeys {
		byKey[item.key] = item
	}
	result := make(map[int64]int64, len(source))
	for sourceID, item := range source {
		mappedUser, ok := userMap[item.userID]
		if !ok {
			return nil, fmt.Errorf("api key %d belongs to unmapped source user %d", sourceID, item.userID)
		}
		if existing, ok := byKey[item.key]; ok {
			if existing.userID != mappedUser {
				return nil, fmt.Errorf("api key %d already belongs to another target user", sourceID)
			}
			result[sourceID] = existing.id
			continue
		}
		if dryRun {
			// No target row is created during validation. A stable placeholder is
			// enough to validate the remaining foreign-key mappings; the write run
			// replaces it with the actual target API key ID.
			result[sourceID] = sourceID
			continue
		}
		var targetGroup any
		if item.groupID.Valid {
			mappedGroup, ok := groupMap[item.groupID.Int64]
			if ok {
				targetGroup = mappedGroup
			} else {
				log.Printf("usage sync warning: source API key %d group %d has no target mapping; target group_id will be NULL", sourceID, item.groupID.Int64)
			}
		}
		var targetID int64
		err := target.QueryRowContext(ctx, `
			INSERT INTO api_keys (user_id, key, name, group_id, status, created_at, updated_at, deleted_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (key) DO NOTHING
			RETURNING id`, mappedUser, item.key, item.name, targetGroup, item.status, item.createdAt, item.updatedAt, nullTimeValue(item.deletedAt)).Scan(&targetID)
		if errors.Is(err, sql.ErrNoRows) {
			if err := target.QueryRowContext(ctx, `SELECT id FROM api_keys WHERE key=$1`, item.key).Scan(&targetID); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, fmt.Errorf("insert api key %d: %w", sourceID, err)
		}
		result[sourceID] = targetID
	}
	return result, nil
}

func loadSubscriptions(ctx context.Context, db *sql.DB, userIDs []int64) (map[int64]subscription, error) {
	query := `SELECT id, user_id, group_id, starts_at, expires_at FROM user_subscriptions`
	args := []any{}
	if len(userIDs) > 0 {
		query += ` WHERE user_id = ANY($1)`
		args = append(args, pqArray(userIDs))
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]subscription)
	for rows.Next() {
		var item subscription
		if err := rows.Scan(&item.id, &item.userID, &item.groupID, &item.startsAt, &item.expiresAt); err != nil {
			return nil, err
		}
		result[item.id] = item
	}
	return result, rows.Err()
}

func mapSubscriptions(source, target map[int64]subscription, userMap, groupMap map[int64]int64) map[int64]int64 {
	byKey := make(map[string]int64, len(target))
	for _, item := range target {
		byKey[subscriptionKey(item.userID, item.groupID, item.startsAt, item.expiresAt)] = item.id
	}
	result := make(map[int64]int64, len(source))
	for sourceID, item := range source {
		mappedUser, userOK := userMap[item.userID]
		mappedGroup, groupOK := groupMap[item.groupID]
		if !userOK || !groupOK {
			continue
		}
		if targetID, ok := byKey[subscriptionKey(mappedUser, mappedGroup, item.startsAt, item.expiresAt)]; ok {
			result[sourceID] = targetID
		}
	}
	return result
}

func subscriptionKey(userID, groupID int64, startsAt time.Time, expiresAt sql.NullTime) string {
	expires := ""
	if expiresAt.Valid {
		expires = expiresAt.Time.UTC().Format(time.RFC3339Nano)
	}
	return fmt.Sprintf("%d:%d:%s:%s", userID, groupID, startsAt.UTC().Format(time.RFC3339Nano), expires)
}

func ensureSyncTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS a1_usage_log_sync (
			source_id BIGINT PRIMARY KEY,
			target_id BIGINT NOT NULL,
			synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS a1_billing_usage_entry_sync (
			source_id BIGINT PRIMARY KEY,
			target_id BIGINT NOT NULL,
			synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`)
	return err
}

func loadLogState(ctx context.Context, db *sql.DB, dryRun bool) (map[int64]int64, map[int64]int64, error) {
	result := make(map[int64]int64)
	if dryRun {
		return result, result, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT source_id, target_id FROM a1_usage_log_sync`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sourceID, targetID int64
		if err := rows.Scan(&sourceID, &targetID); err != nil {
			return nil, nil, err
		}
		result[sourceID] = targetID
	}
	return result, result, rows.Err()
}

func syncUsageLogs(ctx context.Context, source, target *sql.DB, userIDs []int64, userMap, keyMap, accountMap, groupMap, subMap map[int64]int64, state, initial map[int64]int64, dryRun bool) (stats, error) {
	cols, err := usageLogColumns(ctx, target)
	if err != nil {
		return stats{}, err
	}
	query := `SELECT id, ` + strings.Join(quoteIdentifiers(cols), ", ") + ` FROM usage_logs WHERE user_id = ANY($1) ORDER BY id`
	rows, err := source.QueryContext(ctx, query, pqArray(userIDs))
	if err != nil {
		return stats{}, err
	}
	defer rows.Close()

	insertSQL := `INSERT INTO usage_logs (` + strings.Join(quoteIdentifiers(cols), ", ") + `) VALUES (` + placeholders(len(cols)) + `) ON CONFLICT (request_id, api_key_id) DO NOTHING RETURNING id`
	indexes := make(map[string]int, len(cols))
	for i, col := range cols {
		indexes[col] = i
	}
	result := stats{}
	batch := 0
	var tx *sql.Tx
	var insertStmt *sql.Stmt
	var stateStmt *sql.Stmt
	beginBatch := func() error {
		if dryRun {
			return nil
		}
		var err error
		tx, err = target.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		insertStmt, err = tx.PrepareContext(ctx, insertSQL)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		stateStmt, err = tx.PrepareContext(ctx, `INSERT INTO a1_usage_log_sync (source_id,target_id) VALUES ($1,$2) ON CONFLICT (source_id) DO NOTHING`)
		if err != nil {
			_ = insertStmt.Close()
			_ = tx.Rollback()
			return err
		}
		return nil
	}
	finishBatch := func() error {
		if dryRun || tx == nil {
			return nil
		}
		if err := insertStmt.Close(); err != nil {
			_ = stateStmt.Close()
			_ = tx.Rollback()
			return err
		}
		if err := stateStmt.Close(); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		tx = nil
		return nil
	}
	if err := beginBatch(); err != nil {
		return stats{}, err
	}

	for rows.Next() {
		var sourceID int64
		raw := make([]sql.RawBytes, len(cols))
		dest := make([]any, len(cols)+1)
		dest[0] = &sourceID
		for i := range raw {
			dest[i+1] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			if tx != nil {
				_ = tx.Rollback()
			}
			return result, err
		}
		if _, ok := state[sourceID]; ok && !dryRun {
			result.logsSkipped++
			continue
		}
		params := make([]any, len(cols))
		for i, value := range raw {
			if value != nil {
				params[i] = string(value)
			}
		}
		if err := remapUsageParams(params, indexes, userMap, keyMap, accountMap, groupMap, subMap); err != nil {
			if tx != nil {
				_ = tx.Rollback()
			}
			return result, fmt.Errorf("source usage log %d: %w", sourceID, err)
		}
		if dryRun {
			result.logsSkipped++
			continue
		}
		var targetID int64
		err := insertStmt.QueryRowContext(ctx, params...).Scan(&targetID)
		if errors.Is(err, sql.ErrNoRows) {
			requestID, _ := params[indexes["request_id"]].(string)
			apiKeyID, err := asInt64(params[indexes["api_key_id"]])
			if err != nil {
				_ = tx.Rollback()
				return result, err
			}
			if err := tx.QueryRowContext(ctx, `SELECT id FROM usage_logs WHERE request_id=$1 AND api_key_id=$2`, requestID, apiKeyID).Scan(&targetID); err != nil {
				_ = tx.Rollback()
				return result, err
			}
			result.logsExisting++
		} else if err != nil {
			_ = tx.Rollback()
			return result, fmt.Errorf("insert usage log %d: %w", sourceID, err)
		} else {
			result.logsInserted++
		}
		state[sourceID] = targetID
		initial[sourceID] = targetID
		if _, err := stateStmt.ExecContext(ctx, sourceID, targetID); err != nil {
			_ = tx.Rollback()
			return result, err
		}
		batch++
		if batch >= batchSize {
			if err := finishBatch(); err != nil {
				return result, err
			}
			batch = 0
			if err := beginBatch(); err != nil {
				return result, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		if tx != nil {
			_ = tx.Rollback()
		}
		return result, err
	}
	if err := finishBatch(); err != nil {
		return result, err
	}
	return result, nil
}

func usageLogColumns(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name='usage_logs' AND column_name <> 'id' ORDER BY ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		result = append(result, column)
	}
	return result, rows.Err()
}

func remapUsageParams(params []any, indexes map[string]int, userMap, keyMap, accountMap, groupMap, subMap map[int64]int64) error {
	remap := func(column string, mapping map[int64]int64, required bool) error {
		index, ok := indexes[column]
		if !ok || params[index] == nil {
			return nil
		}
		sourceID, err := asInt64(params[index])
		if err != nil {
			return fmt.Errorf("invalid %s: %w", column, err)
		}
		targetID, ok := mapping[sourceID]
		if !ok {
			if required {
				return fmt.Errorf("no target mapping for %s %d", column, sourceID)
			}
			params[index] = nil
			return nil
		}
		params[index] = targetID
		return nil
	}
	if err := remap("user_id", userMap, true); err != nil {
		return err
	}
	if err := remap("api_key_id", keyMap, true); err != nil {
		return err
	}
	if err := remap("account_id", accountMap, true); err != nil {
		return err
	}
	if err := remap("group_id", groupMap, false); err != nil {
		return err
	}
	return remap("subscription_id", subMap, false)
}

func syncBillingEntries(ctx context.Context, source, target *sql.DB, userIDs []int64, userMap, keyMap, subMap, logMap map[int64]int64, result stats) error {
	rows, err := source.QueryContext(ctx, `SELECT id, usage_log_id, user_id, api_key_id, subscription_id, billing_type, applied, delta_usd, created_at FROM billing_usage_entries WHERE user_id = ANY($1) ORDER BY id`, pqArray(userIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	stateRows, err := target.QueryContext(ctx, `SELECT source_id FROM a1_billing_usage_entry_sync`)
	if err != nil {
		return err
	}
	known := make(map[int64]struct{})
	for stateRows.Next() {
		var sourceID int64
		if err := stateRows.Scan(&sourceID); err != nil {
			_ = stateRows.Close()
			return err
		}
		known[sourceID] = struct{}{}
	}
	if err := stateRows.Close(); err != nil {
		return err
	}
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	insertEntry, err := tx.PrepareContext(ctx, `INSERT INTO billing_usage_entries (usage_log_id,user_id,api_key_id,subscription_id,billing_type,applied,delta_usd,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`)
	if err != nil {
		return err
	}
	defer insertEntry.Close()
	insertState, err := tx.PrepareContext(ctx, `INSERT INTO a1_billing_usage_entry_sync (source_id,target_id) VALUES ($1,$2) ON CONFLICT (source_id) DO NOTHING`)
	if err != nil {
		return err
	}
	defer insertState.Close()

	for rows.Next() {
		var sourceID, sourceLogID, sourceUserID, sourceKeyID int64
		var sourceSubID sql.NullInt64
		var billingType int16
		var applied bool
		var delta float64
		var createdAt time.Time
		if err := rows.Scan(&sourceID, &sourceLogID, &sourceUserID, &sourceKeyID, &sourceSubID, &billingType, &applied, &delta, &createdAt); err != nil {
			return err
		}
		if _, ok := known[sourceID]; ok {
			result.billingExists++
			continue
		}
		targetLogID, ok := logMap[sourceLogID]
		if !ok {
			continue
		}
		targetUserID, userOK := userMap[sourceUserID]
		targetKeyID, keyOK := keyMap[sourceKeyID]
		if !userOK || !keyOK {
			return fmt.Errorf("billing entry %d has unmapped user or api key", sourceID)
		}
		var targetSub any
		if sourceSubID.Valid {
			mappedSub, ok := subMap[sourceSubID.Int64]
			if ok {
				targetSub = mappedSub
			}
		}
		var targetID int64
		if err := insertEntry.QueryRowContext(ctx, targetLogID, targetUserID, targetKeyID, targetSub, billingType, applied, delta, createdAt).Scan(&targetID); err != nil {
			return err
		}
		if _, err := insertState.ExecContext(ctx, sourceID, targetID); err != nil {
			return err
		}
		known[sourceID] = struct{}{}
		result.billingAdded++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func asInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported integer value %T", value)
	}
}

func nullTimeValue(value sql.NullTime) any {
	if value.Valid {
		return value.Time
	}
	return nil
}

func placeholders(count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "$" + strconv.Itoa(i+1)
	}
	return strings.Join(parts, ",")
}

func quoteIdentifiers(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return result
}

type postgresArray []int64

func pqArray(values []int64) any {
	return postgresArray(values)
}

func (a postgresArray) Value() (driver.Value, error) {
	parts := make([]string, len(a))
	for i, value := range a {
		parts[i] = strconv.FormatInt(value, 10)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}
