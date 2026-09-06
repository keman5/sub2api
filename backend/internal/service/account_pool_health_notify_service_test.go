package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type accountPoolHealthRepoStub struct {
	accounts    []Account
	schedulable []Account
}

var _ AccountRepository = (*accountPoolHealthRepoStub)(nil)

func (r *accountPoolHealthRepoStub) Create(context.Context, *Account) error { return nil }
func (r *accountPoolHealthRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return nil, ErrAccountNotFound
}
func (r *accountPoolHealthRepoStub) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ExistsByID(context.Context, int64) (bool, error) {
	return false, nil
}
func (r *accountPoolHealthRepoStub) GetByCRSAccountID(context.Context, string) (*Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) FindByExtraField(context.Context, string, any) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) Update(context.Context, *Account) error { return nil }
func (r *accountPoolHealthRepoStub) Delete(context.Context, int64) error    { return nil }
func (r *accountPoolHealthRepoStub) ResetQuotaUsedAndClearRateLimitCooldown(context.Context, int64) error {
	return nil
}
func (r *accountPoolHealthRepoStub) List(_ context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	size := params.Limit()
	start := (page - 1) * size
	if start >= len(r.accounts) {
		return nil, &pagination.PaginationResult{Total: int64(len(r.accounts)), Page: page, PageSize: size, Pages: pagesFor(len(r.accounts), size)}, nil
	}
	end := start + size
	if end > len(r.accounts) {
		end = len(r.accounts)
	}
	return r.accounts[start:end], &pagination.PaginationResult{Total: int64(len(r.accounts)), Page: page, PageSize: size, Pages: pagesFor(len(r.accounts), size)}, nil
}
func (r *accountPoolHealthRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *accountPoolHealthRepoStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListByGroup(context.Context, int64) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListActive(context.Context) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) UpdateLastUsed(context.Context, int64) error { return nil }
func (r *accountPoolHealthRepoStub) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (r *accountPoolHealthRepoStub) SetError(context.Context, int64, string) error { return nil }
func (r *accountPoolHealthRepoStub) ClearError(context.Context, int64) error       { return nil }
func (r *accountPoolHealthRepoStub) SetSchedulable(context.Context, int64, bool) error {
	return nil
}
func (r *accountPoolHealthRepoStub) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *accountPoolHealthRepoStub) BindGroups(context.Context, int64, []int64) error {
	return nil
}
func (r *accountPoolHealthRepoStub) ListSchedulable(context.Context) ([]Account, error) {
	return r.schedulable, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	return nil, nil
}
func (r *accountPoolHealthRepoStub) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}
func (r *accountPoolHealthRepoStub) SetModelRateLimit(context.Context, int64, string, time.Time, ...string) error {
	return nil
}
func (r *accountPoolHealthRepoStub) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}
func (r *accountPoolHealthRepoStub) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}
func (r *accountPoolHealthRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	return nil
}
func (r *accountPoolHealthRepoStub) ClearRateLimit(context.Context, int64) error { return nil }
func (r *accountPoolHealthRepoStub) ClearAntigravityQuotaScopes(context.Context, int64) error {
	return nil
}
func (r *accountPoolHealthRepoStub) ClearModelRateLimits(context.Context, int64) error {
	return nil
}
func (r *accountPoolHealthRepoStub) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	return nil
}
func (r *accountPoolHealthRepoStub) UpdateSessionWindowEnd(context.Context, int64, time.Time) error {
	return nil
}
func (r *accountPoolHealthRepoStub) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}
func (r *accountPoolHealthRepoStub) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	return 0, nil
}
func (r *accountPoolHealthRepoStub) IncrementQuotaUsed(context.Context, int64, float64) error {
	return nil
}
func (r *accountPoolHealthRepoStub) ResetQuotaUsed(context.Context, int64) error { return nil }
func (r *accountPoolHealthRepoStub) RevertProxyFallback(context.Context, int64) error {
	return nil
}

func pagesFor(total, pageSize int) int {
	if pageSize <= 0 {
		pageSize = 20
	}
	if total == 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	return pages
}

