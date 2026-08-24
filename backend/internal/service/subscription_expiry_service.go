package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/google/uuid"
)

const (
	subscriptionExpiryReminderSMTPWarningInterval = time.Minute
	// subscriptionExpiryReminderLeaderLockKey gates the per-cycle reminder scan so
	// that only one instance walks all active subscriptions and sends reminder
	// emails, avoiding redundant full scans and duplicate emails.
	subscriptionExpiryReminderLeaderLockKey = "subscription:expiry:reminder:leader"
	// subscriptionExpiryReminderLeaderLockTTL bounds crash recovery; the scan can
	// page through many subscriptions, so keep it comfortably above one cycle.
	subscriptionExpiryReminderLeaderLockTTL = 5 * time.Minute
	subscriptionExpiredAdminBatchWindow     = 10 * time.Minute
)

// SubscriptionExpiryService periodically updates expired subscription status.
type SubscriptionExpiryService struct {
	userSubRepo              UserSubscriptionRepository
	settingRepo              SettingRepository
	notificationEmailService *NotificationEmailService
	interval                 time.Duration
	stopCh                   chan struct{}
	stopOnce                 sync.Once
	wg                       sync.WaitGroup

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
	nowFunc    func() time.Time

	expiredAdminBatchMu       sync.Mutex
	expiredAdminBatchOpenedAt time.Time
	expiredAdminBatchItems    []UserSubscription

	smtpWarningMu   sync.Mutex
	lastSMTPWarning time.Time
}

func NewSubscriptionExpiryService(userSubRepo UserSubscriptionRepository, interval time.Duration) *SubscriptionExpiryService {
	return &SubscriptionExpiryService{
		userSubRepo: userSubRepo,
		interval:    interval,
		stopCh:      make(chan struct{}),
		instanceID:  uuid.NewString(),
		nowFunc:     time.Now,
	}
}

// SetLeaderLock injects the leader-lock cache and DB used to elect a single
// instance for the periodic expiry-reminder scan. When both are nil the scan runs
// ungated (single-instance / test behavior).
func (s *SubscriptionExpiryService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *SubscriptionExpiryService) SetSettingRepository(settingRepo SettingRepository) {
	s.settingRepo = settingRepo
}

func (s *SubscriptionExpiryService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
}

func (s *SubscriptionExpiryService) Start() {
	if s == nil || s.userSubRepo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *SubscriptionExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *SubscriptionExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expired, err := s.userSubRepo.BatchUpdateExpiredStatus(ctx)
	if err != nil {
		log.Printf("[SubscriptionExpiry] Update expired subscriptions failed: %v", err)
		return
	}
	if len(expired) > 0 {
		log.Printf("[SubscriptionExpiry] Updated %d expired subscriptions", len(expired))
	}
	s.sendExpiredAdminNotifications(ctx, expired)
	s.sendExpiryReminders(ctx)
}

func (s *SubscriptionExpiryService) sendExpiredAdminNotifications(ctx context.Context, expired []UserSubscription) {
	if s == nil || s.userSubRepo == nil || s.settingRepo == nil || s.notificationEmailService == nil {
		return
	}
	s.queueExpiredAdminNotifications(expired)
	s.flushExpiredAdminNotificationsIfDue(ctx)
}

func (s *SubscriptionExpiryService) queueExpiredAdminNotifications(expired []UserSubscription) {
	if len(expired) == 0 {
		return
	}
	now := s.now()
	s.expiredAdminBatchMu.Lock()
	defer s.expiredAdminBatchMu.Unlock()
	if s.expiredAdminBatchOpenedAt.IsZero() {
		s.expiredAdminBatchOpenedAt = now
	}
	s.expiredAdminBatchItems = append(s.expiredAdminBatchItems, expired...)
}

func (s *SubscriptionExpiryService) flushExpiredAdminNotificationsIfDue(ctx context.Context) {
	if s == nil || s.settingRepo == nil || s.notificationEmailService == nil {
		return
	}
	items := s.dueExpiredAdminNotificationBatch()
	if len(items) == 0 {
		return
	}
	if !s.expiredAdminNotifyEnabled(ctx) {
		return
	}
	recipients := s.expiredAdminNotifyEmails(ctx)
	if len(recipients) == 0 {
		return
	}

	s.sendExpiredAdminNotificationBatch(ctx, items, recipients)
}

