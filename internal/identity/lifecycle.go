// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity/leaked"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
)

// The single-purpose links of an account's life. They live in auth_challenges, so
// they are hashed, expire, and work once.
const (
	challengeReset      = "password_reset"
	challengeInvitation = "invitation"
	challengeSignup     = "signup"

	resetTTL      = time.Hour
	invitationTTL = 7 * 24 * time.Hour
	signupTTL     = 24 * time.Hour

	minNamePartForPassword = 4
)

// The kinds of mail an account sends.
const (
	MailReset      = "password_reset"
	MailInvitation = "invitation"
	MailSignup     = "signup"
	MailNewDevice  = "new_device"
	// MailSignInChanged says that a password, a second factor or a passkey changed.
	MailSignInChanged = "sign_in_changed"
	// MailPasswordLocked says that password login stopped after too many wrong passwords.
	MailPasswordLocked = "password_locked"
)

// AccountMail is one message about an account. Link holds a secret, so it must
// never reach a log.
type AccountMail struct {
	To   string
	Kind string
	Link string
}

// Mailer sends account mail. The mail queue of wave C implements it.
type Mailer interface {
	Send(ctx context.Context, m AccountMail) error
}

// SetMailer turns on everything that needs mail. Without it, reset links and
// sign-up stay hidden, and invitations show their link to the administrator.
func (s *Service) SetMailer(m Mailer, publicOrigin string) {
	s.mailer, s.origin = m, publicOrigin
}

// MailReady says whether account mail can be sent.
func (s *Service) MailReady() bool { return s.mailer != nil }

func (s *Service) send(ctx context.Context, m AccountMail) error {
	if s.mailer == nil {
		return nil
	}
	return s.mailer.Send(ctx, m)
}

// tellOfChange mails a person that a way into their account changed (ASVS 6.3.7, SEC-127),
// so someone who did not make the change learns of it while they can still act.
func (s *Service) tellOfChange(ctx context.Context, q db.Querier, userID uuid.UUID) error {
	var email string
	if err := q.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", userID).Scan(&email); err != nil {
		return err
	}
	return s.send(ctx, AccountMail{To: email, Kind: MailSignInChanged})
}

// Policy holds the rules an administrator sets. The settings package depends on
// this one, so the rules are handed in as functions.
type Policy struct {
	IdleLimit    func(context.Context) time.Duration
	SecondFactor func(context.Context) string
}

// SetPolicy sets where the rules come from.
func (s *Service) SetPolicy(p Policy) {
	s.idleSetting, s.secondFactorRule = p.IdleLimit, p.SecondFactor
}

// LoadBlocklist reads known bad passwords from a file of an operator, one per
// line. An empty path means none.
func LoadBlocklist(path string) (map[string]bool, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path) // #nosec G304 -- the path comes from the operator's configuration
	if err != nil {
		return nil, fmt.Errorf("identity: read password blocklist: %v", err)
	}
	defer func() { _ = f.Close() }()
	out := map[string]bool{}
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		if line := strings.TrimSpace(scan.Text()); line != "" {
			out[strings.ToLower(line)] = true
		}
	}
	return out, scan.Err()
}

// productWord may not appear in a password.
const productWord = "diffluence"

// validateNewPassword applies every rule to a new password. NIST SP 800-63B asks
// for a check against known bad passwords, and not for odd characters.
func (s *Service) validateNewPassword(password, email, name string) error {
	if err := ValidatePassword(password); err != nil {
		return toInvalid(err)
	}
	lower := strings.ToLower(password)
	filter, err := leaked.Embedded()
	if err != nil {
		return err
	}
	if s.blocklist[lower] || filter.Contains(lower) {
		return apperr.Invalid("This password is on a list of leaked passwords. Choose another one.")
	}
	local, _, _ := strings.Cut(strings.ToLower(email), "@")
	// The name of the product is the first word anyone tries against a login here (SEC-106).
	parts := append(strings.Fields(strings.ToLower(name)), local, productWord)
	for _, part := range parts {
		if utf8.RuneCountInString(part) >= minNamePartForPassword && strings.Contains(lower, part) {
			return apperr.Invalid("A password cannot hold your name or e-mail address.")
		}
	}
	return nil
}

type sessionStart struct {
	UserID uuid.UUID
	Method string
	Meta   RequestMeta
}

