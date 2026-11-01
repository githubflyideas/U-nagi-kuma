package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// TenantID and NetworkScopeID are immutable opaque identities, not labels.
type TenantID string
type NetworkScopeID string

type Tenant struct {
	ID   TenantID
	Name string
}

// TenantContext is an explicit repository context, NOT an authentication token.
// Callers MUST derive UserID from the authenticated principal, never a request
// body. Every operation rechecks current membership; do not cache authorization.
// TenantMonitors scopes the monitor/state/history subset; other business-data
// methods remain default-instance APIs and are not tenant-ready.
type TenantContext struct {
	TenantID TenantID
	UserID   int64
}

type TenantMember struct {
	TenantID TenantID
	UserID   int64
	Role     string
	// Existing global area restrictions still apply; membership never widens them.
	Areas []string
}

type NetworkScope struct {
	ID       NetworkScopeID
	TenantID TenantID
	Label    string
}

// ErrTenantAccess deliberately does not distinguish missing resources from
// missing, disabled or unauthorized membership.
var ErrTenantAccess = errors.New("tenant access denied")
var ErrNetworkScopeLabel = errors.New("network scope label must not be empty")

// DefaultTenant returns the persisted compatibility tenant, not a name lookup.
// No tenant creation/grant API is exposed until business isolation is complete.
func (s *Store) DefaultTenant() (Tenant, error) {
	var t Tenant
	err := s.core.QueryRow(`SELECT id,name FROM tenants WHERE is_default=1`).Scan(&t.ID, &t.Name)
	return t, err
}

func tenantAccessError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTenantAccess
	}
	return err
}

// TenantMember resolves only an explicit, active (tenant,user) membership.
func (s *Store) TenantMember(ctx TenantContext) (TenantMember, error) {
	var m TenantMember
	var areas string
	err := s.core.QueryRow(`SELECT m.tenant_id,m.user_id,m.role,u.areas
 FROM tenant_members m JOIN users u ON u.id=m.user_id JOIN tenants t ON t.id=m.tenant_id
 WHERE m.tenant_id=? AND m.user_id=? AND u.disabled=0 AND m.disabled=0`, ctx.TenantID, ctx.UserID).
		Scan(&m.TenantID, &m.UserID, &m.Role, &areas)
	if err != nil {
		return TenantMember{}, tenantAccessError(err)
	}
	m.Areas = splitAreas(areas)
	return m, nil
}

// CreateNetworkScope adds registry metadata only; it does not route probes or
// bind monitors. Only active tenant admins may write. Authorization and insert
// are one statement, so concurrent revocation cannot race a preflight check.
func (s *Store) CreateNetworkScope(ctx TenantContext, label string) (NetworkScope, error) {
	if strings.TrimSpace(label) == "" {
		return NetworkScope{}, ErrNetworkScopeLabel
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return NetworkScope{}, err
	}
	id := NetworkScopeID(hex.EncodeToString(raw[:]))
	s.wmu.Lock()
	defer s.wmu.Unlock()
	var out NetworkScope
	err := s.core.QueryRow(`INSERT INTO network_scopes(id,tenant_id,label)
 SELECT ?,m.tenant_id,? FROM tenant_members m JOIN users u ON u.id=m.user_id
 WHERE m.tenant_id=? AND m.user_id=? AND m.role='admin' AND m.disabled=0 AND u.disabled=0
 RETURNING id,tenant_id,label`, id, label, ctx.TenantID, ctx.UserID).Scan(&out.ID, &out.TenantID, &out.Label)
	if err != nil {
		return NetworkScope{}, tenantAccessError(err)
	}
	return out, nil
}

// NetworkScope never resolves by label or falls back to the default tenant.
func (s *Store) NetworkScope(ctx TenantContext, id NetworkScopeID) (NetworkScope, error) {
	var out NetworkScope
	err := s.core.QueryRow(`SELECT n.id,n.tenant_id,n.label FROM network_scopes n
 JOIN tenant_members m ON m.tenant_id=n.tenant_id JOIN users u ON u.id=m.user_id
 WHERE n.id=? AND n.tenant_id=? AND m.user_id=? AND m.disabled=0 AND u.disabled=0`, id, ctx.TenantID, ctx.UserID).
		Scan(&out.ID, &out.TenantID, &out.Label)
	if err != nil {
		return NetworkScope{}, tenantAccessError(err)
	}
	return out, nil
}
