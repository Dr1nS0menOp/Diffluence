// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/outbox"
)

const (
	challengeMFA     = "mfa"
	mfaTTL           = 5 * time.Minute
	maxMFAAttempts   = 5
	totpPeriod       = 30
	recoveryCodeSize = 10
	// Under 112 bits a plain hash is not enough (ASVS 6.5.2), and 128 keeps it simple.
	recoveryCodeBytes = 16
)

// MFARequiredError means the password was right and a second factor is needed.
// No session exists yet.
type MFARequiredError struct {
	Token string
}

func (e *MFARequiredError) Error() string { return "identity: second factor required" }

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// NewChallenge stores a short-lived login step and returns its token. Only a
// hash of the token is stored.
func (s *Service) NewChallenge(ctx context.Context, q db.Querier, kind string, userID *uuid.UUID, data any, ttl time.Duration) (string, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	token := randomToken()
	_, err = q.Exec(ctx, "INSERT INTO auth_challenges (token_hash, kind, user_id, data, expires_at) VALUES ($1, $2, $3, $4, $5)",
		hashToken(token), kind, userID, body, s.now().Add(ttl))
	return token, err
}

// TakeChallenge removes a challenge and returns it. Each can be used only once.
func (s *Service) TakeChallenge(ctx context.Context, q db.Querier, kind, token string, data any) (*uuid.UUID, error) {
	var userID *uuid.UUID
	var body []byte
	var expires time.Time
	err := q.QueryRow(ctx, "DELETE FROM auth_challenges WHERE token_hash = $1 AND kind = $2 RETURNING user_id, data, expires_at",
		hashToken(token), kind).Scan(&userID, &body, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !s.now().Before(expires) {
		return nil, ErrInvalidCredentials
	}
	if data != nil {
		if err := json.Unmarshal(body, data); err != nil {
			return nil, err
		}
	}
	return userID, nil
}

func (s *Service) hasTOTP(ctx context.Context, q db.Querier, userID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM totp_credentials WHERE user_id = $1 AND confirmed_at IS NOT NULL)", userID).Scan(&ok)
	return ok, err
}

func totpContext(userID uuid.UUID) string { return "totp:" + userID.String() }

// TOTPContext is the context a TOTP seed of a person is sealed under, for tools that seal it again.
func TOTPContext(userID uuid.UUID) string { return totpContext(userID) }

// TOTPSetup is shown once to the user while they enrol.
type TOTPSetup struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
	QRCode string `json:"qr_code"` // data: URL of a PNG
}