func (s *SubscriptionExpiryService) dueExpiredAdminNotificationBatch() []UserSubscription {
	now := s.now()
	s.expiredAdminBatchMu.Lock()
	defer s.expiredAdminBatchMu.Unlock()
	if s.expiredAdminBatchOpenedAt.IsZero() || len(s.expiredAdminBatchItems) == 0 {
		return nil
	}
	if now.Sub(s.expiredAdminBatchOpenedAt) < subscriptionExpiredAdminBatchWindow {
		return nil
	}
	items := append([]UserSubscription(nil), s.expiredAdminBatchItems...)
	s.expiredAdminBatchItems = nil
	s.expiredAdminBatchOpenedAt = time.Time{}
	return items
}

func (s *SubscriptionExpiryService) now() time.Time {
	if s == nil || s.nowFunc == nil {
		return time.Now()
	}
	return s.nowFunc()
}

func (s *SubscriptionExpiryService) expiredAdminNotifyEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySubscriptionExpiredAdminNotifyEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return false
		}
		log.Printf("[SubscriptionExpiry] Read expired admin notification switch failed: %v", err)
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func (s *SubscriptionExpiryService) expiredAdminNotifyEmails(ctx context.Context) []string {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeySubscriptionExpiredAdminNotifyEmails)
	if err != nil || strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "[]" {
		return nil
	}
	return filterVerifiedEmails(ParseNotifyEmails(raw))
}

func (s *SubscriptionExpiryService) sendExpiredAdminNotificationBatch(ctx context.Context, subs []UserSubscription, recipients []string) {
	rows, first := buildExpiredAdminNotificationRows(subs)
	if len(rows) == 0 || first == nil {
		return
	}
	sourceID := expiredAdminNotificationBatchSourceID(subs)
	for _, recipient := range recipients {
		if err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventSubscriptionExpiredAdmin,
			RecipientEmail: recipient,
			RecipientName:  emailRecipientName(recipient),
			SourceType:     "user_subscription_expired_batch",
			SourceID:       sourceID,
			ReminderKey:    "expired",
			Variables: map[string]string{
				"subscription_id":    strconv.FormatInt(first.SubscriptionID, 10),
				"subscription_group": first.GroupName,
				"user_id":            strconv.FormatInt(first.UserID, 10),
				"user_email":         first.UserEmail,
				"user_name":          first.UserName,
				"expiry_time":        first.ExpiryTime.Format("2006-01-02 15:04"),
				"expired_count":      strconv.Itoa(len(rows)),
			},
			RawHTMLVariables: map[string]string{
				"expired_subscriptions": renderExpiredAdminNotificationRows(rows),
			},
		}); err != nil {
			log.Printf("[SubscriptionExpiry] Send expired admin notification failed: count=%d recipient=%s err=%v", len(rows), recipient, err)
		}
	}
}

type expiredAdminNotificationRow struct {
	SubscriptionID int64
	GroupName      string
	UserID         int64
	UserEmail      string
	UserName       string
	UserNotes      string
	ExpiryTime     time.Time
}

