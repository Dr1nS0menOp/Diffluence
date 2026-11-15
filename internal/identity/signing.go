// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
)

// Electronic signatures (spec 064 wave DC4, blueprint 09.8): who may sign, and the fresh
// confirmation of identity that every single signature needs. The pages module asks for a
// confirmation for one purpose, the person confirms it with two components, and the
// signature takes it inside its own transaction, once.

// How a person confirmed who they are for a signature (DOC-710).
const (
	SigningByPasswordAndCode = "password_and_code"
	SigningByPasskey         = "passkey"
	SigningByProvider        = "identity_provider"
)

const (
	// SigningConfirmationTTL is how long a confirmation counts: for one signature, within two
	// minutes (DOC-730, DOC-1416). It is separate from ReauthWindow.
	SigningConfirmationTTL = 2 * time.Minute
	// signingBeginTTL is how long a person has to confirm once the signing dialog opened,
	// long enough for a login at the identity provider.
	signingBeginTTL = 10 * time.Minute
	// maxPurposeRunes bounds the purpose a module binds a confirmation to.
	maxPurposeRunes = 300
)

// How an administrator confirmed who a person is before enabling signing (DOC-751). A
// choice, never free text: the audit chain keeps it for good, so it must hold no number of
// an identity document.
const (
	IdentifiedInPerson      = "in_person"
	IdentifiedByVideo       = "video_call"
	IdentifiedByHRRecords   = "hr_records"
	IdentifiedAsKnownPerson = "known_personally"
)

// IdentificationMethods lists the ways an administrator can confirm who a person is.
var IdentificationMethods = []string{IdentifiedInPerson, IdentifiedByVideo, IdentifiedByHRRecords, IdentifiedAsKnownPerson}

func signingRefused(code, message string) *apperr.Error {
	return &apperr.Error{Kind: apperr.KindForbidden, Code: code, Message: message}
}

// errSigningNeedsPerson refuses tokens, automation, the system and sessions limited to setting
// up a second factor: a signature is a person's act at a session of their own (DOC-733).
func errSigningNeedsPerson() *apperr.Error {
	return signingRefused("signing_needs_person", "Only a person signs, in a session of their own: never a token, a rule or the system.")
}

func errSigningConfirmation() *apperr.Error {
	return signingRefused("signing_confirmation_invalid", "This confirmation was used already, or belongs to another signature. Confirm who you are again.")
}

// SigningRight says whether a person may sign, and who enabled it.
type SigningRight struct {
	Enabled      bool       `json:"enabled"`
	EnabledBy    string     `json:"enabled_by"`
	IdentifiedBy string     `json:"identified_by"`
	EnabledAt    *time.Time `json:"enabled_at"`
}

// MaySign says whether a person may sign now: an active member whose signing an
// administrator enabled (DOC-014, DOC-751). Document control asks it before a delegation or a
// stand-in hands someone a step that is signed (DOC-240).
func (s *Service) MaySign(ctx context.Context, q db.Querier, id uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM signing_rights r JOIN users u ON u.id = r.user_id
		WHERE r.user_id = $1 AND u.type = $2 AND u.status = $3)`, id, TypeMember, StatusActive).Scan(&ok)
	return ok, err
}

// SigningRightOf reads whether a person may sign.
func (s *Service) SigningRightOf(ctx context.Context, id uuid.UUID) (SigningRight, error) {
	var r SigningRight
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(a.display_name, ''), r.identified_by, r.enabled_at
		FROM signing_rights r LEFT JOIN users a ON a.id = r.enabled_by WHERE r.user_id = $1`, id).Scan(&r.EnabledBy, &r.IdentifiedBy, &r.EnabledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SigningRight{}, nil
	}
	r.Enabled = err == nil
	return r, err
}

