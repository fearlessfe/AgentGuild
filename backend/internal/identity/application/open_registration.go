package application

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

const (
	defaultOpenRegistrationChallengeTTL = 5 * time.Minute
	defaultOpenRegistrationTokenTTL     = 15 * time.Minute
	defaultOpenRegistrationOrganization = "public"
	defaultOpenRegistrationOperator     = "open-registration"
	defaultOpenRegistrationEmail        = "open-registration@agentguild.local"
)

var defaultOpenRegistrationScopes = []string{
	"tasks:read",
	"tasks:claim",
	"tasks:execute",
}

var openRegistrationHandlePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type GlobalTokenIssuer interface {
	IssueGlobal(*domain.AgentIdentity, *domain.AgentVersion, []string, time.Time) (string, error)
}

type OpenRegistrationStore interface {
	WithOpenRegistrationTx(context.Context, func(OpenRegistrationTx) error) error
}

type OpenRegistrationTx interface {
	Now(context.Context) (time.Time, error)
	InsertChallenge(context.Context, *domain.AgentRegistrationChallenge) error
	GetChallengeForUpdate(context.Context, string) (*domain.AgentRegistrationChallenge, error)
	ConsumeChallenge(context.Context, *domain.AgentRegistrationChallenge) error
	PublicKeyRegistered(context.Context, string) (bool, error)
	GetGlobalAgentSessionByThumbprint(context.Context, string, string) (*GlobalAgentSession, error)
	InsertIdentity(context.Context, *domain.AgentIdentity) error
	InsertVersion(context.Context, *domain.AgentVersion) error
	SetIdentityCurrentVersion(context.Context, string, string) error
	InsertIdentityKey(context.Context, *domain.AgentIdentityKey) error
	InsertMembership(context.Context, *domain.OrganizationMembership) error
	AppendIdentityEvent(context.Context, domain.AgentIdentityEvent) error
	GetGlobalAgentSession(context.Context, string, string, string) (*GlobalAgentSession, error)
	TouchGlobalAgent(context.Context, string, string, string, time.Time) (*GlobalAgentSession, error)
}

type GlobalAgentSession struct {
	Identity *domain.AgentIdentity
	Version  *domain.AgentVersion
	Scopes   []string
}

type OpenRegistrationOptions struct {
	NewID          func() string
	TokenIssuer    GlobalTokenIssuer
	OrganizationID string
	DefaultScopes  []string
	ChallengeTTL   time.Duration
	AccessTokenTTL time.Duration
	OperatorID     string
	OperatorEmail  string
}

type OpenRegistrationService struct {
	store          OpenRegistrationStore
	newID          func() string
	tokenIssuer    GlobalTokenIssuer
	organizationID string
	defaultScopes  []string
	challengeTTL   time.Duration
	tokenTTL       time.Duration
	operatorID     string
	operatorEmail  string
}

type CreateRegistrationChallenge struct {
	PublicKey []byte
}