func buildExpiredAdminNotificationRows(subs []UserSubscription) ([]expiredAdminNotificationRow, *expiredAdminNotificationRow) {
	rows := make([]expiredAdminNotificationRow, 0, len(subs))
	for i := range subs {
		sub := subs[i]
		if sub.User == nil || sub.Group == nil {
			continue
		}
		rows = append(rows, expiredAdminNotificationRow{
			SubscriptionID: sub.ID,
			GroupName:      sub.Group.Name,
			UserID:         sub.UserID,
			UserEmail:      sub.User.Email,
			UserName:       firstNonEmpty(sub.User.Username, sub.User.Email),
			UserNotes:      sub.User.Notes,
			ExpiryTime:     sub.ExpiresAt,
		})
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows, &rows[0]
}

func renderExpiredAdminNotificationRows(rows []expiredAdminNotificationRow) string {
	var builder strings.Builder
	write := func(value string) {
		_, _ = builder.WriteString(value)
	}
	write(`<table style="width:100%;border-collapse:collapse;margin:16px 0;">`)
	write(`<thead><tr>`)
	for _, header := range []string{"ID", "Group", "User", "Expired at"} {
		write(`<th style="border-bottom:1px solid #e5e7eb;padding:8px;text-align:left;">`)
		write(html.EscapeString(header))
		write(`</th>`)
	}
	write(`</tr></thead><tbody>`)
	for _, row := range rows {
		write(`<tr>`)
		write(`<td style="border-bottom:1px solid #f3f4f6;padding:8px;">#`)
		write(strconv.FormatInt(row.SubscriptionID, 10))
		write(`</td>`)
		write(`<td style="border-bottom:1px solid #f3f4f6;padding:8px;">`)
		write(html.EscapeString(row.GroupName))
		write(`</td>`)
		write(`<td style="border-bottom:1px solid #f3f4f6;padding:8px;">`)
		write(html.EscapeString(row.UserEmail))
		secondary := strings.TrimSpace(row.UserNotes)
		if secondary == "" && row.UserName != row.UserEmail {
			secondary = strings.TrimSpace(row.UserName)
		}
		if secondary != "" {
			write(`<br><span style="color:#6b7280;">`)
			write(html.EscapeString(secondary))
			write(`</span>`)
		}
		write(`</td>`)
		write(`<td style="border-bottom:1px solid #f3f4f6;padding:8px;">`)
		write(html.EscapeString(row.ExpiryTime.Format("2006-01-02 15:04")))
		write(`</td>`)
		write(`</tr>`)
	}
	write(`</tbody></table>`)
	return builder.String()
}

func expiredAdminNotificationBatchSourceID(subs []UserSubscription) string {
	ids := make([]string, 0, len(subs))
	for i := range subs {
		if subs[i].ID <= 0 {
			continue
		}
		ids = append(ids, strconv.FormatInt(subs[i].ID, 10))
	}
	if len(ids) == 0 {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return strings.Join(ids, ",")
}

func (s *SubscriptionExpiryService) sendExpiryReminders(ctx context.Context) {
	if s == nil || s.userSubRepo == nil || s.notificationEmailService == nil {
		return
	}
	if !s.expiryReminderEnabled(ctx) {
		return
	}
	if !s.smtpConfigured(ctx) {
		return
	}

	// Multi-instance guard: only the leader walks every active subscription and
	// sends reminders, avoiding N× full scans and duplicate reminder emails.
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, subscriptionExpiryReminderLeaderLockKey, s.instanceID, subscriptionExpiryReminderLeaderLockTTL)
	if !ok {
		return
	}
	defer release()
	for page := 1; ; page++ {
		subs, pag, err := s.userSubRepo.List(ctx, pagination.PaginationParams{Page: page, PageSize: 200}, nil, nil, SubscriptionStatusActive, "", "expires_at", "asc")
		if err != nil {
			log.Printf("[SubscriptionExpiry] List active subscriptions for reminder failed: %v", err)
			return
		}
		for i := range subs {
			s.sendExpiryReminderIfDue(ctx, &subs[i])
		}
		if pag == nil || page >= pag.Pages || len(subs) == 0 {
			return
		}
	}
}

func (s *SubscriptionExpiryService) expiryReminderEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return true
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeySubscriptionExpiryNotifyEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return true
		}
		log.Printf("[SubscriptionExpiry] Read expiry reminder switch failed: %v", err)
		return false
	}
	return !isFalseSettingValue(value)
}

func (s *SubscriptionExpiryService) smtpConfigured(ctx context.Context) bool {
	if s == nil || s.notificationEmailService == nil || s.notificationEmailService.emailService == nil {
		return false
	}
	_, err := s.notificationEmailService.emailService.GetSMTPConfig(ctx)
	if err == nil {
		return true
	}
	if errors.Is(err, ErrEmailNotConfigured) {
		s.smtpWarningMu.Lock()
		defer s.smtpWarningMu.Unlock()
		now := time.Now()
		if s.lastSMTPWarning.IsZero() || now.Sub(s.lastSMTPWarning) >= subscriptionExpiryReminderSMTPWarningInterval {
			log.Printf("[SubscriptionExpiry] SMTP is not configured; skipping expiry reminders")
			s.lastSMTPWarning = now
		}
		return false
	}
	log.Printf("[SubscriptionExpiry] Read SMTP configuration failed; skipping expiry reminders: %v", err)
	return false
}

func (s *SubscriptionExpiryService) sendExpiryReminderIfDue(ctx context.Context, sub *UserSubscription) {
	if sub == nil || sub.User == nil || sub.Group == nil || sub.User.Email == "" {
		return
	}
	daysRemaining := sub.DaysRemaining()
	if daysRemaining != 7 && daysRemaining != 3 && daysRemaining != 1 {
		return
	}
	if err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionExpiryReminder,
		RecipientEmail: sub.User.Email,
		RecipientName:  firstNonEmpty(sub.User.Username, sub.User.Email),
		UserID:         sub.UserID,
		SourceType:     "user_subscription",
		SourceID:       strconv.FormatInt(sub.ID, 10),
		ReminderKey:    fmt.Sprintf("%dd", daysRemaining),
		Variables: map[string]string{
			"subscription_group": sub.Group.Name,
			"expiry_time":        sub.ExpiresAt.Format("2006-01-02 15:04"),
			"days_remaining":     strconv.Itoa(daysRemaining),
		},
	}); err != nil {
		log.Printf("[SubscriptionExpiry] Send expiry reminder failed: subscription=%d user=%d err=%v", sub.ID, sub.UserID, err)
	}
}
