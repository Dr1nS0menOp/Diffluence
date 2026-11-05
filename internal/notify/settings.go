// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
	_ "time/tzdata" // due dates and digests work in a person's own time zone

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
)

// The types of notification.
const (
	TypeMention       = "mention"
	TypeTaskAssigned  = "task_assigned"
	TypeTaskDue       = "task_due"
	TypeShare         = "share"
	TypeComment       = "comment"
	TypeReply         = "reply"
	TypeReaction      = "reaction"
	TypePageChanged   = "page_changed"
	TypeAccessRequest = "access_request"
	TypeAccessAnswer  = "access_answer"
)

// Where a type of notification goes.
const (
	ChannelOff    = "off"
	ChannelApp    = "app"
	ChannelMail   = "app_mail"
	ChannelDigest = "app_digest"
)

// Digest frequencies.
const (
	DigestDaily  = "daily"
	DigestWeekly = "weekly"
)

const defaultDigestHour = 8

// Types lists every type a person can choose a channel for.
var Types = []string{TypeMention, TypeTaskAssigned, TypeTaskDue, TypeShare, TypeComment, TypeReply, TypeReaction,
	TypePageChanged, TypeAccessRequest, TypeAccessAnswer}

var channels = []string{ChannelOff, ChannelApp, ChannelMail, ChannelDigest}

// Settings are what a person chooses (spec 010, C13).
type Settings struct {
	Channels         map[string]string `json:"channels"`
	DigestFrequency  string            `json:"digest_frequency"`
	DigestDay        int               `json:"digest_day"`
	DigestHour       int               `json:"digest_hour"`
	TimeZone         string            `json:"time_zone"`
	QuietStart       string            `json:"quiet_start"`
	QuietEnd         string            `json:"quiet_end"`
	PausedUntil      string            `json:"paused_until"`
	AutoWatchCreate  bool              `json:"auto_watch_create"`
	AutoWatchEdit    bool              `json:"auto_watch_edit"`
	AutoWatchComment bool              `json:"auto_watch_comment"`
	NotifyOwn        bool              `json:"notify_own"`
	CommentOrder     string            `json:"comment_order"`
}

// defaultChannels: what is addressed to a person arrives at once, and the rest in the
// digest (spec 010, D25).
func defaultChannels() map[string]string {
	return map[string]string{
		TypeMention: ChannelMail, TypeTaskAssigned: ChannelMail, TypeTaskDue: ChannelMail, TypeShare: ChannelMail,
		TypeAccessRequest: ChannelMail, TypeAccessAnswer: ChannelMail,
		TypeComment: ChannelDigest, TypeReply: ChannelDigest, TypeReaction: ChannelDigest, TypePageChanged: ChannelDigest,
	}
}

func defaultSettings() Settings {
	return Settings{Channels: defaultChannels(), DigestFrequency: DigestDaily, DigestHour: defaultDigestHour, TimeZone: "UTC",
		AutoWatchCreate: true, AutoWatchEdit: true, AutoWatchComment: true, CommentOrder: "oldest"}
}

func validClock(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("15:04", s)
	return err == nil
}

func (v Settings) validate() error {
	for name, channel := range v.Channels {
		if !slices.Contains(Types, name) || !slices.Contains(channels, channel) {
			return apperr.Invalid("That is not a choice for a notification.")
		}
	}
	if !slices.Contains([]string{DigestDaily, DigestWeekly}, v.DigestFrequency) {
		return apperr.Invalid("A digest comes daily or weekly.")
	}
	if v.DigestDay < 0 || v.DigestDay > 6 || v.DigestHour < 0 || v.DigestHour > 23 {
		return apperr.Invalid("That is not a day or an hour for a digest.")
	}
	if _, err := time.LoadLocation(v.TimeZone); err != nil {
		return apperr.Invalid("That is not a time zone.")
	}
	if !validClock(v.QuietStart) || !validClock(v.QuietEnd) {
		return apperr.Invalid("Quiet hours look like 22:00.")
	}
	if v.PausedUntil != "" {
		if _, err := time.Parse(time.DateOnly, v.PausedUntil); err != nil {
			return apperr.Invalid("A pause ends on a date.")
		}
	}
	if !slices.Contains([]string{"oldest", "newest"}, v.CommentOrder) {
		return apperr.Invalid("Comments are sorted oldest or newest first.")
	}
	return nil
}

func (v Settings) channel(kind string) string {
	if c, ok := v.Channels[kind]; ok {
		return c
	}
	return defaultChannels()[kind]
}

// load reads a person's choices. What they never set has its default.
func load(ctx context.Context, q db.Querier, user uuid.UUID) (Settings, error) {
	out := defaultSettings()
	var raw []byte
	err := q.QueryRow(ctx, "SELECT notifications FROM user_preferences WHERE user_id = $1", user).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	stored := Settings{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return out, fmt.Errorf("notify: read settings: %v", err)
	}
	if stored.Channels != nil {
		maps.Copy(out.Channels, stored.Channels)
	}
	stored.Channels = out.Channels
	// An empty stored value means nothing was ever saved.
	if stored.DigestFrequency == "" {
		return out, nil
	}
	return stored, nil
}

func save(ctx context.Context, q db.Querier, user uuid.UUID, v Settings) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO user_preferences (user_id, notifications) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET notifications = EXCLUDED.notifications, updated_at = now()`, user, raw)
	return err
}

// Settings reads a person's choices.
func (s *Service) Settings(ctx context.Context, user uuid.UUID) (Settings, error) {
	return load(ctx, s.pool, user)
}

// SetSettings stores a person's choices. They are personal, so they are not audited.
func (s *Service) SetSettings(ctx context.Context, user uuid.UUID, v Settings) error {
	if err := v.validate(); err != nil {
		return err
	}
	return save(ctx, s.pool, user, v)
}

func (v Settings) location() *time.Location {
	if loc, err := time.LoadLocation(v.TimeZone); err == nil {
		return loc
	}
	return time.UTC
}

func (v Settings) paused(now time.Time) bool {
	return v.PausedUntil != "" && now.In(v.location()).Format(time.DateOnly) <= v.PausedUntil
}

func atClock(day time.Time, clock string) time.Time {
	t, err := time.Parse("15:04", clock)
	if err != nil {
		return day
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, day.Location())
}

// quietUntil says when quiet hours end, or nothing when mail may go now. A period can
// run over midnight, such as 22:00 to 07:00.
func (v Settings) quietUntil(now time.Time) time.Time {
	if v.QuietStart == "" || v.QuietEnd == "" || v.QuietStart == v.QuietEnd {
		return time.Time{}
	}
	local := now.In(v.location())
	start, end := atClock(local, v.QuietStart), atClock(local, v.QuietEnd)
	if !end.After(start) {
		if local.Before(end) {
			return end
		}
		if !local.Before(start) {
			return end.AddDate(0, 0, 1)
		}
		return time.Time{}
	}
	if !local.Before(start) && local.Before(end) {
		return end
	}
	return time.Time{}
}

// digestDue reports whether this hour is the person's digest hour.
func (v Settings) digestDue(now time.Time) bool {
	local := now.In(v.location())
	if local.Hour() != v.DigestHour {
		return false
	}
	return v.DigestFrequency == DigestDaily || int(local.Weekday()) == v.DigestDay
}