type RegistrationChallengeView struct {
	ChallengeID string    `json:"challenge_id"`
	Nonce       string    `json:"nonce"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type OpenRegisterAgent struct {
	ChallengeID       string
	PublicKey         []byte
	KeyID             string
	Signature         []byte
	Handle            string
	DisplayName       string
	Description       string
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
}

type OpenRegistrationResponse struct {
	AgentID             string    `json:"agent_id"`
	AgentVersionID      string    `json:"agent_version_id"`
	OrganizationID      string    `json:"organization_id"`
	PublicKeyThumbprint string    `json:"public_key_thumbprint"`
	TrustLevel          string    `json:"trust_level"`
	AccessToken         string    `json:"access_token"`
	TokenType           string    `json:"token_type"`
	ExpiresAt           time.Time `json:"expires_at"`
	Scopes              []string  `json:"scopes"`
}

type GlobalAgentView struct {
	AgentID        string     `json:"agent_id"`
	AgentVersionID string     `json:"agent_version_id"`
	Handle         string     `json:"handle"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	OrganizationID string     `json:"organization_id"`
	Scopes         []string   `json:"scopes"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
}

func NewOpenRegistrationService(store OpenRegistrationStore, options OpenRegistrationOptions) (*OpenRegistrationService, error) {
	if store == nil {
		return nil, domain.ErrInvalidArgument
	}
	if options.TokenIssuer == nil {
		return nil, domain.ErrInvalidArgument
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.OrganizationID == "" {
		options.OrganizationID = defaultOpenRegistrationOrganization
	}
	if len(options.DefaultScopes) == 0 {
		options.DefaultScopes = defaultOpenRegistrationScopes
	}
	if options.ChallengeTTL <= 0 {
		options.ChallengeTTL = defaultOpenRegistrationChallengeTTL
	}
	if options.AccessTokenTTL <= 0 {
		options.AccessTokenTTL = defaultOpenRegistrationTokenTTL
	}
	if options.OperatorID == "" {
		options.OperatorID = defaultOpenRegistrationOperator
	}
	if options.OperatorEmail == "" {
		options.OperatorEmail = defaultOpenRegistrationEmail
	}
	return &OpenRegistrationService{
		store:          store,
		newID:          options.NewID,
		tokenIssuer:    options.TokenIssuer,
		organizationID: options.OrganizationID,
		defaultScopes:  append([]string(nil), options.DefaultScopes...),
		challengeTTL:   options.ChallengeTTL,
		tokenTTL:       options.AccessTokenTTL,
		operatorID:     options.OperatorID,
		operatorEmail:  options.OperatorEmail,
	}, nil
}

func (s *OpenRegistrationService) CreateChallenge(ctx context.Context, command CreateRegistrationChallenge) (Envelope[RegistrationChallengeView], error) {
	var result Envelope[RegistrationChallengeView]
	if len(command.PublicKey) != ed25519.PublicKeySize {
		return result, invalid("public_key")
	}
	err := s.store.WithOpenRegistrationTx(ctx, func(tx OpenRegistrationTx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		challenge, err := domain.NewAgentRegistrationChallenge(s.newID(), ed25519.PublicKey(command.PublicKey), s.challengeTTL, now)
		if err != nil {
			return err
		}
		if err := tx.InsertChallenge(ctx, challenge); err != nil {
			return err
		}
		result = Envelope[RegistrationChallengeView]{
			Data: RegistrationChallengeView{
				ChallengeID: challenge.ID,
				Nonce:       base64.RawURLEncoding.EncodeToString(challenge.Nonce),
				ExpiresAt:   challenge.ExpiresAt,
			},
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *OpenRegistrationService) Register(ctx context.Context, command OpenRegisterAgent) (Envelope[OpenRegistrationResponse], error) {
	var result Envelope[OpenRegistrationResponse]
	if command.ChallengeID == "" {
		return result, invalid("challenge_id")
	}
	if len(command.PublicKey) != ed25519.PublicKeySize {
		return result, invalid("public_key")
	}
	if len(command.Signature) != ed25519.SignatureSize {
		return result, invalid("signature")
	}
	if command.Runtime == "" {
		return result, invalid("runtime")
	}
	if command.Model == "" {
		return result, invalid("model")
	}
	command.Handle = strings.ToLower(strings.TrimSpace(command.Handle))
	if command.Handle != "" && !openRegistrationHandlePattern.MatchString(command.Handle) {
		return result, invalid("handle")
	}
	if len(command.DisplayName) > 128 {
		return result, invalid("display_name")
	}
	if len(command.Description) > 2048 {
		return result, invalid("description")
	}
	if len(command.Runtime) > 128 {
		return result, invalid("runtime")
	}
	if len(command.Model) > 128 {
		return result, invalid("model")
	}
	if len(command.Capabilities) > 64 || !validRegistrationCapabilities(command.Capabilities) {
		return result, invalid("capabilities")
	}
	if len(command.ConfigFingerprint) > 256 {
		return result, invalid("config_fingerprint")
	}
	if command.KeyID == "" {
		command.KeyID = "key-1"
	}
	thumbprint := domain.PublicKeyThumbprint(command.PublicKey)
	err := s.store.WithOpenRegistrationTx(ctx, func(tx OpenRegistrationTx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		challenge, err := tx.GetChallengeForUpdate(ctx, command.ChallengeID)
		if err != nil {
			return err
		}
		if !equalBytes(challenge.PublicKey, command.PublicKey) {
			return domain.ErrForbidden
		}
		if !ed25519.Verify(ed25519.PublicKey(command.PublicKey), domain.RegistrationProofMessage(challenge.ID, challenge.Nonce, command.PublicKey), command.Signature) {
			return domain.ErrForbidden
		}
		if !now.Before(challenge.ExpiresAt) {
			return domain.ErrTokenExpired
		}
		if challenge.Status == domain.RegistrationChallengeConsumed {
			return domain.ErrTokenExpired
		}
		registered, err := tx.PublicKeyRegistered(ctx, thumbprint)
		if err != nil {
			return err
		}
		if registered {
			session, err := tx.GetGlobalAgentSessionByThumbprint(ctx, thumbprint, s.organizationID)
			if err != nil {
				return err
			}
			challenge.RegisteredAgentID = session.Identity.ID
			challenge.RegisteredVersionID = session.Version.ID
			if err := challenge.Consume(now); err != nil {
				return err
			}
			if err := tx.ConsumeChallenge(ctx, challenge); err != nil {
				return err
			}
			return s.issueOpenRegistrationResponse(session.Identity, session.Version, session.Scopes, thumbprint, now, &result)
		}

		agentID := s.newID()
		handle := command.Handle
		if handle == "" {
			handle = "agent-" + thumbprint[:12]
		}
		displayName := command.DisplayName
		if displayName == "" {
			displayName = handle
		}
		identity, err := domain.NewAgentIdentity(agentID, handle, displayName, optionalString(command.Description), now)
		if err != nil {
			return err
		}
		version, err := domain.NewGlobalAgentVersion(s.newID(), agentID, 1, command.Runtime, command.Model, command.Capabilities, command.ConfigFingerprint, now)
		if err != nil {
			return err
		}
		if err := identity.Activate(version, agentID, now); err != nil {
			return err
		}
		membership, err := domain.NewOrganizationMembership(agentID, s.organizationID, s.operatorID, s.operatorEmail, "", s.defaultScopes, now)
		if err != nil {
			return err
		}
		if err := tx.InsertIdentity(ctx, identity); err != nil {
			return err
		}
		if err := tx.InsertVersion(ctx, version); err != nil {
			return err
		}
		if err := tx.SetIdentityCurrentVersion(ctx, agentID, version.ID); err != nil {
			return err
		}
		if err := tx.InsertIdentityKey(ctx, &domain.AgentIdentityKey{AgentID: agentID, KeyID: command.KeyID, Algorithm: "Ed25519", Thumbprint: thumbprint, PublicKey: append([]byte(nil), command.PublicKey...), CreatedAt: now}); err != nil {
			return err
		}
		if err := tx.InsertMembership(ctx, membership); err != nil {
			return err
		}
		for _, event := range identity.Events {
			if err := tx.AppendIdentityEvent(ctx, event); err != nil {
				return err
			}
		}
		challenge.RegisteredAgentID = identity.ID
		challenge.RegisteredVersionID = version.ID
		if err := challenge.Consume(now); err != nil {
			return err
		}
		if err := tx.ConsumeChallenge(ctx, challenge); err != nil {
			return err
		}
		return s.issueOpenRegistrationResponse(identity, version, s.defaultScopes, thumbprint, now, &result)
	})
	return result, err
}

func (s *OpenRegistrationService) issueOpenRegistrationResponse(identity *domain.AgentIdentity, version *domain.AgentVersion, scopes []string, thumbprint string, now time.Time, result *Envelope[OpenRegistrationResponse]) error {
	token, err := s.tokenIssuer.IssueGlobal(identity, version, scopes, now)
	if err != nil {
		return err
	}
	*result = Envelope[OpenRegistrationResponse]{
		Data: OpenRegistrationResponse{
			AgentID: identity.ID, AgentVersionID: version.ID, OrganizationID: s.organizationID,
			PublicKeyThumbprint: thumbprint, TrustLevel: "unverified",
			AccessToken: token, TokenType: "Bearer", ExpiresAt: now.Add(s.tokenTTL),
			Scopes: append([]string(nil), scopes...),
		},
		Meta: Meta{ServerTime: now},
	}
	return nil
}

func (s *OpenRegistrationService) Refresh(ctx context.Context, agentID, versionID string) (Envelope[AccessTokenView], error) {
	var result Envelope[AccessTokenView]
	if agentID == "" {
		return result, invalid("agent_id")
	}
	if versionID == "" {
		return result, invalid("agent_version_id")
	}
	err := s.store.WithOpenRegistrationTx(ctx, func(tx OpenRegistrationTx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		session, err := tx.GetGlobalAgentSession(ctx, agentID, versionID, s.organizationID)
		if err != nil {
			return err
		}
		if session.Identity.Status != domain.AgentActive || session.Identity.CurrentVersionID != session.Version.ID {
			return domain.ErrTokenRevoked
		}
		token, err := s.tokenIssuer.IssueGlobal(session.Identity, session.Version, session.Scopes, now)
		if err != nil {
			return err
		}
		result = Envelope[AccessTokenView]{
			Data: AccessTokenView{
				Token: token, TokenType: "Bearer", ExpiresAt: now.Add(s.tokenTTL),
				AgentID: session.Identity.ID, AgentVersionID: session.Version.ID,
				Scopes: append([]string(nil), session.Scopes...),
			},
			Meta: Meta{ServerTime: now},
		}
		return nil
	})
	return result, err
}

func (s *OpenRegistrationService) GetSelf(ctx context.Context, agentID, versionID string) (Envelope[GlobalAgentView], error) {
	return s.globalAgentView(ctx, agentID, versionID, false)
}

func (s *OpenRegistrationService) Heartbeat(ctx context.Context, agentID, versionID string) (Envelope[GlobalAgentView], error) {
	return s.globalAgentView(ctx, agentID, versionID, true)
}

func (s *OpenRegistrationService) globalAgentView(ctx context.Context, agentID, versionID string, heartbeat bool) (Envelope[GlobalAgentView], error) {
	var result Envelope[GlobalAgentView]
	if agentID == "" {
		return result, invalid("agent_id")
	}
	if versionID == "" {
		return result, invalid("agent_version_id")
	}
	err := s.store.WithOpenRegistrationTx(ctx, func(tx OpenRegistrationTx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		var session *GlobalAgentSession
		if heartbeat {
			session, err = tx.TouchGlobalAgent(ctx, agentID, versionID, s.organizationID, now)
		} else {
			session, err = tx.GetGlobalAgentSession(ctx, agentID, versionID, s.organizationID)
		}
		if err != nil {
			return err
		}
		result = Envelope[GlobalAgentView]{
			Data: GlobalAgentView{
				AgentID: session.Identity.ID, AgentVersionID: session.Version.ID,
				Handle: session.Identity.Handle, DisplayName: session.Identity.DisplayName,
				Status: session.Identity.Status, OrganizationID: s.organizationID,
				Scopes: append([]string(nil), session.Scopes...), LastSeenAt: session.Identity.LastSeenAt,
			},
			Meta: Meta{ServerTime: now, PollAfterSeconds: 30},
		}
		return nil
	})
	return result, err
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validRegistrationCapabilities(capabilities []string) bool {
	for _, capability := range capabilities {
		if capability == "" || len(capability) > 128 {
			return false
		}
	}
	return true
}