// noteLogin keeps the history a person can read, and warns about a new device.
func (s *Service) noteLogin(ctx context.Context, tx pgx.Tx, in sessionStart) error {
	if _, err := tx.Exec(ctx, "INSERT INTO login_history (id, user_id, method, ip, user_agent) VALUES ($1, $2, $3, $4, $5)",
		ids.New(), in.UserID, in.Method, in.Meta.IP, truncate(in.Meta.UserAgent, 512)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM login_history WHERE user_id = $1 AND id NOT IN
		(SELECT id FROM login_history WHERE user_id = $1 ORDER BY at DESC LIMIT 20)`, in.UserID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET last_login_at = $2 WHERE id = $1", in.UserID, s.now()); err != nil {
		return err
	}
	var seenBefore bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM known_devices WHERE user_id = $1)", in.UserID).Scan(&seenBefore); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO known_devices (user_id, device_hash) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		in.UserID, hashToken(in.Meta.UserAgent))
	if err != nil || !seenBefore || tag.RowsAffected() == 0 {
		return err
	}
	var email string
	if err := tx.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", in.UserID).Scan(&email); err != nil {
		return err
	}
	return s.send(ctx, AccountMail{To: email, Kind: MailNewDevice})
}

// needsSecondFactor says whether a person must set up a second factor before they
// can do anything else. Accounts of an identity provider are exempt: their login
// is that provider's business.
func (s *Service) needsSecondFactor(ctx context.Context, q db.Querier, userID uuid.UUID) (bool, error) {
	rule := ""
	if s.secondFactorRule != nil {
		rule = s.secondFactorRule(ctx)
	}
	if rule == "" || rule == "none" {
		return false, nil
	}
	var needed bool
	err := q.QueryRow(ctx, `SELECT u.source = 'local'
			AND ($2 = 'members' OR perm_has_global(u.id, 'administrator'))
			AND NOT EXISTS (SELECT 1 FROM totp_credentials t WHERE t.user_id = u.id AND t.confirmed_at IS NOT NULL)
			AND NOT EXISTS (SELECT 1 FROM webauthn_credentials w WHERE w.user_id = u.id)
		FROM users u WHERE u.id = $1`, userID, rule).Scan(&needed)
	return needed, err
}

// ClearSecondFactorDemand lifts the limit on a person's sessions, once they have a factor.
func (s *Service) clearSecondFactorDemand(ctx context.Context, q db.Querier, userID uuid.UUID) error {
	_, err := q.Exec(ctx, "UPDATE sessions SET needs_second_factor = false WHERE user_id = $1", userID)
	return err
}

// How long a person can put off the question about a stronger sign-in. An
// administrator is asked again sooner (product owner, 2026-11-05).
const (
	signInPromptDelay      = 7 * 24 * time.Hour
	signInPromptAdminDelay = 24 * time.Hour
)

// SignIn says how strong a person's sign-in is, and whether to ask them now.
type SignIn struct {
	// Weak means a password alone. The account screen always says so.
	Weak bool `json:"weak"`
	// Prompt asks for a second factor or passkey now. It can be put off, and it
	// cannot be switched off.
	Prompt bool `json:"prompt"`
}

// SignInStatus reads how a person signs in. An account of an identity provider
// is not asked: its login is that provider's business.
func (s *Service) SignInStatus(ctx context.Context, userID uuid.UUID) (SignIn, error) {
	var out SignIn
	var after *time.Time
	err := s.pool.QueryRow(ctx, `SELECT u.source = 'local' AND u.type <> 'service'
			AND NOT EXISTS (SELECT 1 FROM totp_credentials t WHERE t.user_id = u.id AND t.confirmed_at IS NOT NULL)
			AND NOT EXISTS (SELECT 1 FROM webauthn_credentials w WHERE w.user_id = u.id),
			u.sign_in_prompt_after
		FROM users u WHERE u.id = $1`, userID).Scan(&out.Weak, &after)
	if err != nil {
		return out, err
	}
	out.Prompt = out.Weak && (after == nil || !s.now().Before(*after))
	return out, nil
}

// PostponeSignInPrompt puts the question off. The delay is fixed here, so no
// client can turn it into "never".
func (s *Service) PostponeSignInPrompt(ctx context.Context, c Caller) error {
	admin, err := s.HasGlobalPermission(ctx, c.UserID, PermAdministrator)
	if err != nil {
		return err
	}
	delay := signInPromptDelay
	if admin {
		delay = signInPromptAdminDelay
	}
	_, err = s.pool.Exec(ctx, "UPDATE users SET sign_in_prompt_after = $2 WHERE id = $1", c.UserID, s.now().Add(delay))
	return err
}

// LoginEntry is one entry of a person's login history.
type LoginEntry struct {
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	At        time.Time `json:"at"`
}

// LoginHistory lists a person's last logins.
func (s *Service) LoginHistory(ctx context.Context, userID uuid.UUID) ([]LoginEntry, error) {
	rows, err := s.pool.Query(ctx, "SELECT method, ip, user_agent, at FROM login_history WHERE user_id = $1 ORDER BY at DESC LIMIT 20", userID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (LoginEntry, error) {
		var e LoginEntry
		err := r.Scan(&e.Method, &e.IP, &e.UserAgent, &e.At)
		return e, err
	})
	if out == nil {
		out = []LoginEntry{}
	}
	return out, err
}

func (s *Service) link(path, token string) string { return s.origin + path + "?token=" + token }

// RequestReset sends a reset link when the address belongs to an account with a
// password. It never says whether it does.
func (s *Service) RequestReset(ctx context.Context, email string, meta RequestMeta) error {
	email = normalizeEmail(email)
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT u.id FROM users u WHERE lower(u.email) = $1 AND u.status = 'active'
		AND u.source = 'local' AND u.type <> 'service' AND (u.expires_at IS NULL OR u.expires_at > now())`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM auth_challenges WHERE kind = $1 AND user_id = $2", challengeReset, id); err != nil {
		return err
	}
	token, err := s.NewChallenge(ctx, s.pool, challengeReset, &id, map[string]any{}, resetTTL)
	if err != nil {
		return err
	}
	if err := audit.RecordStandalone(ctx, s.pool, event(meta, Actor{Type: audit.ActorAnonymous}, "auth.reset_requested", "user", id.String(), audit.ResultSuccess, nil)); err != nil {
		return err
	}
	return s.send(ctx, AccountMail{To: email, Kind: MailReset, Link: s.link("/reset", token)})
}

