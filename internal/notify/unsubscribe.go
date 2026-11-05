// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/watches"
)

// What an unsubscribe stops.
const (
	ScopeAll    = "all"
	ScopeDigest = "digest"
	ScopeWatch  = "watch"
)

const tokenContext = "mail-unsubscribe"

type unsubscribe struct {
	User  uuid.UUID `json:"u"`
	Scope string    `json:"s"`
	Page  uuid.UUID `json:"p"`
	Space uuid.UUID `json:"w"`
}

// UnsubscribeToken makes the address that stops mail. It is sealed, so it cannot be
// changed or guessed, and it never expires: the worst abuse is less mail (spec 010, D15).
func (s *Service) UnsubscribeToken(user uuid.UUID, scope string) (string, error) {
	return s.seal(unsubscribe{User: user, Scope: scope})
}

// StopWatchingToken makes the address that stops what brings one page to a person.
func (s *Service) StopWatchingToken(user, pageID, spaceID uuid.UUID) (string, error) {
	return s.seal(unsubscribe{User: user, Scope: ScopeWatch, Page: pageID, Space: spaceID})
}

func (s *Service) seal(u unsubscribe) (string, error) {
	raw, err := json.Marshal(u)
	if err != nil {
		return "", err
	}
	sealed, err := s.options.Keyring.Seal(raw, tokenContext)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func quieter(channel, scope string) string {
	if channel == ChannelDigest || (scope == ScopeAll && channel == ChannelMail) {
		return ChannelApp
	}
	return channel
}

// Unsubscribe carries out what a token says. No session is needed, because RFC 8058
// sends none: the token is the proof. A bad token does nothing, and says so plainly.
func (s *Service) Unsubscribe(ctx context.Context, token string) error {
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return apperr.NotFound()
	}
	raw, err := s.options.Keyring.Open(sealed, tokenContext)
	if err != nil {
		return apperr.NotFound()
	}
	var u unsubscribe
	if err := json.Unmarshal(raw, &u); err != nil {
		return apperr.NotFound()
	}
	if u.Scope == ScopeWatch {
		return watches.StopCovering(ctx, s.pool, u.User, u.Page, u.Space)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		choices, err := load(ctx, tx, u.User)
		if err != nil {
			return err
		}
		for _, kind := range Types {
			choices.Channels[kind] = quieter(choices.channel(kind), u.Scope)
		}
		return save(ctx, tx, u.User, choices)
	})
}