// EnableSigning lets a person sign. The administrator says how they confirmed who the
// person is, and the audit entry names both (DOC-751). An administrator never enables
// their own signing, and a guest or a service account never signs (DOC-1422).
func (s *Service) EnableSigning(ctx context.Context, c Caller, id uuid.UUID, identifiedBy string) error {
	if c.System != "" || c.Rule != nil || c.UserID == uuid.Nil {
		return errSigningNeedsPerson()
	}
	if !slices.Contains(IdentificationMethods, identifiedBy) {
		return apperr.Invalid("Say how you confirmed who this person is.")
	}
	if c.UserID == id {
		return apperr.Conflict("own_signing", "Another administrator enables signing for you, after confirming who you are.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		u, err := s.GetUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.mayActOn(ctx, tx, c, id); err != nil {
			return err
		}
		if u.Type != TypeMember || u.Status != StatusActive {
			return apperr.Conflict("cannot_sign", "Only an active member can sign: never a guest or a service account.")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO signing_rights (user_id, enabled_by, identified_by) VALUES ($1, $2, $3)
			ON CONFLICT (user_id) DO UPDATE SET enabled_by = EXCLUDED.enabled_by, identified_by = EXCLUDED.identified_by, enabled_at = now()`,
			id, c.UserID, identifiedBy); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "user.signing_enabled", "user", id.String(), map[string]any{"identified_by": identifiedBy})
	})
}

// DisableSigning takes the right to sign away. The signatures made stay and still verify
// (DOC-750).
func (s *Service) DisableSigning(ctx context.Context, c Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.mayActOn(ctx, tx, c, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM signing_rights WHERE user_id = $1", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("signing_not_enabled", "Signing is not enabled for this person.")
		}
		if _, err := tx.Exec(ctx, "DELETE FROM signing_confirmations WHERE user_id = $1", id); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "user.signing_disabled", "user", id.String(), nil)
	})
}

// requireSigner refuses everyone but an active member with signing enabled, acting in a
// live session of their own (DOC-733, DOC-751, DOC-1422).
func (s *Service) requireSigner(ctx context.Context, q db.Querier, c Caller, sessionID uuid.UUID) error {
	if c.System != "" || c.Rule != nil || c.UserID == uuid.Nil || sessionID == uuid.Nil {
		return errSigningNeedsPerson()
	}
	var live, enabled bool
	err := q.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM sessions WHERE id = $1 AND user_id = $2 AND expires_at > now() AND NOT needs_second_factor),
			EXISTS (SELECT 1 FROM signing_rights r JOIN users u ON u.id = r.user_id
				WHERE r.user_id = $2 AND u.type = 'member' AND u.status = 'active' AND (u.expires_at IS NULL OR u.expires_at > now()))`,
		sessionID, c.UserID).Scan(&live, &enabled)
	if err != nil {
		return err
	}
	if !live {
		return errSigningNeedsPerson()
	}
	if !enabled {
		return signingRefused("signing_not_enabled", "Signing is not enabled for you. An administrator enables it after confirming who you are.")
	}
	return nil
}

// SigningRequest is one signature a module asks a confirmation for.
type SigningRequest struct {
	SessionID uuid.UUID
	// Purpose binds the confirmation to one signature: the page, its version and the step.
	Purpose string
	// ReturnPath is where the browser comes back to after a login at the identity provider.
	// It is made by the module, never taken from a request.
	ReturnPath string
}

// SigningStart is what the signing dialog needs to ask for a confirmation.
type SigningStart struct {
	Token string `json:"token"`
	// Methods are the ways this person can confirm: SigningByPasswordAndCode,
	// SigningByPasskey, SigningByProvider.
	Methods []string `json:"methods"`
	// Passkey holds the options for the browser when the person has a passkey.
	Passkey   any       `json:"passkey,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// signingMethods are the ways a person can confirm a signature: a password with the code of
// an authenticator app, a passkey with user verification, or, for someone who signs in only
// through the identity provider, a fresh login there (DOC-730, DOC-731). A password alone
// is never one of them.
func (s *Service) signingMethods(ctx context.Context, q db.Querier, userID uuid.UUID) ([]string, error) {
	var password, code, passkey, provider bool
	if err := q.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM local_credentials WHERE user_id = $1),
			EXISTS (SELECT 1 FROM totp_credentials WHERE user_id = $1 AND confirmed_at IS NOT NULL),
			EXISTS (SELECT 1 FROM webauthn_credentials WHERE user_id = $1),
			EXISTS (SELECT 1 FROM user_identities WHERE user_id = $1)`, userID).Scan(&password, &code, &passkey, &provider); err != nil {
		return nil, err
	}
	methods := []string{}
	if password && code {
		methods = append(methods, SigningByPasswordAndCode)
	}
	if passkey {
		methods = append(methods, SigningByPasskey)
	}
	if provider && !password {
		methods = append(methods, SigningByProvider)
	}
	return methods, nil
}