// ResetWithLink sets a new password from a reset link, and ends every session. It
// opens no session: the second factor still applies at the next login.
func (s *Service) ResetWithLink(ctx context.Context, token, password string, meta RequestMeta) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		userID, err := s.TakeChallenge(ctx, tx, challengeReset, token, nil)
		if errors.Is(err, ErrInvalidCredentials) || (err == nil && userID == nil) {
			return apperr.Conflict("link_invalid", "This link is no longer valid. Ask for a new one.")
		}
		if err != nil {
			return err
		}
		u, err := s.GetUser(ctx, tx, *userID)
		if err != nil {
			return err
		}
		if u.Status != StatusActive {
			return apperr.Conflict("link_invalid", "This link is no longer valid. Ask for a new one.")
		}
		if err := s.validateNewPassword(password, u.Email, u.DisplayName); err != nil {
			return err
		}
		if err := s.storePassword(ctx, tx, u.ID, password); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", u.ID); err != nil {
			return err
		}
		actor := Actor{Type: audit.ActorUser, ID: u.ID.String()}
		if _, err := audit.Record(ctx, tx, event(meta, actor, "auth.password_reset", "user", u.ID.String(), audit.ResultSuccess, nil)); err != nil {
			return err
		}
		if _, err := outbox.Enqueue(ctx, tx, "auth.password_reset", map[string]any{"user_id": u.ID}); err != nil {
			return err
		}
		return s.tellOfChange(ctx, tx, u.ID)
	})
}

