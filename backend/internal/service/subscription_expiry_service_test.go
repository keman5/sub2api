package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type subscriptionExpiryRepoStub struct {
	listCalls             int
	expiredSubscriptions  []UserSubscription
	subscriptionsByStatus map[string][]UserSubscription
	receivedListStatuses  []string
}

func (r *subscriptionExpiryRepoStub) Create(context.Context, *UserSubscription) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetByIDForUpdate(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) Update(context.Context, *UserSubscription) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) Restore(context.Context, int64, string) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

func (r *subscriptionExpiryRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func (r *subscriptionExpiryRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func (r *subscriptionExpiryRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *subscriptionExpiryRepoStub) List(_ context.Context, _ pagination.PaginationParams, _ *int64, _ *int64, status string, _ string, _ string, _ string) ([]UserSubscription, *pagination.PaginationResult, error) {
	r.listCalls++
	r.receivedListStatuses = append(r.receivedListStatuses, status)
	if r.subscriptionsByStatus != nil {
		if subs, ok := r.subscriptionsByStatus[status]; ok {
			return subs, &pagination.PaginationResult{Page: 1, Pages: 1}, nil
		}
	}
	return nil, &pagination.PaginationResult{Page: 1, Pages: 1}, nil
}

func (r *subscriptionExpiryRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (r *subscriptionExpiryRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (r *subscriptionExpiryRepoStub) ExtendExpiry(context.Context, int64, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) UpdateStatus(context.Context, int64, string) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) UpdateNotes(context.Context, int64, string) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ActivateWindows(context.Context, int64, time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) IncrementUsage(context.Context, int64, float64) error {
	return nil
}

func (r *subscriptionExpiryRepoStub) BatchUpdateExpiredStatus(context.Context) ([]UserSubscription, error) {
	return r.expiredSubscriptions, nil
}

type subscriptionExpirySettingRepoStub struct {
	values   map[string]string
	err      error
	multiErr error
}

func (r *subscriptionExpirySettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *subscriptionExpirySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *subscriptionExpirySettingRepoStub) Set(context.Context, string, string) error {
	return nil
}

func (r *subscriptionExpirySettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.multiErr != nil {
		return nil, r.multiErr
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (r *subscriptionExpirySettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (r *subscriptionExpirySettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}

func (r *subscriptionExpirySettingRepoStub) Delete(context.Context, string) error {
	return nil
}

func TestSubscriptionExpiryService_ExpiryReminderEnabledDefaultsToTrue(t *testing.T) {
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{values: map[string]string{}})

	require.True(t, svc.expiryReminderEnabled(context.Background()))
}

func TestSubscriptionExpiryService_ExpiryReminderDisabledSkipsSubscriptionScan(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{
		values: map[string]string{SettingKeySubscriptionExpiryNotifyEnabled: "false"},
	}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls)
}

func TestSubscriptionExpiryService_ExpiryReminderSettingReadErrorFailsClosed(t *testing.T) {
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{err: errors.New("db down")})

	require.False(t, svc.expiryReminderEnabled(context.Background()))
}

func TestSubscriptionExpiryService_ExpiredAdminNotifyEnabledDefaultsToFalse(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		err    error
	}{
		{name: "missing setting", values: map[string]string{}},
		{name: "empty setting", values: map[string]string{SettingKeySubscriptionExpiredAdminNotifyEnabled: ""}},
		{name: "false setting", values: map[string]string{SettingKeySubscriptionExpiredAdminNotifyEnabled: "false"}},
		{name: "read error", err: errors.New("db down")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSubscriptionExpiryService(nil, time.Minute)
			svc.SetSettingRepository(&subscriptionExpirySettingRepoStub{values: tc.values, err: tc.err})

			require.False(t, svc.expiredAdminNotifyEnabled(context.Background()))
		})
	}
}

func TestSubscriptionExpiryService_RunOnceSendsExpiredAdminNotification(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)
	defer smtpServer.close()

	settings := newNotificationEmailMemorySettingRepo()
	for key, value := range smtpServer.settings() {
		require.NoError(t, settings.Set(context.Background(), key, value))
	}
	require.NoError(t, settings.Set(context.Background(), SettingKeySiteName, "51token"))
	require.NoError(t, settings.Set(context.Background(), SettingKeyNotificationEmailDefaultLocale, "en"))
	require.NoError(t, settings.Set(context.Background(), SettingKeySubscriptionExpiredAdminNotifyEnabled, "true"))
	require.NoError(t, settings.Set(context.Background(), SettingKeySubscriptionExpiredAdminNotifyEmails, MarshalNotifyEmails([]NotifyEmailEntry{
		{Email: "macseek@upit.top", Verified: true},
	})))

	expiresAt := time.Date(2026, 6, 27, 9, 30, 0, 0, time.UTC)
	repo := &subscriptionExpiryRepoStub{
		expiredSubscriptions: []UserSubscription{
			{
				ID:        42,
				UserID:    7,
				GroupID:   3,
				ExpiresAt: expiresAt,
				Status:    SubscriptionStatusExpired,
				User:      &User{ID: 7, Email: "user@example.com", Username: "demo"},
				Group:     &Group{ID: 3, Name: "Pro"},
			},
		},
	}
	emailSvc := NewEmailService(settings, nil)
	notificationSvc := NewNotificationEmailService(settings, emailSvc)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settings)
	svc.SetNotificationEmailService(notificationSvc)
	svc.nowFunc = func() time.Time { return expiresAt }

	svc.runOnce()

	require.EqualValues(t, 0, smtpServer.messageCount())

	repo.expiredSubscriptions = nil
	svc.nowFunc = func() time.Time { return expiresAt.Add(10*time.Minute + time.Second) }
	svc.runOnce()

	require.EqualValues(t, 1, smtpServer.messageCount())
}