// BeginSigning starts the confirmation of one signature. Nothing is signed yet.
func (s *Service) BeginSigning(ctx context.Context, c Caller, in SigningRequest) (SigningStart, error) {
	in.Purpose = strings.TrimSpace(in.Purpose)
	if in.Purpose == "" || len(in.Purpose) > maxPurposeRunes {
		return SigningStart{}, errors.New("identity: a signing request needs a purpose")
	}
	if err := s.requireSigner(ctx, s.pool, c, in.SessionID); err != nil {
		return SigningStart{}, err
	}
	methods, err := s.signingMethods(ctx, s.pool, c.UserID)
	if err != nil {
		return SigningStart{}, err
	}
	if len(methods) == 0 {
		return SigningStart{}, signingRefused("second_factor_required",
			"A signature needs more than a password. Set up an authenticator app or a passkey first, under Your account.")
	}
	out := SigningStart{Methods: methods, Token: randomToken()}
	var session []byte
	if slices.Contains(methods, SigningByPasskey) {
		u, err := s.loadPasskeyUser(ctx, s.pool, c.UserID)
		if err != nil {
			return SigningStart{}, err
		}
		options, data, err := s.webauthn.BeginLogin(u, requireLocalCheck())
		if err != nil {
			return SigningStart{}, err
		}
		if session, err = json.Marshal(data); err != nil {
			return SigningStart{}, err
		}
		out.Passkey = options
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// A person has one signing step waiting for its confirmation at a time; a new dialog
		// replaces it. A confirmed one stays for its two minutes, so a dialog that comes back
		// from the identity provider can still use it.
		if _, err := tx.Exec(ctx, "DELETE FROM signing_confirmations WHERE user_id = $1 AND (confirmed_at IS NULL OR expires_at <= now())", c.UserID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO signing_confirmations (token_hash, user_id, session_id, purpose, return_path, passkey, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now() + make_interval(secs => $7)) RETURNING expires_at`,
			hashToken(out.Token), c.UserID, in.SessionID, in.Purpose, in.ReturnPath, session, signingBeginTTL.Seconds()).Scan(&out.ExpiresAt)
	})
	return out, err
}

// openSigning is a signing step of this person in this session that waits for its
// confirmation, locked.
type openSigning struct {
	purpose string
	passkey []byte
	began   time.Time
}

func (s *Service) lockOpenSigning(ctx context.Context, tx pgx.Tx, c Caller, sessionID uuid.UUID, token string) (openSigning, error) {
	var o openSigning
	err := tx.QueryRow(ctx, `SELECT purpose, passkey, began_at FROM signing_confirmations
		WHERE token_hash = $1 AND user_id = $2 AND session_id = $3 AND confirmed_at IS NULL AND expires_at > now() FOR UPDATE`,
		hashToken(token), c.UserID, sessionID).Scan(&o.purpose, &o.passkey, &o.began)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, signingRefused("signing_expired", "This signing step has ended. Open it again.")
	}
	return o, err
}

// markConfirmed starts the two minutes in which one signature may take the confirmation.
func markConfirmed(ctx context.Context, tx pgx.Tx, token, method string) error {
	_, err := tx.Exec(ctx, `UPDATE signing_confirmations SET confirmed_at = now(), method = $2,
		expires_at = now() + make_interval(secs => $3) WHERE token_hash = $1`, hashToken(token), method, SigningConfirmationTTL.Seconds())
	return err
}

// ConfirmSigningWithPassword confirms a signing step with the password and a code of the
// authenticator app, both given now (DOC-730). A recovery code does not sign: it is a
// way back into an account, written down where others may find it. A wrong password or code
// counts toward the stop after wrong passwords (DOC-732); the answer never says which was
// wrong.
func (s *Service) ConfirmSigningWithPassword(ctx context.Context, c Caller, sessionID uuid.UUID, token, password, code string) error {
	if err := s.requireSigner(ctx, s.pool, c, sessionID); err != nil {
		return err
	}
	var hash string
	var locked bool
	err := s.pool.QueryRow(ctx, "SELECT password_hash, password_locked_at IS NOT NULL FROM local_credentials WHERE user_id = $1", c.UserID).
		Scan(&hash, &locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return signingRefused("no_password", "This account has no password. Confirm with a passkey or at your identity provider.")
	}
	if err != nil {
		return err
	}
	ok, _, err := VerifyPassword(ctx, hash, password, s.params)
	if err != nil {
		return err
	}
	if locked {
		return s.refuseSigning(ctx, c, "password_locked", false)
	}
	if !ok {
		return s.refuseSigning(ctx, c, "bad_password", true)
	}
	wrongCode := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		open, err := s.lockOpenSigning(ctx, tx, c, sessionID, token)
		if err != nil {
			return err
		}
		_, used, err := s.useTOTP(ctx, tx, c.UserID, code)
		if err != nil {
			return err
		}
		if !used {
			wrongCode = true
			return errWrongConfirmation()
		}
		// A right password and code end a run of wrong ones, as a login does (ADR-038).
		if _, err := tx.Exec(ctx, "UPDATE local_credentials SET failed_logins_in_row = 0 WHERE user_id = $1 AND failed_logins_in_row > 0", c.UserID); err != nil {
			return err
		}
		if err := markConfirmed(ctx, tx, token, SigningByPasswordAndCode); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "auth.signing_confirmed", "user", c.UserID.String(),
			map[string]any{"method": SigningByPasswordAndCode, "purpose": open.purpose})
	})
	if wrongCode {
		return s.refuseSigning(ctx, c, "bad_code", true)
	}
	return err
}

func errWrongConfirmation() *apperr.Error {
	return signingRefused("wrong_confirmation", "The password or the code is not right.")
}

// refuseSigning records a failed confirmation in its own transaction, as the refusal rolled
// back its own. A wrong password or code counts as a guess (DOC-732); at the limit, password
// login stops and the person is told by mail, as after wrong logins.
func (s *Service) refuseSigning(ctx context.Context, c Caller, reason string, guess bool) error {
	lockedNow := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if guess {
			var err error
			if lockedNow, err = countGuess(ctx, tx, c.UserID); err != nil {
				return err
			}
		}
		e := c.Event("auth.signing_failed", "user", c.UserID.String(), "", map[string]any{"reason": reason})
		e.Result = audit.ResultFailure
		if _, err := audit.Record(ctx, tx, e); err != nil {
			return err
		}
		if !lockedNow {
			return nil
		}
		_, err := audit.Record(ctx, tx, event(c.Meta, SystemActor("signing"), "auth.password_locked", "user", c.UserID.String(),
			audit.ResultSuccess, map[string]any{"failed_in_row": PasswordLockAfter}))
		return err
	})
	if err != nil {
		return err
	}
	if lockedNow {
		u, err := s.GetUser(ctx, s.pool, c.UserID)
		if err != nil {
			return err
		}
		if err := s.send(ctx, s.pool, AccountMail{To: u.Email, Kind: MailPasswordLocked}); err != nil {
			return err
		}
	}
	return errWrongConfirmation()
}

// ConfirmSigningWithPasskey confirms a signing step with a passkey of the person, which must
// report user verification: a PIN or a biometric on the device, not only its presence
// (DOC-730, SEC-134).
func (s *Service) ConfirmSigningWithPasskey(ctx context.Context, c Caller, sessionID uuid.UUID, token string, response []byte) error {
	if err := s.requireSigner(ctx, s.pool, c, sessionID); err != nil {
		return err
	}
	failed := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		open, err := s.lockOpenSigning(ctx, tx, c, sessionID, token)
		if err != nil {
			return err
		}
		if len(open.passkey) == 0 {
			failed = true
			return nil
		}
		var session webauthn.SessionData
		if err := json.Unmarshal(open.passkey, &session); err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
		if err != nil {
			failed = true
			return nil
		}
		u, err := s.loadPasskeyUser(ctx, tx, c.UserID)
		if err != nil {
			return err
		}
		cred, err := s.webauthn.ValidateLogin(u, session, parsed)
		if err != nil {
			failed = true
			return nil
		}
		updated, err := json.Marshal(cred)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE webauthn_credentials SET credential = $2, last_used_at = now() WHERE credential_id = $1 AND user_id = $3",
			cred.ID, updated, c.UserID); err != nil {
			return err
		}
		if err := markConfirmed(ctx, tx, token, SigningByPasskey); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "auth.signing_confirmed", "user", c.UserID.String(),
			map[string]any{"method": SigningByPasskey, "purpose": open.purpose})
	})
	if err != nil || !failed {
		return err
	}
	// A passkey is not guessed, so a failed one is recorded but does not count as a guess.
	if err := s.refuseSigning(ctx, c, "bad_passkey", false); !isWrongConfirmation(err) {
		return err
	}
	return signingRefused("passkey_refused", "This passkey could not confirm who you are. It must ask for your PIN or fingerprint.")
}

func isWrongConfirmation(err error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == "wrong_confirmation"
}

// SigningAtProvider is a signing step that waits for a fresh login at the identity provider.
type SigningAtProvider struct {
	BeganAt time.Time
}

// BeginSigningAtProvider checks that a signing step of this person may be confirmed at the
// identity provider, and says when it began: the provider's login must come after that
// (DOC-731).
func (s *Service) BeginSigningAtProvider(ctx context.Context, c Caller, sessionID uuid.UUID, token string) (SigningAtProvider, error) {
	if err := s.requireSigner(ctx, s.pool, c, sessionID); err != nil {
		return SigningAtProvider{}, err
	}
	methods, err := s.signingMethods(ctx, s.pool, c.UserID)
	if err != nil {
		return SigningAtProvider{}, err
	}
	if !slices.Contains(methods, SigningByProvider) {
		return SigningAtProvider{}, signingRefused("provider_not_for_you", "You confirm with your password and code, or a passkey.")
	}
	var out SigningAtProvider
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		open, err := s.lockOpenSigning(ctx, tx, c, sessionID, token)
		out.BeganAt = open.began
		return err
	})
	return out, err
}

// ConfirmSigningByProvider confirms a signing step after the identity provider said that the
// account linked to userID logged in again after the step began; the sso module checked that.
// It answers where the browser goes back to.
func (s *Service) ConfirmSigningByProvider(ctx context.Context, token string, userID uuid.UUID, meta RequestMeta) (string, error) {
	var back, purpose string
	var sessionID uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT return_path, purpose, session_id FROM signing_confirmations
			WHERE token_hash = $1 AND user_id = $2 AND confirmed_at IS NULL AND expires_at > now() FOR UPDATE`,
			hashToken(token), userID).Scan(&back, &purpose, &sessionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errSigningConfirmation()
		}
		if err != nil {
			return err
		}
		c := Caller{UserID: userID, Meta: meta}
		if err := s.requireSigner(ctx, tx, c, sessionID); err != nil {
			return err
		}
		if err := markConfirmed(ctx, tx, token, SigningByProvider); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "auth.signing_confirmed", "user", userID.String(),
			map[string]any{"method": SigningByProvider, "purpose": purpose})
	})
	return back, err
}

