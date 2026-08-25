package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// MemoryStore holds the centralized in-memory state for IAM, Audit, and Nodes.
type MemoryStore struct {
	mu           sync.RWMutex
	users        map[string]*identity.User
	roles        map[string]*identity.Role
	permissions  map[string]*identity.Permission
	grants       map[string][]*identity.UserRoleGrant
	apiKeys      map[string]*identity.APIKey
	apiKeysByH   map[string]*identity.APIKey
	sessions     map[string]*identity.RefreshSession
	auditLogs    []*audit.AuditLog
	auditCounter int64
	nodeStore    *MemoryNodeStore
	instanceStore *MemoryInstanceStore
	ipPoolStore   *MemoryIPPoolRepo
	ipAllocStore  *MemoryIPAllocationRepo
	firewallStore *MemoryFirewallRepo
	storagePools     *StoragePoolRepository
	volumes          *VolumeRepository
	snapshots        *VolumeSnapshotRepository
	metricsRepo      *MemoryMetricsRepo
	thresholdRepo    *MemoryAlertThresholdRepo
	alertEventRepo   *MemoryAlertEventRepo
	siemRepo         *MemorySIEMRepo
	templateRepo     *MemoryTemplateRepo
	imageRepo        *MemoryImageRepo
	planRepo         *MemoryPlanRepo
	subRepo          *MemorySubscriptionRepo
	quotaRepo        *MemoryQuotaRepo
	usageRepo        *MemoryUsageRepo
	invoiceRepo      *MemoryInvoiceRepo
	eventRepo        *MemoryEventRepo
	notifRepo        *MemoryNotificationRepo
	prefRepo         *MemoryPreferenceRepo
	webhookRepo      *MemoryWebhookRepo
	deliveryRepo     *MemoryDeliveryRepo
	jobStore         *MemoryJobStore
	migrationStore   *MemoryMigrationRepo
	rateLimiter      *MemoryRateLimiter
	backupStore      *MemoryBackupRepo
	reconcileStore   *MemoryReconcileRepo
	keyRotationStore *MemoryKeyRotationRepo
}

// NewMemoryStore initializes a memory store pre-seeded with default system roles and permissions.
func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		users:            make(map[string]*identity.User),
		roles:            make(map[string]*identity.Role),
		permissions:      make(map[string]*identity.Permission),
		grants:           make(map[string][]*identity.UserRoleGrant),
		apiKeys:          make(map[string]*identity.APIKey),
		apiKeysByH:       make(map[string]*identity.APIKey),
		sessions:         make(map[string]*identity.RefreshSession),
		auditLogs:        make([]*audit.AuditLog, 0),
		nodeStore:        NewMemoryNodeStore(),
		instanceStore:    NewMemoryInstanceStore(),
		ipPoolStore:      NewMemoryIPPoolRepo(),
		ipAllocStore:     NewMemoryIPAllocationRepo(),
		firewallStore:    NewMemoryFirewallRepo(),
		storagePools:     NewStoragePoolRepository(),
		volumes:          NewVolumeRepository(),
		snapshots:        NewVolumeSnapshotRepository(),
		metricsRepo:      NewMemoryMetricsRepo(),
		thresholdRepo:    NewMemoryAlertThresholdRepo(),
		alertEventRepo:   NewMemoryAlertEventRepo(),
		siemRepo:         NewMemorySIEMRepo(),
		templateRepo:     NewMemoryTemplateRepo(),
		imageRepo:        NewMemoryImageRepo(),
		planRepo:         NewMemoryPlanRepo(),
		subRepo:          NewMemorySubscriptionRepo(),
		quotaRepo:        NewMemoryQuotaRepo(),
		usageRepo:        NewMemoryUsageRepo(),
		invoiceRepo:      NewMemoryInvoiceRepo(),
		eventRepo:        NewMemoryEventRepo(),
		notifRepo:        NewMemoryNotificationRepo(),
		prefRepo:         NewMemoryPreferenceRepo(),
		webhookRepo:      NewMemoryWebhookRepo(),
		deliveryRepo:     NewMemoryDeliveryRepo(),
		jobStore:         NewMemoryJobStore(),
		migrationStore:   NewMemoryMigrationRepo(),
		rateLimiter:      NewMemoryRateLimiter(),
		backupStore:      NewMemoryBackupRepo(),
		reconcileStore:   NewMemoryReconcileRepo(),
		keyRotationStore: NewMemoryKeyRotationRepo(),
	}
	m.seedDefaults()
	return m
}