func TestSubscriptionExpiryService_ExpiredAdminNotificationsAreBatched(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)
	defer smtpServer.close()

	settings := newNotificationEmailMemorySettingRepo()
	for key, value := range smtpServer.settings() {
		require.NoError(t, settings.Set(context.Background(), key, value))
	}
	require.NoError(t, settings.Set(context.Background(), SettingKeySiteName, "51token"))
	require.NoError(t, settings.Set(context.Background(), SettingKeyNotificationEmailDefaultLocale, "en"))
	require.NoError(t, settings.Set(context.Background(), SettingKeySubscriptionExpiredAdminNotifyEnabled, "true"))
	require.NoError(t, settings.Set(context.Background(), SettingKeySubscriptionExpiredAdminNotifyEmails, MarshalNotifyEmails([]NotifyEmailEntry{
		{Email: "macseek@upit.top", Verified: true},
	})))

	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	repo := &subscriptionExpiryRepoStub{
		expiredSubscriptions: []UserSubscription{
			{
				ID:        42,
				UserID:    7,
				GroupID:   3,
				ExpiresAt: now.Add(-time.Minute),
				Status:    SubscriptionStatusExpired,
				User:      &User{ID: 7, Email: "alpha@example.com", Username: "alpha", Notes: "alpha admin note"},
				Group:     &Group{ID: 3, Name: "Pro"},
			},
			{
				ID:        43,
				UserID:    8,
				GroupID:   4,
				ExpiresAt: now.Add(-2 * time.Minute),
				Status:    SubscriptionStatusExpired,
				User:      &User{ID: 8, Email: "beta@example.com", Username: "beta", Notes: "beta admin note"},
				Group:     &Group{ID: 4, Name: "Spark"},
			},
		},
	}
	emailSvc := NewEmailService(settings, nil)
	notificationSvc := NewNotificationEmailService(settings, emailSvc)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settings)
	svc.SetNotificationEmailService(notificationSvc)
	svc.nowFunc = func() time.Time { return now }

	svc.runOnce()

	require.EqualValues(t, 0, smtpServer.messageCount(), "first scan should only open the 10 minute aggregation window")

	repo.expiredSubscriptions = nil
	svc.nowFunc = func() time.Time { return now.Add(10*time.Minute + time.Second) }
	svc.runOnce()

	require.EqualValues(t, 1, smtpServer.messageCount())
	bodies := smtpServer.messageBodies()
	require.Len(t, bodies, 1)
	require.Contains(t, bodies[0], "Subscriptions expired")
	require.Contains(t, bodies[0], "subscription(s) expired")
	require.Contains(t, bodies[0], "alpha@example.com")
	require.Contains(t, bodies[0], "beta@example.com")
	require.Contains(t, bodies[0], "alpha admin note")
	require.Contains(t, bodies[0], "beta admin note")
}