func TestAccountPoolHealthNotifyService_SendsOnceWhileAllAccountsUnavailable(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)

	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.SetMultiple(ctx, smtpServer.settings()))
	require.NoError(t, settings.Set(ctx, SettingKeySiteName, "51token"))
	require.NoError(t, settings.Set(ctx, SettingKeyNotificationEmailDefaultLocale, "en"))
	require.NoError(t, settings.Set(ctx, SettingKeyAccountQuotaNotifyEnabled, "true"))
	require.NoError(t, settings.Set(ctx, SettingKeyAccountQuotaNotifyEmails, MarshalNotifyEmails([]NotifyEmailEntry{
		{Email: "ops@example.com", Verified: true},
	})))

	notes := "primary token"
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	repo := &accountPoolHealthRepoStub{
		accounts: []Account{
			{ID: 1, Name: "openai-main", Notes: &notes, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled, Schedulable: true},
			{ID: 2, Name: "openai-backup", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false},
		},
	}
	emailSvc := NewEmailService(settings, nil)
	notificationSvc := NewNotificationEmailService(settings, emailSvc)
	svc := NewAccountPoolHealthNotifyService(repo, settings, notificationSvc, time.Minute)
	svc.nowFunc = func() time.Time { return now }

	svc.runOnce()
	svc.runOnce()

	require.EqualValues(t, 1, smtpServer.messageCount())
	bodies := smtpServer.messageBodies()
	require.Len(t, bodies, 1)
	require.Contains(t, bodies[0], "All accounts unavailable")
	require.Contains(t, bodies[0], "No upstream account is currently schedulable")
	require.Contains(t, bodies[0], "openai-main")
	require.Contains(t, bodies[0], "primary token")
	require.Contains(t, bodies[0], "openai-backup")
}

func TestAccountPoolHealthNotifyService_SkipsEmptyAccountPool(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)

	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.SetMultiple(ctx, smtpServer.settings()))
	require.NoError(t, settings.Set(ctx, SettingKeyAccountQuotaNotifyEnabled, "true"))
	require.NoError(t, settings.Set(ctx, SettingKeyAccountQuotaNotifyEmails, MarshalNotifyEmails([]NotifyEmailEntry{
		{Email: "ops@example.com", Verified: true},
	})))

	emailSvc := NewEmailService(settings, nil)
	notificationSvc := NewNotificationEmailService(settings, emailSvc)
	svc := NewAccountPoolHealthNotifyService(&accountPoolHealthRepoStub{}, settings, notificationSvc, time.Minute)

	svc.runOnce()

	require.EqualValues(t, 0, smtpServer.messageCount())
}

func TestAccountPoolHealthNotifyService_RecoveryClearsOneTimeGuard(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)

	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.SetMultiple(ctx, smtpServer.settings()))
	require.NoError(t, settings.Set(ctx, SettingKeyAccountQuotaNotifyEnabled, "true"))
	require.NoError(t, settings.Set(ctx, SettingKeyAccountQuotaNotifyEmails, MarshalNotifyEmails([]NotifyEmailEntry{
		{Email: "ops@example.com", Verified: true},
	})))

	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	repo := &accountPoolHealthRepoStub{
		accounts: []Account{{ID: 1, Name: "claude-main", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false}},
	}
	emailSvc := NewEmailService(settings, nil)
	notificationSvc := NewNotificationEmailService(settings, emailSvc)
	svc := NewAccountPoolHealthNotifyService(repo, settings, notificationSvc, time.Minute)
	svc.nowFunc = func() time.Time { return now }

	svc.runOnce()
	require.EqualValues(t, 1, smtpServer.messageCount())

	repo.schedulable = []Account{{ID: 1, Name: "claude-main", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}}
	now = now.Add(time.Minute)
	svc.runOnce()
	_, err := settings.GetValue(ctx, SettingKeyAccountPoolUnavailableOutageID)
	require.ErrorIs(t, err, ErrSettingNotFound)

	repo.schedulable = nil
	now = now.Add(time.Minute)
	svc.runOnce()
	require.EqualValues(t, 2, smtpServer.messageCount())
}

func TestNotificationEmailAccountPoolUnavailableAllowsAccountRowsHTML(t *testing.T) {
	rendered, err := renderNotificationEmail(
		NotificationEmailEventAccountPoolUnavailable,
		"[{{site_name}}] {{account_count}} 个账号不可调度",
		"<p>{{checked_at}}</p>{{accounts}}",
		map[string]string{
			"site_name":     "51token",
			"account_count": "2",
			"checked_at":    "2026-07-01 10:00:00",
		},
		map[string]string{
			"accounts": "<table><tbody><tr><td>openai-main</td></tr></tbody></table>",
		},
	)

	require.NoError(t, err)
	require.Contains(t, rendered.Subject, "2")
	require.Contains(t, rendered.HTML, "<table>")
	require.Contains(t, rendered.HTML, "openai-main")
	require.NotContains(t, rendered.HTML, "&lt;table&gt;")
}

func TestRenderAccountPoolUnavailableRowsEscapesAccountFields(t *testing.T) {
	notes := `<img src=x onerror=alert(1)>`
	rendered := renderAccountPoolUnavailableRows([]Account{
		{
			ID:          1,
			Name:        `<script>alert("x")</script>`,
			Notes:       &notes,
			Platform:    PlatformOpenAI,
			Status:      StatusDisabled,
			Schedulable: false,
		},
	})

	require.Contains(t, rendered, `&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`)
	require.Contains(t, rendered, `&lt;img src=x onerror=alert(1)&gt;`)
	require.NotContains(t, rendered, `<script>`)
	require.NotContains(t, rendered, `<img src=x`)
}