// Invitation asks someone to make an account.
type Invitation struct {
	ID        string      `json:"id"`
	Email     string      `json:"email"`
	Type      string      `json:"type"`
	Groups    []uuid.UUID `json:"groups"`
	SpaceID   *uuid.UUID  `json:"space_id"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type invitationData struct {
	Email   string      `json:"email"`
	Type    string      `json:"type"`
	Groups  []uuid.UUID `json:"groups"`
	SpaceID *uuid.UUID  `json:"space_id"`
}

// NewInvitation is what an administrator fills in.
type NewInvitation struct {
	Email   string
	Type    string
	Groups  []uuid.UUID
	SpaceID *uuid.UUID
}

// InvitationAddress cleans an address someone typed for an invitation.
func InvitationAddress(email string) (string, error) {
	address := normalizeEmail(email)
	if !validEmail(address) {
		return "", apperr.Invalid("That is not an e-mail address.")
	}
	return address, nil
}

// Invite stores an invitation, and sends it. Without mail it returns the link, so
// the administrator can pass it on.
func (s *Service) Invite(ctx context.Context, c Caller, in NewInvitation) (string, error) {
	var link string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		link, err = s.InviteWithin(ctx, tx, c, in)
		return err
	})
	return link, err
}

// InviteWithin is Invite inside a transaction of the caller. The mail is queued before
// that transaction ends, so a failure to queue it undoes everything.
func (s *Service) InviteWithin(ctx context.Context, tx pgx.Tx, c Caller, in NewInvitation) (string, error) {
	address, err := InvitationAddress(in.Email)
	if err != nil {
		return "", err
	}
	in.Email = address
	if in.Type != TypeMember && in.Type != TypeGuest {
		return "", apperr.Invalid("A person is invited as a member or a guest.")
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)", in.Email).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "", apperr.Conflict("email_taken", "This e-mail address already has an account.")
	}
	if _, err := tx.Exec(ctx, "DELETE FROM auth_challenges WHERE kind = $1 AND data->>'email' = $2", challengeInvitation, in.Email); err != nil {
		return "", err
	}
	token, err := s.NewChallenge(ctx, tx, challengeInvitation, nil, invitationData(in), invitationTTL)
	if err != nil {
		return "", err
	}
	if err := s.record(ctx, tx, c, "invitation.created", "invitation", "", map[string]any{"type": in.Type}); err != nil {
		return "", err
	}
	link := s.link("/invitation", token)
	if !s.MailReady() {
		return link, nil
	}
	return "", s.send(ctx, AccountMail{To: in.Email, Kind: MailInvitation, Link: link})
}

// Invitations lists what is waiting for an answer.
func (s *Service) Invitations(ctx context.Context) ([]Invitation, error) {
	rows, err := s.pool.Query(ctx, `SELECT token_hash, data->>'email', data->>'type', expires_at FROM auth_challenges
		WHERE kind = $1 AND expires_at > now() ORDER BY expires_at DESC LIMIT 500`, challengeInvitation)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Invitation, error) {
		var v Invitation
		var hash []byte
		err := r.Scan(&hash, &v.Email, &v.Type, &v.ExpiresAt)
		v.ID = hex.EncodeToString(hash)
		return v, err
	})
	if out == nil {
		out = []Invitation{}
	}
	return out, err
}

// CancelInvitation ends an invitation.
func (s *Service) CancelInvitation(ctx context.Context, c Caller, id string) error {
	hash, err := hex.DecodeString(id)
	if err != nil {
		return apperr.NotFound()
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM auth_challenges WHERE token_hash = $1 AND kind = $2", hash, challengeInvitation)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return s.record(ctx, tx, c, "invitation.cancelled", "invitation", "", nil)
	})
}

// InvitationFor reads an invitation for the person who holds the link.
func (s *Service) InvitationFor(ctx context.Context, token string) (string, error) {
	var email string
	var expires time.Time
	err := s.pool.QueryRow(ctx, "SELECT data->>'email', expires_at FROM auth_challenges WHERE token_hash = $1 AND kind = $2",
		hashToken(token), challengeInvitation).Scan(&email, &expires)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !s.now().Before(expires)) {
		return "", apperr.Conflict("link_invalid", "This link is no longer valid. Ask for a new one.")
	}
	return email, err
}

// Accepted is what a new account needs to finish setting up.
type Accepted struct {
	UserID  uuid.UUID
	SpaceID *uuid.UUID
}

// NewAccount is what a person fills in on an invitation or sign-up page.
type NewAccount struct {
	Token       string
	DisplayName string
	Password    string
}

// AcceptInvitation makes the account. grant runs in the same transaction, so a
// guest gets their space or no account at all.
func (s *Service) AcceptInvitation(ctx context.Context, in NewAccount, meta RequestMeta, grant func(pgx.Tx, Accepted) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var data invitationData
		_, err := s.TakeChallenge(ctx, tx, challengeInvitation, in.Token, &data)
		if errors.Is(err, ErrInvalidCredentials) {
			return apperr.Conflict("link_invalid", "This link is no longer valid. Ask for a new one.")
		}
		if err != nil {
			return err
		}
		id, err := s.newLocalAccount(ctx, tx, data, in, meta)
		if err != nil {
			return err
		}
		return grant(tx, Accepted{UserID: id, SpaceID: data.SpaceID})
	})
}

func (s *Service) newLocalAccount(ctx context.Context, tx pgx.Tx, data invitationData, in NewAccount, meta RequestMeta) (uuid.UUID, error) {
	name := strings.TrimSpace(in.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return uuid.Nil, apperr.Invalid("A name has 1 to 200 characters.")
	}
	if err := s.validateNewPassword(in.Password, data.Email, name); err != nil {
		return uuid.Nil, err
	}
	id := ids.New()
	_, err := tx.Exec(ctx, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
		VALUES ($1, $2, $3, 'active', $4, $5)`, id, s.instanceID, data.Type, data.Email, name)
	if db.IsUniqueViolation(err) {
		return uuid.Nil, apperr.Conflict("email_taken", "This e-mail address already has an account.")
	}
	if err != nil {
		return uuid.Nil, err
	}
	if err := s.storePassword(ctx, tx, id, in.Password); err != nil {
		return uuid.Nil, err
	}
	for _, p := range defaultPermissions(data.Type) {
		if _, err := tx.Exec(ctx, "SELECT rbac_add_permissions(NULL, 'user', $1, ARRAY[$2::text])", id, p); err != nil {
			return uuid.Nil, err
		}
	}
	for _, g := range data.Groups {
		if _, err := tx.Exec(ctx, "INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", g, id); err != nil {
			return uuid.Nil, err
		}
	}
	actor := Actor{Type: audit.ActorUser, ID: id.String()}
	if _, err := audit.Record(ctx, tx, event(meta, actor, "user.created", "user", id.String(), audit.ResultSuccess, map[string]any{"email": data.Email, "type": data.Type, "auth": "invitation"})); err != nil {
		return uuid.Nil, err
	}
	_, err = outbox.Enqueue(ctx, tx, "user.created", map[string]any{"user_id": id, "type": data.Type})
	return id, err
}