func (m *MemoryStore) Users() *MemoryUserRepo                { return &MemoryUserRepo{s: m} }
func (m *MemoryStore) Roles() *MemoryRoleRepo                { return &MemoryRoleRepo{s: m} }
func (m *MemoryStore) Permissions() *MemoryRoleRepo          { return &MemoryRoleRepo{s: m} }
func (m *MemoryStore) APIKeys() *MemoryAPIKeyRepo            { return &MemoryAPIKeyRepo{s: m} }
func (m *MemoryStore) Sessions() *MemorySessionRepo          { return &MemorySessionRepo{s: m} }
func (m *MemoryStore) Audit() *MemoryAuditRepo               { return &MemoryAuditRepo{s: m} }
func (m *MemoryStore) Nodes() *MemoryNodeRepo                { return m.nodeStore.Nodes() }
func (m *MemoryStore) Enrollments() *MemoryEnrollmentRepo    { return m.nodeStore.Enrollments() }
func (m *MemoryStore) Instances() *MemoryInstanceRepo        { return m.instanceStore.Instances() }
func (m *MemoryStore) IPPools() *MemoryIPPoolRepo            { return m.ipPoolStore }
func (m *MemoryStore) IPAllocations() *MemoryIPAllocationRepo { return m.ipAllocStore }
func (m *MemoryStore) Firewall() *MemoryFirewallRepo         { return m.firewallStore }
func (m *MemoryStore) StoragePools() *StoragePoolRepository  { return m.storagePools }
func (m *MemoryStore) Volumes() *VolumeRepository            { return m.volumes }
func (m *MemoryStore) Snapshots() *VolumeSnapshotRepository  { return m.snapshots }
func (m *MemoryStore) Metrics() *MemoryMetricsRepo           { return m.metricsRepo }
func (m *MemoryStore) AlertThresholds() *MemoryAlertThresholdRepo { return m.thresholdRepo }
func (m *MemoryStore) AlertEvents() *MemoryAlertEventRepo    { return m.alertEventRepo }
func (m *MemoryStore) SIEM() *MemorySIEMRepo                 { return m.siemRepo }
func (m *MemoryStore) Templates() *MemoryTemplateRepo        { return m.templateRepo }
func (m *MemoryStore) Images() *MemoryImageRepo              { return m.imageRepo }
func (m *MemoryStore) Plans() *MemoryPlanRepo                { return m.planRepo }
func (m *MemoryStore) Subscriptions() *MemorySubscriptionRepo { return m.subRepo }
func (m *MemoryStore) Quotas() *MemoryQuotaRepo              { return m.quotaRepo }
func (m *MemoryStore) Usage() *MemoryUsageRepo               { return m.usageRepo }
func (m *MemoryStore) Invoices() *MemoryInvoiceRepo          { return m.invoiceRepo }
func (m *MemoryStore) Events() *MemoryEventRepo              { return m.eventRepo }
func (m *MemoryStore) Notifications() *MemoryNotificationRepo { return m.notifRepo }
func (m *MemoryStore) Preferences() *MemoryPreferenceRepo    { return m.prefRepo }
func (m *MemoryStore) Webhooks() *MemoryWebhookRepo          { return m.webhookRepo }
func (m *MemoryStore) Deliveries() *MemoryDeliveryRepo        { return m.deliveryRepo }
func (m *MemoryStore) Jobs() *MemoryJobRepo                  { return m.jobStore.Jobs() }
func (m *MemoryStore) Leases() *MemoryWorkerLeaseRepo        { return m.jobStore.Leases() }
func (m *MemoryStore) Migrations() *MemoryMigrationRepo      { return m.migrationStore }
func (m *MemoryStore) RateLimiter() *MemoryRateLimiter       { return m.rateLimiter }
func (m *MemoryStore) Backups() *MemoryBackupRepo            { return m.backupStore }
func (m *MemoryStore) Reconcile() *MemoryReconcileRepo       { return m.reconcileStore }
func (m *MemoryStore) KeyRotations() *MemoryKeyRotationRepo  { return m.keyRotationStore }