// BeginTOTP starts enrolment. The user confirms their password first, so a stolen
// session cannot add a second factor. Nothing takes effect until ConfirmTOTP.
func (s *Service) BeginTOTP(ctx context.Context, c Caller, email, password string) (TOTPSetup, error) {
	if s.keyring == nil {
		return TOTPSetup{}, errors.New("identity: no keyring configured")
	}
	if err := s.checkPassword(ctx, s.pool, c.UserID, password); err != nil {
		return TOTPSetup{}, err
	}
	if on, err := s.hasTOTP(ctx, s.pool, c.UserID); err != nil {
		return TOTPSetup{}, err
	} else if on {
		return TOTPSetup{}, apperr.Conflict("totp_active", "An authenticator app is already set up.")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.issuer, AccountName: email, Period: totpPeriod,
		Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return TOTPSetup{}, err
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return TOTPSetup{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return TOTPSetup{}, err
	}
	sealed, err := s.keyring.Seal([]byte(key.Secret()), totpContext(c.UserID))
	if err != nil {
		return TOTPSetup{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO totp_credentials (user_id, secret_sealed) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET secret_sealed = EXCLUDED.secret_sealed, created_at = now()
		WHERE totp_credentials.confirmed_at IS NULL`, c.UserID, sealed)
	if err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: key.Secret(), URL: key.URL(),
		QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// matchTOTP returns the time step the code belongs to, or 0 when it is wrong.
// The window is one step either side, for clocks that differ a little.
func (s *Service) matchTOTP(secret, code string) (int64, error) {
	now := s.now()
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	for _, offset := range []int64{0, -1, 1} {
		at := now.Add(time.Duration(offset*totpPeriod) * time.Second)
		want, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err != nil {
			return 0, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return at.Unix() / totpPeriod, nil
		}
	}
	return 0, nil
}

func (s *Service) totpSecret(ctx context.Context, q db.Querier, userID uuid.UUID) (string, error) {
	var sealed []byte
	if err := q.QueryRow(ctx, "SELECT secret_sealed FROM totp_credentials WHERE user_id = $1", userID).Scan(&sealed); err != nil {
		return "", err
	}
	plain, err := s.keyring.Open(sealed, totpContext(userID))
	return string(plain), err
}

func newRecoveryCode() string {
	b := make([]byte, recoveryCodeBytes)
	_, _ = rand.Read(b)
	s := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	half := len(s) / 2
	return s[:half] + "-" + s[half:]
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), "-", ""), " ", ""))
}

// ConfirmTOTP finishes enrolment with a first code and returns the recovery
// codes, which are shown once and stored only as hashes.
func (s *Service) ConfirmTOTP(ctx context.Context, c Caller, code string) ([]string, error) {
	var codes []string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var confirmed *time.Time
		err := tx.QueryRow(ctx, "SELECT confirmed_at FROM totp_credentials WHERE user_id = $1 FOR UPDATE", c.UserID).Scan(&confirmed)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("totp_not_started", "Start the setup first.")
		}
		if err != nil {
			return err
		}
		if confirmed != nil {
			return apperr.Conflict("totp_active", "An authenticator app is already set up.")
		}
		secret, err := s.totpSecret(ctx, tx, c.UserID)
		if err != nil {
			return err
		}
		step, err := s.matchTOTP(secret, code)
		if err != nil {
			return err
		}
		if step == 0 {
			return apperr.Invalid("That code is not correct.")
		}
		if _, err := tx.Exec(ctx, "UPDATE totp_credentials SET confirmed_at = now(), last_step = $2 WHERE user_id = $1", c.UserID, step); err != nil {
			return err
		}
		if err := s.clearSecondFactorDemand(ctx, tx, c.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM recovery_codes WHERE user_id = $1", c.UserID); err != nil {
			return err
		}
		for range recoveryCodeSize {
			code := newRecoveryCode()
			codes = append(codes, code)
			if _, err := tx.Exec(ctx, "INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, $2)",
				c.UserID, hashToken(normalizeRecoveryCode(code))); err != nil {
				return err
			}
		}
		if err := s.record(ctx, tx, c, "auth.totp_enabled", "user", c.UserID.String(), nil); err != nil {
			return err
		}
		return s.tellOfChange(ctx, tx, c.UserID)
	})
	return codes, err
}

// DisableTOTP removes the authenticator after the user confirms their password.
func (s *Service) DisableTOTP(ctx context.Context, c Caller, password string) error {
	if err := s.checkPassword(ctx, s.pool, c.UserID, password); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return s.removeTOTP(ctx, tx, c, c.UserID, nil)
	})
}

// EndOtherSessions logs a person out everywhere but here. The screen offers it after a
// factor is removed, because a session someone stole survives that (SEC-126).
func (s *Service) EndOtherSessions(ctx context.Context, c Caller, sessionID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1 AND id <> $2", c.UserID, sessionID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, c, "auth.sessions_ended", "user", c.UserID.String(), map[string]any{"sessions": tag.RowsAffected()})
	})
}

// EndSessions logs out one person, by e-mail address, or everyone when email is empty,
// without disabling anyone. It is for an operator during an incident (SEC-178), so it runs
// from the command line as the system, and is audited with how many sessions ended.
func (s *Service) EndSessions(ctx context.Context, actor Actor, email string, meta RequestMeta) (int64, error) {
	var ended int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		target, objectID := "everyone", ""
		query, args := "DELETE FROM sessions", []any{}
		if email != "" {
			var id uuid.UUID
			err := tx.QueryRow(ctx, "SELECT id FROM users WHERE lower(email) = $1", normalizeEmail(email)).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound()
			}
			if err != nil {
				return err
			}
			target, objectID = "one person", id.String()
			query, args = "DELETE FROM sessions WHERE user_id = $1", []any{id}
		}
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return err
		}
		ended = tag.RowsAffected()
		_, err = audit.Record(ctx, tx, event(meta, actor, "sessions.ended", "user", objectID, audit.ResultSuccess,
			map[string]any{"whose": target, "sessions": ended}))
		return err
	})
	return ended, err
}

// ResetMFA removes someone's authenticator, for a lost phone, and ends their sessions:
// an attacker with a session must not outlive the reset of the account (SEC-126). The
// route requires administrator.
func (s *Service) ResetMFA(ctx context.Context, c Caller, id uuid.UUID) error {
	if _, err := s.GetUser(ctx, s.pool, id); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.removeTOTP(ctx, tx, c, id, map[string]any{"by_administrator": true}); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id)
		return err
	})
}

func (s *Service) removeTOTP(ctx context.Context, tx pgx.Tx, c Caller, id uuid.UUID, details map[string]any) error {
	if _, err := tx.Exec(ctx, "DELETE FROM totp_credentials WHERE user_id = $1", id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM recovery_codes WHERE user_id = $1", id); err != nil {
		return err
	}
	if err := s.record(ctx, tx, c, "auth.totp_disabled", "user", id.String(), details); err != nil {
		return err
	}
	return s.tellOfChange(ctx, tx, id)
}

// useSecondFactor checks a TOTP or recovery code inside tx. It returns the method
// that worked, or "" when the code is wrong.
func (s *Service) useSecondFactor(ctx context.Context, tx pgx.Tx, userID uuid.UUID, code string) (string, error) {
	secret, err := s.totpSecret(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	step, err := s.matchTOTP(secret, code)
	if err != nil {
		return "", err
	}
	if step != 0 {
		// A code works once: the step must be newer than the last one used.
		tag, err := tx.Exec(ctx, "UPDATE totp_credentials SET last_step = $2 WHERE user_id = $1 AND last_step < $2", userID, step)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 1 {
			return "totp", nil
		}
		return "", nil
	}
	tag, err := tx.Exec(ctx, "UPDATE recovery_codes SET used_at = now() WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL",
		userID, hashToken(normalizeRecoveryCode(code)))
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 1 {
		return "recovery_code", nil
	}
	return "", nil
}

// CompleteMFA finishes a login with a second factor. The challenge survives a
// few wrong codes, then it is gone and the password step must be repeated.
func (s *Service) CompleteMFA(ctx context.Context, challenge, code string, meta RequestMeta) (string, Principal, error) {
	hash := hashToken(challenge)
	var token string
	var p Principal
	var failedFor *uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var userID *uuid.UUID
		var expires time.Time
		var attempts int
		err := tx.QueryRow(ctx, `SELECT user_id, expires_at, attempts FROM auth_challenges
			WHERE token_hash = $1 AND kind = $2 FOR UPDATE`, hash, challengeMFA).Scan(&userID, &expires, &attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCredentials
		}
		if err != nil {
			return err
		}
		if !s.now().Before(expires) || attempts >= maxMFAAttempts || userID == nil {
			_, _ = tx.Exec(ctx, "DELETE FROM auth_challenges WHERE token_hash = $1", hash)
			return nil // deleted; reported as a failure below
		}
		method, err := s.useSecondFactor(ctx, tx, *userID, code)
		if err != nil {
			return err
		}
		if method == "" {
			failedFor = userID
			_, err := tx.Exec(ctx, "UPDATE auth_challenges SET attempts = attempts + 1 WHERE token_hash = $1", hash)
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM auth_challenges WHERE token_hash = $1", hash); err != nil {
			return err
		}
		if p, err = s.principalByID(ctx, tx, *userID); err != nil {
			return err
		}
		token, p.SessionID, err = s.createSession(ctx, tx, sessionStart{UserID: *userID, Method: "password and code", Meta: meta})
		if err != nil {
			return err
		}
		actor := Actor{Type: audit.ActorUser, ID: userID.String()}
		if method == "recovery_code" {
			if _, err := audit.Record(ctx, tx, event(meta, actor, "auth.recovery_code_used", "user", userID.String(), audit.ResultSuccess, nil)); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, event(meta, actor, "auth.login", "session", p.SessionID.String(), audit.ResultSuccess,
			map[string]any{"method": "password+" + method})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "auth.login_succeeded", map[string]any{"user_id": userID, "session_id": p.SessionID})
		return err
	})
	if err != nil {
		return "", Principal{}, err
	}
	if token == "" {
		objID := ""
		if failedFor != nil {
			objID = failedFor.String()
		}
		e := event(meta, Actor{Type: audit.ActorAnonymous}, "auth.login_mfa", "user", objID, audit.ResultFailure, map[string]any{"reason": "bad_code"})
		if err := audit.RecordStandalone(ctx, s.pool, e); err != nil {
			return "", Principal{}, err
		}
		return "", Principal{}, ErrInvalidCredentials
	}
	return token, p, nil
}

func (s *Service) principalByID(ctx context.Context, q db.Querier, id uuid.UUID) (Principal, error) {
	var p Principal
	err := q.QueryRow(ctx, `SELECT id, type, status, email, display_name, created_at FROM users WHERE id = $1`, id).
		Scan(&p.User.ID, &p.User.Type, &p.User.Status, &p.User.Email, &p.User.DisplayName, &p.User.CreatedAt)
	if err != nil {
		return p, err
	}
	if p.User.Status != StatusActive {
		return p, ErrInvalidCredentials
	}
	return p, nil
}

// OpenSession logs a user in after an external method (OIDC, passkey) proved who
// they are. Disabled accounts are refused here, for every method.
func (s *Service) OpenSession(ctx context.Context, tx pgx.Tx, userID uuid.UUID, method string, meta RequestMeta) (string, Principal, error) {
	p, err := s.principalByID(ctx, tx, userID)
	if err != nil {
		return "", p, err
	}
	token, id, err := s.createSession(ctx, tx, sessionStart{UserID: userID, Method: method, Meta: meta})
	if err != nil {
		return "", p, err
	}
	p.SessionID = id
	actor := Actor{Type: audit.ActorUser, ID: userID.String()}
	if _, err := audit.Record(ctx, tx, event(meta, actor, "auth.login", "session", id.String(), audit.ResultSuccess, map[string]any{"method": method})); err != nil {
		return "", p, err
	}
	_, err = outbox.Enqueue(ctx, tx, "auth.login_succeeded", map[string]any{"user_id": userID, "session_id": id})
	return token, p, err
}

// RecordLoginFailure audits a failed login that did not involve a password.
func (s *Service) RecordLoginFailure(ctx context.Context, method, reason string, meta RequestMeta) error {
	return audit.RecordStandalone(ctx, s.pool, event(meta, Actor{Type: audit.ActorAnonymous}, "auth.login", "user", "", audit.ResultFailure,
		map[string]any{"method": method, "reason": reason}))
}

// Pool exposes the connection pool to the login modules (sso, passkeys).
func (s *Service) Pool() Beginner { return s.pool }

// Beginner starts transactions.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PurgeExpired removes finished sessions and challenges. Runs in the worker.
func (s *Service) PurgeExpired(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE expires_at < now() OR last_seen_at < now() - make_interval(secs => $1)", s.idle.Seconds()); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM auth_challenges WHERE expires_at < now()")
	return err
}