// RequestSignup proves an address before an account exists. The answer never
// says whether the address is known or the domain allowed.
func (s *Service) RequestSignup(ctx context.Context, email string, domains []string) error {
	email = normalizeEmail(email)
	_, domain, _ := strings.Cut(email, "@")
	if !validEmail(email) || !slices.Contains(domains, domain) {
		return nil
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)", email).Scan(&exists); err != nil || exists {
		return err
	}
	token, err := s.NewChallenge(ctx, s.pool, challengeSignup, nil, invitationData{Email: email, Type: TypeMember}, signupTTL)
	if err != nil {
		return err
	}
	return s.send(ctx, AccountMail{To: email, Kind: MailSignup, Link: s.link("/signup", token)})
}

// ConfirmSignup makes the account from a proven address.
func (s *Service) ConfirmSignup(ctx context.Context, in NewAccount, meta RequestMeta) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var data invitationData
		_, err := s.TakeChallenge(ctx, tx, challengeSignup, in.Token, &data)
		if errors.Is(err, ErrInvalidCredentials) {
			return apperr.Conflict("link_invalid", "This link is no longer valid. Ask for a new one.")
		}
		if err != nil {
			return err
		}
		_, err = s.newLocalAccount(ctx, tx, data, in, meta)
		return err
	})
}