func (m *MemoryStore) seedDefaults() {
	perms := []*identity.Permission{
		{Code: "*", Description: "All permissions (Superadmin wildcard)", Category: "system"},
		{Code: "instance:read", Description: "View instances", Category: "instance"},
		{Code: "instance:create", Description: "Create new instances", Category: "instance"},
		{Code: "instance:update", Description: "Update instance specs", Category: "instance"},
		{Code: "instance:delete", Description: "Delete instances", Category: "instance"},
		{Code: "instance:power", Description: "Power control instances", Category: "instance"},
		{Code: "instance:console", Description: "Access instance web console", Category: "instance"},
		{Code: "instance:files:read", Description: "Read instance files", Category: "instance"},
		{Code: "instance:files:write", Description: "Write instance files", Category: "instance"},
		{Code: "node:read", Description: "View hypervisor nodes", Category: "node"},
		{Code: "node:create", Description: "Enroll new nodes", Category: "node"},
		{Code: "node:update", Description: "Edit node configuration", Category: "node"},
		{Code: "node:maintenance", Description: "Toggle node maintenance", Category: "node"},
		{Code: "node:drain", Description: "Drain and undrain hypervisor nodes", Category: "node"},
		{Code: "node:evacuate", Description: "Evacuate workloads from hypervisor nodes", Category: "node"},
		{Code: "job:read", Description: "View async jobs", Category: "job"},
		{Code: "job:manage", Description: "Cancel, retry, and manage async jobs", Category: "job"},
		{Code: "migration:read", Description: "View instance migrations", Category: "compute"},
		{Code: "migration:manage", Description: "Initiate and control instance migrations", Category: "compute"},
		{Code: "backup:read", Description: "View and inspect backups", Category: "backup"},
		{Code: "backup:create", Description: "Create backup recovery points", Category: "backup"},
		{Code: "backup:restore", Description: "Restore workloads from backups", Category: "backup"},
		{Code: "backup:delete", Description: "Delete backup recovery points", Category: "backup"},
		{Code: "backup:manage", Description: "Configure backup policies and disaster recovery", Category: "backup"},
		{Code: "reconcile:manage", Description: "Execute control plane state reconciliation", Category: "system"},
		{Code: "keys:manage", Description: "Rotate and revoke cryptographic credentials", Category: "security"},
		{Code: "ipam:read", Description: "View IP pools and subnets", Category: "ipam"},
		{Code: "ipam:manage", Description: "Manage IP pools and port forwards", Category: "ipam"},
		{Code: "storage:read", Description: "View storage pools", Category: "storage"},
		{Code: "storage:manage", Description: "Manage storage pools", Category: "storage"},
		{Code: "volume:read", Description: "View volumes and snapshots", Category: "storage"},
		{Code: "volume:create", Description: "Create storage volumes", Category: "storage"},
		{Code: "volume:update", Description: "Update or resize volumes", Category: "storage"},
		{Code: "volume:delete", Description: "Delete storage volumes", Category: "storage"},
		{Code: "volume:attach", Description: "Attach volume to instance", Category: "storage"},
		{Code: "volume:detach", Description: "Detach volume from instance", Category: "storage"},
		{Code: "volume:snapshot", Description: "Create volume snapshots", Category: "storage"},
		{Code: "volume:restore", Description: "Restore volume from snapshot", Category: "storage"},
		{Code: "monitoring:read", Description: "View telemetry metrics and alerts", Category: "monitoring"},
		{Code: "monitoring:manage", Description: "Configure alert thresholds and acknowledge alerts", Category: "monitoring"},
		{Code: "user:read", Description: "View user accounts", Category: "user"},
		{Code: "user:manage", Description: "Manage users and roles", Category: "user"},
		{Code: "audit:read", Description: "View security audit trails", Category: "audit"},
		{Code: "audit:manage", Description: "Configure SIEM destinations and export compliance reports", Category: "audit"},
		{Code: "template:read", Description: "View OS templates", Category: "template"},
		{Code: "template:create", Description: "Create new OS templates", Category: "template"},
		{Code: "template:update", Description: "Update OS template metadata", Category: "template"},
		{Code: "template:delete", Description: "Delete or deprecate OS templates", Category: "template"},
		{Code: "image:read", Description: "View image artifacts and node cache availability", Category: "image"},
		{Code: "image:manage", Description: "Register, sync, verify, and retire image artifacts", Category: "image"},
		{Code: "billing:read", Description: "View plans, subscriptions, quotas, and invoices", Category: "billing"},
		{Code: "billing:manage", Description: "Subscribe, change plans, and manage billing", Category: "billing"},
		{Code: "billing:plans", Description: "Create and update system billing plans", Category: "billing"},
		{Code: "billing:admin", Description: "Manage cross-tenant subscriptions, usage, and invoices", Category: "billing"},
		{Code: "notification:read", Description: "View user notifications and preferences", Category: "notification"},
		{Code: "notification:manage", Description: "Manage notifications and preferences", Category: "notification"},
		{Code: "webhook:read", Description: "View webhook endpoints and delivery history", Category: "webhook"},
		{Code: "webhook:create", Description: "Create webhook endpoints", Category: "webhook"},
		{Code: "webhook:update", Description: "Update webhook endpoints", Category: "webhook"},
		{Code: "webhook:delete", Description: "Delete webhook endpoints", Category: "webhook"},
		{Code: "webhook:rotate", Description: "Rotate webhook signing secret", Category: "webhook"},
		{Code: "webhook:test", Description: "Trigger test webhook delivery ping", Category: "webhook"},
	}

	var allPerms []identity.Permission
	for _, p := range perms {
		m.permissions[p.Code] = p
		allPerms = append(allPerms, *p)
	}

	superadminRole := &identity.Role{
		ID:          "role_superadmin",
		Name:        "superadmin",
		Description: "Unrestricted platform administrator",
		IsSystem:    true,
		Permissions: allPerms,
		CreatedAt:   time.Now().UTC(),
	}

	customerRole := &identity.Role{
		ID:          "role_customer",
		Name:        "customer",
		Description: "Standard tenant / VPS owner",
		IsSystem:    true,
		Permissions: []identity.Permission{
			*m.permissions["instance:read"],
			*m.permissions["instance:create"],
			*m.permissions["instance:power"],
			*m.permissions["instance:console"],
			*m.permissions["instance:files:read"],
			*m.permissions["instance:files:write"],
			*m.permissions["backup:read"],
			*m.permissions["backup:create"],
			*m.permissions["backup:restore"],
			*m.permissions["volume:read"],
			*m.permissions["volume:create"],
			*m.permissions["volume:update"],
			*m.permissions["volume:delete"],
			*m.permissions["volume:attach"],
			*m.permissions["volume:detach"],
			*m.permissions["volume:snapshot"],
			*m.permissions["volume:restore"],
			*m.permissions["storage:read"],
			*m.permissions["monitoring:read"],
			*m.permissions["monitoring:manage"],
			*m.permissions["template:read"],
			*m.permissions["billing:read"],
			*m.permissions["billing:manage"],
			*m.permissions["notification:read"],
			*m.permissions["notification:manage"],
			*m.permissions["webhook:read"],
			*m.permissions["webhook:create"],
			*m.permissions["webhook:update"],
			*m.permissions["webhook:delete"],
			*m.permissions["webhook:rotate"],
			*m.permissions["webhook:test"],
		},
		CreatedAt: time.Now().UTC(),
	}

	operatorRole := &identity.Role{
		ID:          "role_operator",
		Name:        "operator",
		Description: "Infrastructure hypervisor operator",
		IsSystem:    true,
		Permissions: []identity.Permission{
			*m.permissions["instance:read"],
			*m.permissions["node:read"],
			*m.permissions["node:maintenance"],
			*m.permissions["ipam:read"],
			*m.permissions["audit:read"],
		},
		CreatedAt: time.Now().UTC(),
	}

	m.roles[superadminRole.ID] = superadminRole
	m.roles[superadminRole.Name] = superadminRole
	m.roles[customerRole.ID] = customerRole
	m.roles[customerRole.Name] = customerRole
	m.roles[operatorRole.ID] = operatorRole
	m.roles[operatorRole.Name] = operatorRole
}