// SigningReturnPath is where a signing step goes back to when the provider's login failed, so
// the dialog can say so. It answers "" for a step that is not this person's.
func (s *Service) SigningReturnPath(ctx context.Context, token string) (string, error) {
	var back string
	err := s.pool.QueryRow(ctx, "SELECT return_path FROM signing_confirmations WHERE token_hash = $1", hashToken(token)).Scan(&back)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return back, err
}

// TakeSigningConfirmation uses a confirmation for the signature of purpose, inside the
// transaction that writes the signature (DOC-712). It counts once: a second use, another
// person, another session, another purpose, an unconfirmed step or one older than two
// minutes are all refused (DOC-730). It answers how the person confirmed.
func (s *Service) TakeSigningConfirmation(ctx context.Context, tx pgx.Tx, c Caller, sessionID uuid.UUID, token, purpose string) (string, error) {
	if err := s.requireSigner(ctx, tx, c, sessionID); err != nil {
		return "", err
	}
	if token == "" {
		return "", signingRefused("signature_required", "This step needs your signature. Confirm who you are first.")
	}
	var owner, session uuid.UUID
	var bound string
	var method *string
	var fresh bool
	err := tx.QueryRow(ctx, `DELETE FROM signing_confirmations WHERE token_hash = $1
		RETURNING user_id, session_id, purpose, method, confirmed_at IS NOT NULL AND expires_at > now()`, hashToken(token)).
		Scan(&owner, &session, &bound, &method, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errSigningConfirmation()
	}
	if err != nil {
		return "", err
	}
	if owner != c.UserID || session != sessionID || bound != purpose || method == nil {
		return "", errSigningConfirmation()
	}
	if !fresh {
		return "", signingRefused("signing_confirmation_expired", "Your confirmation is more than two minutes old. Confirm who you are again.")
	}
	return *method, nil
}