// CreateServiceAccount makes an account for a script. It can never log in through
// a browser, and only works through tokens an administrator creates.
func (s *Service) CreateServiceAccount(ctx context.Context, c Caller, name string) (UserDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return UserDetail{}, apperr.Invalid("A name has 1 to 200 characters.")
	}
	id := ids.New()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
			VALUES ($1, $2, 'service', 'active', $3, $4)`, id, s.instanceID, "service-"+id.String()+"@service.invalid", name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT rbac_add_permissions(NULL, 'user', $1, ARRAY[$2::text])", id, PermUseProduct); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "user.created", "user", id.String(), map[string]any{"type": TypeService})
	})
	if err != nil {
		return UserDetail{}, err
	}
	return s.GetUser(ctx, s.pool, id)
}

// CreateServiceToken makes a token for a service account.
func (s *Service) CreateServiceToken(ctx context.Context, c Caller, id uuid.UUID, in NewToken) (Token, string, error) {
	u, err := s.GetUser(ctx, s.pool, id)
	if err != nil {
		return Token{}, "", err
	}
	if u.Type != TypeService {
		return Token{}, "", apperr.Conflict("not_a_service_account", "Only a service account gets a token from an administrator.")
	}
	return s.createTokenFor(ctx, c, id, in)
}

// SetExpiry sets the date an account ends. Nil means it does not end.
func (s *Service) SetExpiry(ctx context.Context, c Caller, id uuid.UUID, at *time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE users SET expires_at = $2, updated_at = now() WHERE id = $1", id, at)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return s.record(ctx, tx, c, "user.expiry_changed", "user", id.String(), map[string]any{"expires_at": at})
	})
}

// EndExpiredAccounts disables accounts past their end date and ends their
// sessions. The rule already treats them as not active: this only tidies up.
func (s *Service) EndExpiredAccounts(ctx context.Context) error {
	c := SystemCaller("account-expiry")
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE users SET status = 'disabled', updated_at = now()
			WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at <= now() RETURNING id`)
		if err != nil {
			return err
		}
		ended, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		if len(ended) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = ANY($1)", ended); err != nil {
			return err
		}
		for _, id := range ended {
			if err := s.record(ctx, tx, c, "user.expired", "user", id.String(), nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// anonymisedDomain cannot receive mail, by RFC 2606.
const anonymisedDomain = "anonymised.invalid"

// Anonymise removes a person's identity and keeps their work. Audit rows and
// backups stay as they are: the dialog says so (spec 013, D10).
func (s *Service) Anonymise(ctx context.Context, c Caller, id uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		return apperr.Invalid("Give a reason of at most 500 characters.")
	}
	if c.UserID == id {
		return apperr.Conflict("cannot_anonymise_self", "You cannot anonymise your own account.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		u, err := s.GetUser(ctx, tx, id)
		if err != nil {
			return err
		}
		code := strings.ReplaceAll(id.String(), "-", "")[:8]
		if _, err := tx.Exec(ctx, `UPDATE users SET display_name = $2, email = $3, username = '', external_id = '',
			status = 'disabled', anonymised_at = now(), updated_at = now() WHERE id = $1`,
			id, "Former user "+code, "former-"+id.String()+"@"+anonymisedDomain); err != nil {
			return err
		}
		for _, table := range personalTables {
			if err := deleteRowsOf(ctx, tx, table, id); err != nil {
				return err
			}
		}
		if err := forgetLinksOf(ctx, tx, id); err != nil {
			return err
		}
		if err := audit.ErasePersonal(ctx, tx, id.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM auth_challenges WHERE user_id = $1 OR data->>'email' = $2", id, u.Email); err != nil {
			return err
		}
		if err := s.keepsAnAdministrator(ctx, tx, c); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "user.anonymised", "user", id.String(), map[string]any{"reason": reason})
	})
}

// personalTables hold nothing but what is about this person, by a user_id column. A
// test walks every uuid column, so a new table cannot be forgotten here (PRM-176).
var personalTables = []string{"sessions", "api_tokens", "local_credentials", "totp_credentials", "recovery_codes",
	"webauthn_credentials", "user_identities", "login_history", "known_devices", "user_preferences", "group_owners",
	"group_members", "follows", "watches", "comment_seen", "notifications", "mail_queue", "favourites", "recent_pages",
	"space_favourites", "drafts"}

// forgetLinksOf removes where others point at this person, without taking from others
// more than that link.
func forgetLinksOf(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	for _, statement := range []string{
		"DELETE FROM follows WHERE followed_id = $1",
		"DELETE FROM access_requests WHERE requester_id = $1",
		"DELETE FROM invitation_requests WHERE requester_id = $1",
		"UPDATE notifications SET actor_id = NULL WHERE actor_id = $1",
	} {
		if _, err := tx.Exec(ctx, statement, id); err != nil {
			return err
		}
	}
	return nil
}

// deleteRowsOf empties one person's rows. The table names come from the list above.
func deleteRowsOf(ctx context.Context, tx pgx.Tx, table string, id uuid.UUID) error {
	_, err := tx.Exec(ctx, "DELETE FROM "+pgx.Identifier{table}.Sanitize()+" WHERE user_id = $1", id)
	return err
}