// ---------------- USER REPOSITORY ----------------

type MemoryUserRepo struct{ s *MemoryStore }

func (r *MemoryUserRepo) Create(ctx context.Context, u *identity.User) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for _, existing := range r.s.users {
		if existing.Username == u.Username {
			return identity.ErrUsernameExists
		}
		if existing.Email == u.Email {
			return identity.ErrEmailExists
		}
	}

	copy := *u
	r.s.users[u.ID] = &copy
	return nil
}

func (r *MemoryUserRepo) GetByID(ctx context.Context, id string) (*identity.User, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	u, exists := r.s.users[id]
	if !exists {
		return nil, identity.ErrUserNotFound
	}
	copy := *u
	return &copy, nil
}

func (r *MemoryUserRepo) GetByUsername(ctx context.Context, username string) (*identity.User, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	for _, u := range r.s.users {
		if u.Username == username {
			copy := *u
			return &copy, nil
		}
	}
	return nil, identity.ErrUserNotFound
}

func (r *MemoryUserRepo) GetByEmail(ctx context.Context, email string) (*identity.User, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	for _, u := range r.s.users {
		if u.Email == email {
			copy := *u
			return &copy, nil
		}
	}
	return nil, identity.ErrUserNotFound
}

func (r *MemoryUserRepo) Update(ctx context.Context, u *identity.User) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	existing, exists := r.s.users[u.ID]
	if !exists {
		return identity.ErrUserNotFound
	}
	existing.Username = u.Username
	existing.Email = u.Email
	existing.IsActive = u.IsActive
	existing.Preferences = u.Preferences
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryUserRepo) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	existing, exists := r.s.users[id]
	if !exists {
		return identity.ErrUserNotFound
	}
	existing.PasswordHash = passwordHash
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryUserRepo) Update2FA(ctx context.Context, id string, enabled bool, secretEnc string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	existing, exists := r.s.users[id]
	if !exists {
		return identity.ErrUserNotFound
	}
	existing.TwoFactorEnabled = enabled
	existing.TwoFactorSecretEnc = secretEnc
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryUserRepo) UpdateLastLogin(ctx context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	existing, exists := r.s.users[id]
	if !exists {
		return identity.ErrUserNotFound
	}
	now := time.Now().UTC()
	existing.LastLoginAt = &now
	return nil
}

func (r *MemoryUserRepo) Count(ctx context.Context) (int64, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	return int64(len(r.s.users)), nil
}

// ---------------- ROLE REPOSITORY ----------------

type MemoryRoleRepo struct{ s *MemoryStore }

func (r *MemoryRoleRepo) GetByID(ctx context.Context, id string) (*identity.Role, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	role, exists := r.s.roles[id]
	if !exists {
		return nil, identity.ErrRoleNotFound
	}
	copy := *role
	return &copy, nil
}

func (r *MemoryRoleRepo) GetByName(ctx context.Context, name string) (*identity.Role, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	for _, role := range r.s.roles {
		if role.Name == name {
			copy := *role
			return &copy, nil
		}
	}
	return nil, identity.ErrRoleNotFound
}

func (r *MemoryRoleRepo) List(ctx context.Context) ([]*identity.Role, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var roles []*identity.Role
	seen := make(map[string]bool)
	for _, role := range r.s.roles {
		if !seen[role.ID] {
			seen[role.ID] = true
			copy := *role
			roles = append(roles, &copy)
		}
	}
	return roles, nil
}

func (r *MemoryRoleRepo) GetGrantsForUser(ctx context.Context, userID string) ([]*identity.UserRoleGrant, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	grants := r.s.grants[userID]
	var copies []*identity.UserRoleGrant
	for _, g := range grants {
		copy := *g
		copies = append(copies, &copy)
	}
	return copies, nil
}