// PersonalData is what a person can take with them.
type PersonalData struct {
	Account     UserDetail    `json:"account"`
	Preferences Preferences   `json:"preferences"`
	Groups      []string      `json:"groups"`
	Sessions    []SessionInfo `json:"sessions"`
	Logins      []LoginEntry  `json:"logins"`
}

// ExportPersonalData collects what the account itself holds. Content comes from
// the modules that own it.
func (s *Service) ExportPersonalData(ctx context.Context, id uuid.UUID) (PersonalData, error) {
	var out PersonalData
	var err error
	if out.Account, err = s.GetUser(ctx, s.pool, id); err != nil {
		return out, err
	}
	if out.Preferences, err = s.Preferences(ctx, id); err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT g.name FROM group_members m JOIN groups g ON g.id = m.group_id
		WHERE m.user_id = $1 ORDER BY lower(g.name)`, id)
	if err != nil {
		return out, err
	}
	if out.Groups, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return out, err
	}
	if out.Sessions, err = s.Sessions(ctx, id, uuid.Nil); err != nil {
		return out, err
	}
	out.Logins, err = s.LoginHistory(ctx, id)
	return out, err
}

// RequestRemoval records that a person asks to be removed. Administrators see the
// request, and decide.
func (s *Service) RequestRemoval(ctx context.Context, c Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return s.record(ctx, tx, c, "user.removal_requested", "user", c.UserID.String(), nil)
	})
}

// TypeImpact says what a change between member and guest does, before it happens.
type TypeImpact struct {
	Spaces      []string `json:"spaces"`
	Permissions []string `json:"permissions"`
}

// ChangeImpact lists the spaces someone reaches only through "all members" or
// "all guests", and the global permissions that go with the current type.
func (s *Service) ChangeImpact(ctx context.Context, id uuid.UUID) (TypeImpact, error) {
	u, err := s.GetUser(ctx, s.pool, id)
	if err != nil {
		return TypeImpact{}, err
	}
	subject := "all_members"
	if u.Type == TypeGuest {
		subject = "all_guests"
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT sp.name FROM space_grants g JOIN spaces sp ON sp.id = g.space_id
		WHERE g.subject_type = $1 AND g.permission = 'space.view' AND sp.status <> 'trashed' ORDER BY sp.name LIMIT 200`, subject)
	if err != nil {
		return TypeImpact{}, err
	}
	spaces, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return TypeImpact{}, err
	}
	if spaces == nil {
		spaces = []string{}
	}
	return TypeImpact{Spaces: spaces, Permissions: u.GlobalPermissions}, nil
}

// Reach is what an administrator sees of one person: groups, the spaces they can
// open and how many pages they own. Titles stay out, because administrators read no
// content by default (ADM-010, ADM-012).
type Reach struct {
	Groups     []string `json:"groups"`
	Spaces     []string `json:"spaces"`
	PagesOwned int      `json:"pages_owned"`
}

// maxReachRows keeps one person's overview to a screenful.
const maxReachRows = 200

// UserReach reads the overview of one person. The route requires administrator.
func (s *Service) UserReach(ctx context.Context, id uuid.UUID) (Reach, error) {
	if _, err := s.GetUser(ctx, s.pool, id); err != nil {
		return Reach{}, err
	}
	out := Reach{Groups: []string{}, Spaces: []string{}}
	err := s.pool.QueryRow(ctx, `SELECT
			COALESCE((SELECT array_agg(name ORDER BY name) FROM (SELECT g.name FROM group_members m
				JOIN groups g ON g.id = m.group_id WHERE m.user_id = $1 ORDER BY g.name LIMIT $2) gs), '{}'),
			COALESCE((SELECT array_agg(key ORDER BY key) FROM (SELECT s.key FROM perm_spaces($1, 'space.view') ps
				JOIN spaces s ON s.id = ps.space_id ORDER BY s.key LIMIT $2) ss), '{}'),
			(SELECT count(*) FROM pages p WHERE COALESCE(p.owner_id, p.created_by) = $1 AND p.status = 'published')`,
		id, maxReachRows).Scan(&out.Groups, &out.Spaces, &out.PagesOwned)
	return out, err
}