func (r *MemoryRoleRepo) AssignRoleToUser(ctx context.Context, grant *identity.UserRoleGrant) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	copy := *grant
	r.s.grants[grant.UserID] = append(r.s.grants[grant.UserID], &copy)
	return nil
}

func (r *MemoryRoleRepo) RevokeRoleFromUser(ctx context.Context, userID, roleID string, scopeType string, scopeID *string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	grants := r.s.grants[userID]
	var filtered []*identity.UserRoleGrant
	for _, g := range grants {
		if g.RoleID == roleID && g.ScopeType == scopeType {
			if scopeID == nil && g.ScopeID == nil {
				continue
			}
			if scopeID != nil && g.ScopeID != nil && *scopeID == *g.ScopeID {
				continue
			}
		}
		filtered = append(filtered, g)
	}
	r.s.grants[userID] = filtered
	return nil
}

func (r *MemoryRoleRepo) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	grants := r.s.grants[userID]
	permMap := make(map[string]bool)

	for _, g := range grants {
		role, exists := r.s.roles[g.RoleID]
		if !exists {
			continue
		}
		if role.Name == "superadmin" {
			return []string{"*"}, nil
		}
		for _, p := range role.Permissions {
			permMap[p.Code] = true
		}
	}

	var perms []string
	for p := range permMap {
		perms = append(perms, p)
	}
	return perms, nil
}

func (r *MemoryRoleRepo) GetByCode(ctx context.Context, code string) (*identity.Permission, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	p, exists := r.s.permissions[code]
	if !exists {
		return nil, identity.ErrPermissionNotFound
	}
	copy := *p
	return &copy, nil
}

// ---------------- API KEY REPOSITORY ----------------

type MemoryAPIKeyRepo struct{ s *MemoryStore }

func (r *MemoryAPIKeyRepo) Create(ctx context.Context, k *identity.APIKey) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	copy := *k
	r.s.apiKeys[k.ID] = &copy
	r.s.apiKeysByH[k.KeyHash] = &copy
	return nil
}

func (r *MemoryAPIKeyRepo) GetByID(ctx context.Context, id string) (*identity.APIKey, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	k, exists := r.s.apiKeys[id]
	if !exists {
		return nil, identity.ErrAPIKeyNotFound
	}
	copy := *k
	return &copy, nil
}

func (r *MemoryAPIKeyRepo) GetByKeyHash(ctx context.Context, keyHash string) (*identity.APIKey, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	k, exists := r.s.apiKeysByH[keyHash]
	if !exists {
		return nil, identity.ErrAPIKeyNotFound
	}
	copy := *k
	return &copy, nil
}

func (r *MemoryAPIKeyRepo) ListByUser(ctx context.Context, userID string) ([]*identity.APIKey, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var keys []*identity.APIKey
	for _, k := range r.s.apiKeys {
		if k.UserID == userID {
			copy := *k
			keys = append(keys, &copy)
		}
	}
	return keys, nil
}

func (r *MemoryAPIKeyRepo) UpdateLastUsed(ctx context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	k, exists := r.s.apiKeys[id]
	if !exists {
		return identity.ErrAPIKeyNotFound
	}
	now := time.Now().UTC()
	k.LastUsedAt = &now
	return nil
}

func (r *MemoryAPIKeyRepo) Revoke(ctx context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	k, exists := r.s.apiKeys[id]
	if !exists {
		return identity.ErrAPIKeyNotFound
	}
	k.IsRevoked = true
	return nil
}

// ---------------- SESSION REPOSITORY ----------------

type MemorySessionRepo struct{ s *MemoryStore }

func (r *MemorySessionRepo) Create(ctx context.Context, session *identity.RefreshSession) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	copy := *session
	r.s.sessions[session.TokenHash] = &copy
	return nil
}

func (r *MemorySessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*identity.RefreshSession, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	s, exists := r.s.sessions[tokenHash]
	if !exists {
		return nil, identity.ErrRefreshTokenInvalid
	}
	copy := *s
	return &copy, nil
}

func (r *MemorySessionRepo) Revoke(ctx context.Context, id string, replacedByID *string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	now := time.Now().UTC()
	for _, s := range r.s.sessions {
		if s.ID == id {
			s.IsRevoked = true
			s.RevokedAt = &now
			s.ReplacedByTokenID = replacedByID
			return nil
		}
	}
	return nil
}

func (r *MemorySessionRepo) RevokeFamily(ctx context.Context, familyID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	now := time.Now().UTC()
	for _, s := range r.s.sessions {
		if s.FamilyID == familyID {
			s.IsRevoked = true
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *MemorySessionRepo) RevokeAllForUser(ctx context.Context, userID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	now := time.Now().UTC()
	for _, s := range r.s.sessions {
		if s.UserID == userID {
			s.IsRevoked = true
			s.RevokedAt = &now
		}
	}
	return nil
}

// ---------------- AUDIT REPOSITORY ----------------

type MemoryAuditRepo struct{ s *MemoryStore }

func (r *MemoryAuditRepo) Record(ctx context.Context, log *audit.AuditLog) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.auditCounter++
	copy := *log
	copy.ID = r.s.auditCounter
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = time.Now().UTC()
	}
	if copy.Severity == "" {
		copy.Severity = audit.SeverityInfo
	}
	if len(r.s.auditLogs) > 0 {
		copy.PrevHash = r.s.auditLogs[len(r.s.auditLogs)-1].TamperProofHash
	}
	copy.TamperProofHash = copy.ComputeHash()

	r.s.auditLogs = append(r.s.auditLogs, &copy)
	return nil
}

func (r *MemoryAuditRepo) List(ctx context.Context, limit, offset int) ([]*audit.AuditLog, error) {
	logs, _, err := r.ListFiltered(ctx, audit.AuditFilter{Limit: limit, Offset: offset})
	return logs, err
}

func (r *MemoryAuditRepo) ListFiltered(ctx context.Context, filter audit.AuditFilter) ([]*audit.AuditLog, int64, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var matching []*audit.AuditLog
	for i := len(r.s.auditLogs) - 1; i >= 0; i-- {
		l := r.s.auditLogs[i]
		if filter.ActorID != "" && (l.ActorID == nil || *l.ActorID != filter.ActorID) {
			continue
		}
		if filter.Action != "" && l.Action != filter.Action {
			continue
		}
		if filter.ResourceType != "" && l.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && (l.ResourceID == nil || *l.ResourceID != filter.ResourceID) {
			continue
		}
		if filter.Severity != "" && l.Severity != filter.Severity {
			continue
		}
		if filter.From != nil && l.CreatedAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && l.CreatedAt.After(*filter.To) {
			continue
		}
		cp := *l
		matching = append(matching, &cp)
	}

	total := int64(len(matching))
	if filter.Offset >= len(matching) {
		return []*audit.AuditLog{}, total, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	end := filter.Offset + limit
	if end > len(matching) {
		end = len(matching)
	}

	return matching[filter.Offset:end], total, nil
}

func (r *MemoryAuditRepo) GetLatestLog(ctx context.Context) (*audit.AuditLog, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	if len(r.s.auditLogs) == 0 {
		return nil, nil
	}
	cp := *r.s.auditLogs[len(r.s.auditLogs)-1]
	return &cp, nil
}

func (r *MemoryAuditRepo) VerifyChainIntegrity(ctx context.Context, limit int) (bool, int64, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	if len(r.s.auditLogs) == 0 {
		return true, 0, nil
	}

	start := 0
	if limit > 0 && len(r.s.auditLogs) > limit {
		start = len(r.s.auditLogs) - limit
	}

	for i := start; i < len(r.s.auditLogs); i++ {
		l := r.s.auditLogs[i]
		if !l.VerifyHash() {
			return false, l.ID, nil
		}
		if i > 0 {
			prev := r.s.auditLogs[i-1]
			if l.PrevHash != prev.TamperProofHash {
				return false, l.ID, nil
			}
		}
	}

	return true, int64(len(r.s.auditLogs) - start), nil
}
