// Project Aurora — Typed API Client & Authentication Layer

export interface User {
  id: string;
  username: string;
  email: string;
  roles: string[];
  permissions: string[];
  twoFactorEnabled?: boolean;
}

export interface Instance {
  id: string;
  userId: string;
  nodeId: string;
  name: string;
  type: 'container' | 'virtual-machine';
  status: 'pending' | 'provisioning' | 'running' | 'stopped' | 'suspended' | 'error';
  cpuCores: number;
  memoryBytes: number;
  storageBytes: number;
  image: string;
  ipv4Address?: string;
  ipv6Address?: string;
  config?: Record<string, any>;
  createdAt: string;
  updatedAt: string;
}

export interface InstanceMetrics {
  timestamp: string;
  cpuPercent: number;
  memoryUsedBytes: number;
  memoryTotalBytes: number;
  memoryPercent: number;
  diskUsedBytes: number;
  diskTotalBytes: number;
  diskPercent: number;
  netRxBytes: number;
  netTxBytes: number;
  netRxPackets: number;
  netTxPackets: number;
}

export interface FirewallRule {
  id: string;
  protocol: 'tcp' | 'udp' | 'icmp' | 'all';
  portRange: string;
  sourceCidr: string;
  action: 'allow' | 'deny';
  direction: 'inbound' | 'outbound';
  description?: string;
}

export interface GuestFile {
  path: string;
  name: string;
  isDir: boolean;
  sizeBytes: number;
  mode: string;
  modTime: string;
}

export interface Backup {
  id: string;
  instanceId: string;
  name: string;
  sizeBytes: number;
  status: string;
  createdAt: string;
}

export interface Snapshot {
  id: string;
  instanceId: string;
  name: string;
  stateful: boolean;
  sizeBytes: number;
  createdAt: string;
}

export interface OSTemplate {
  id: string;
  name: string;
  slug: string;
  description: string;
  distribution: string;
  version: string;
  release: string;
  supportedArchitectures: string[];
  supportedInstanceTypes: string[];
  minDiskBytes: number;
  minMemoryBytes: number;
  cloudInitSupported: boolean;
  status: string;
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  createdAt: string;
  expiresAt?: string;
  lastUsedAt?: string;
}

export interface Session {
  id: string;
  ipAddress: string;
  userAgent: string;
  expiresAt: string;
  createdAt: string;
}

export interface Node {
  id: string;
  name: string;
  fqdn: string;
  locationId: string;
  status: 'online' | 'offline' | 'degraded' | 'unhealthy' | 'draining' | 'maintenance' | 'revoked' | 'enrolling';
  maintenanceMode: boolean;
  drainMode?: boolean;
  unhealthyReason?: string;
  lastStateChangeAt?: string;
  cpuCores?: number;
  memoryBytes?: number;
  storageBytes?: number;
  cpuOvercommitRatio?: number;
  memoryOvercommitRatio?: number;
  capabilities: Record<string, any>;
  lastHeartbeatAt?: string;
  enrolledAt: string;
}

export interface StoragePool {
  id: string;
  nodeId: string;
  name: string;
  driver: string;
  totalBytes: number;
  usedBytes: number;
  freeBytes: number;
  status: string;
  config?: Record<string, any>;
}

export interface IPAMPool {
  id: string;
  name: string;
  cidr: string;
  ipVersion: number;
  gateway: string;
  dnsServers: string[];
  vlanId?: number;
  totalIps: number;
  allocatedIps: number;
  status: string;
}

export interface AuditLog {
  id: number;
  actorId?: string;
  action: string;
  resourceType?: string;
  resourceId?: string;
  statusCode?: number;
  severity: string;
  details?: Record<string, any>;
  prevHash: string;
  tamperProofHash: string;
  createdAt: string;
}

export interface SIEMDestination {
  id: string;
  name: string;
  transportType: 'webhook' | 'syslog_tcp' | 'syslog_udp';
  format: 'json' | 'cef' | 'rfc5424';
  endpointUrl?: string;
  syslogHost?: string;
  syslogPort?: number;
  enabled: boolean;
  createdAt: string;
}

export interface Location {
  id: string;
  name: string;
  country: string;
  region: string;
  description?: string;
  enabled: boolean;
}

export interface ApiResponse<T = any> {
  success: boolean;
  data?: T;
  error?: {
    code: string;
    message: string;
  };
  meta?: {
    requestId?: string;
    timestamp?: string;
  };
}

export interface BillingPlan {
  id: string;
  name: string;
  slug: string;
  description: string;
  currency: string;
  monthlyPriceMinor: number;
  yearlyPriceMinor: number;
  includedVcpu: number;
  includedMemoryMb: number;
  includedStorageMb: number;
  includedIpv4: number;
  includedSnapshots: number;
  includedBackups: number;
  includedBandwidthGb: number;
  maxInstances: number;
  maxVcpu: number;
  maxMemoryMb: number;
  maxStorageMb: number;
  features: Record<string, boolean>;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Subscription {
  id: string;
  userId: string;
  planId: string;
  status: 'active' | 'past_due' | 'canceled' | 'trialing';
  billingCycle: 'monthly' | 'yearly';
  currentPeriodStart: string;
  currentPeriodEnd: string;
  cancelAtPeriodEnd: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface QuotaItem {
  userId: string;
  metric: string;
  limit: number;
  currentUsage: number;
  resetPeriod: string;
  updatedAt: string;
}

export type QuotaSet = Record<string, QuotaItem>;

export interface UsageMetricSummary {
  metric: string;
  totalQuantity: number;
  unit: string;
}

export interface UsageAggregate {
  userId: string;
  periodStart: string;
  periodEnd: string;
  metrics: Record<string, UsageMetricSummary>;
}

export interface InvoiceLine {
  id: string;
  invoiceId: string;
  description: string;
  quantity: number;
  unitPriceMinor: number;
  amountMinor: number;
  metricType?: string;
}

export interface Invoice {
  id: string;
  invoiceNumber: string;
  userId: string;
  status: 'draft' | 'open' | 'paid' | 'uncollectible' | 'void';
  currency: string;
  subtotalMinor: number;
  taxMinor: number;
  totalMinor: number;
  periodStart: string;
  periodEnd: string;
  dueDate: string;
  paidAt?: string;
  paymentIntentId?: string;
  lines: InvoiceLine[];
  createdAt: string;
}

export interface NotificationItem {
  id: string;
  tenantId: string;
  userId: string;
  type: string;
  title: string;
  body: string;
  severity: 'info' | 'success' | 'warning' | 'error' | 'critical';
  resourceType?: string;
  resourceId?: string;
  readAt?: string | null;
  createdAt: string;
}

export interface NotificationPreference {
  userId: string;
  eventType: string;
  inAppEnabled: boolean;
  emailEnabled: boolean;
  webhookEnabled: boolean;
}

export interface Job {
  id: string;
  tenantId: string;
  type: string;
  resourceType?: string;
  resourceId?: string;
  status: 'pending' | 'running' | 'retrying' | 'succeeded' | 'failed' | 'canceled';
  payload?: any;
  result?: any;
  error?: string;
  retryCount: number;
  maxRetries: number;
  nextRetryAt?: string;
  lockedByWorker?: string;
  lockedUntil?: string;
  progressPercent: number;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface PreflightValidation {
  compatibleArch: boolean;
  imageAvailable: boolean;
  storageAvailable: boolean;
  networkAvailable: boolean;
  destinationHealthy: boolean;
  cpuCapacityOk: boolean;
  memoryCapacityOk: boolean;
  storageCapacityOk: boolean;
  failureReason?: string;
}

export interface WorkloadMigration {
  id: string;
  tenantId: string;
  instanceId: string;
  sourceNodeId: string;
  destNodeId: string;
  type: 'live' | 'cold';
  status: 'pending' | 'validating' | 'reserving' | 'transferring' | 'verifying' | 'completed' | 'failed' | 'canceled' | 'rolled_back';
  preflight?: PreflightValidation;
  progressPercent: number;
  bytesTransferred: number;
  totalBytes: number;
  error?: string;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookEndpoint {
  id: string;
  tenantId: string;
  name: string;
  url: string;
  description?: string;
  active: boolean;
  eventTypes: string[];
  failureCount: number;
  lastStatus?: string;
  lastDeliveryAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookDelivery {
  id: string;
  eventId: string;
  webhookId: string;
  tenantId: string;
  eventType: string;
  attempt: number;
  status: 'pending' | 'delivered' | 'failed' | 'dead_letter';
  httpStatus: number;
  responseTimeMs: number;
  error?: string;
  nextRetryAt?: string;
  deliveredAt?: string;
  createdAt: string;
}

export interface AuroraEvent {
  id: string;
  tenantId: string;
  type: string;
  resourceType: string;
  resourceId: string;
  actorId?: string;
  timestamp: string;
  payload: Record<string, any>;
  metadata?: Record<string, string>;
  version: string;
}

export class ApiError extends Error {
  code: string;
  status: number;
  requestId?: string;

  constructor(message: string, code: string = 'UNKNOWN_ERROR', status: number = 500, requestId?: string) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.requestId = requestId;
  }
}

class ApiClient {
  private token: string | null = null;
  private refreshToken: string | null = null;

  constructor() {
    this.token = localStorage.getItem('aurora_token');
    this.refreshToken = localStorage.getItem('aurora_refresh_token');
  }

  public setTokens(token: string, refresh?: string) {
    this.token = token;
    localStorage.setItem('aurora_token', token);
    if (refresh) {
      this.refreshToken = refresh;
      localStorage.setItem('aurora_refresh_token', refresh);
    }
  }

  public clearTokens() {
    this.token = null;
    this.refreshToken = null;
    localStorage.removeItem('aurora_token');
    localStorage.removeItem('aurora_refresh_token');
  }

  public getToken(): string | null {
    return this.token;
  }

  public async request<T = any>(
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
    const headers = new Headers(options.headers || {});
    if (this.token && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${this.token}`);
    }
    if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
      headers.set('Content-Type', 'application/json');
    }

    const res = await fetch(path, {
      ...options,
      headers,
    });

    if (res.status === 401 && this.refreshToken && path !== '/api/v1/auth/refresh' && path !== '/api/v1/auth/login') {
      const refreshed = await this.tryRefreshToken();
      if (refreshed) {
        headers.set('Authorization', `Bearer ${this.token}`);
        const retryRes = await fetch(path, { ...options, headers });
        return this.parseResponse<T>(retryRes);
      }
    }

    return this.parseResponse<T>(res);
  }

  private async parseResponse<T>(res: Response): Promise<T> {
    let json: ApiResponse<T>;
    try {
      json = await res.json();
    } catch {
      throw new ApiError(`HTTP ${res.status} ${res.statusText}`, 'PARSE_ERROR', res.status);
    }

    if (!res.ok || json.success === false) {
      const code = json.error?.code || 'REQUEST_FAILED';
      const msg = json.error?.message || `HTTP ${res.status}`;
      throw new ApiError(msg, code, res.status, json.meta?.requestId);
    }

    return json.data as T;
  }

  private async tryRefreshToken(): Promise<boolean> {
    if (!this.refreshToken) return false;
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken: this.refreshToken }),
      });
      if (res.ok) {
        const json = await res.json();
        if (json.data?.accessToken) {
          this.setTokens(json.data.accessToken, json.data.refreshToken);
          return true;
        }
      }
    } catch {
      // Refresh failed
    }
    this.clearTokens();
    return false;
  }

  // --- Auth Endpoints ---
  public async login(usernameOrEmail: string, password: string, totpCode?: string) {
    const data = await this.request<{ tokens: { accessToken: string; refreshToken: string }; user: User }>(
      '/api/v1/auth/login',
      {
        method: 'POST',
        body: JSON.stringify({ usernameOrEmail, password, totpCode }),
      }
    );
    this.setTokens(data.tokens.accessToken, data.tokens.refreshToken);
    return data;
  }

  public async register(username: string, email: string, password: string) {
    return this.request('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, email, password }),
    });
  }

  public async getMe(): Promise<User> {
    const data = await this.request<{ user: User }>('/api/v1/auth/me');
    return data.user || data;
  }

  public async logout() {
    try {
      await this.request('/api/v1/auth/logout', { method: 'POST' });
    } finally {
      this.clearTokens();
    }
  }

  // --- Account Endpoints ---
  public async getAccountProfile() {
    return this.request<{ user: User }>('/api/v1/account');
  }

  public async updatePassword(currentPassword: string, newPassword: string) {
    return this.request('/api/v1/account/password', {
      method: 'PUT',
      body: JSON.stringify({ currentPassword, newPassword }),
    });
  }

  public async setup2FA() {
    return this.request<{ secret: string; qrCodeDataUrl: string }>('/api/v1/account/2fa/setup', {
      method: 'POST',
    });
  }

  public async verify2FA(code: string) {
    return this.request('/api/v1/account/2fa/verify', {
      method: 'POST',
      body: JSON.stringify({ code }),
    });
  }

  public async disable2FA(code: string) {
    return this.request('/api/v1/account/2fa/disable', {
      method: 'POST',
      body: JSON.stringify({ code }),
    });
  }

  public async listSessions(): Promise<Session[]> {
    const data = await this.request<{ sessions: Session[] }>('/api/v1/account/sessions');
    return data.sessions || [];
  }

  public async revokeSession(sessionId: string) {
    return this.request(`/api/v1/account/sessions/${sessionId}`, { method: 'DELETE' });
  }

  // --- API Keys ---
  public async listApiKeys(): Promise<ApiKey[]> {
    const data = await this.request<{ apiKeys: ApiKey[] }>('/api/v1/account/api-keys');
    return data.apiKeys || [];
  }

  public async createApiKey(name: string, scopes: string[], expiresDays?: number) {
    return this.request<{ apiKey: ApiKey; secret: string }>('/api/v1/account/api-keys', {
      method: 'POST',
      body: JSON.stringify({ name, scopes, expiresDays }),
    });
  }

  public async revokeApiKey(id: string) {
    return this.request(`/api/v1/account/api-keys/${id}`, { method: 'DELETE' });
  }

  // --- Instances ---
  public async listInstances(): Promise<Instance[]> {
    const data = await this.request<Instance[] | { instances: Instance[] }>('/api/v1/instances');
    if (Array.isArray(data)) return data;
    return (data as any).instances || [];
  }

  public async getInstance(id: string): Promise<Instance> {
    return this.request<Instance>(`/api/v1/instances/${id}`);
  }

  public async createInstance(spec: {
    name: string;
    type: 'container' | 'virtual-machine';
    cpuCores: number;
    memoryBytes: number;
    storageBytes: number;
    templateId?: string;
    templateSlug?: string;
    cloudInit?: any;
    startAfterCreate?: boolean;
  }): Promise<Instance> {
    return this.request<Instance>('/api/v1/instances', {
      method: 'POST',
      body: JSON.stringify(spec),
    });
  }

  public async powerAction(instanceId: string, action: 'start' | 'stop' | 'restart' | 'force_stop') {
    return this.request(`/api/v1/instances/${instanceId}/power`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    });
  }

  public async startInstance(instanceId: string) {
    return this.powerAction(instanceId, 'start');
  }

  public async stopInstance(instanceId: string) {
    return this.powerAction(instanceId, 'stop');
  }

  public async restartInstance(instanceId: string) {
    return this.powerAction(instanceId, 'restart');
  }

  public async updateInstanceSpec(instanceId: string, cpuCores: number, memoryBytes: number, storageBytes: number) {
    return this.request(`/api/v1/instances/${instanceId}/spec`, {
      method: 'PATCH',
      body: JSON.stringify({ cpuCores, memoryBytes, storageBytes }),
    });
  }

  public async deleteInstance(instanceId: string, force: boolean = false) {
    return this.request(`/api/v1/instances/${instanceId}?force=${force}`, {
      method: 'DELETE',
    });
  }

  public async getInstanceMetrics(instanceId: string): Promise<InstanceMetrics> {
    return this.request<InstanceMetrics>(`/api/v1/instances/${instanceId}/metrics`);
  }

  public async listFirewallRules(instanceId: string): Promise<FirewallRule[]> {
    const data = await this.request<FirewallRule[] | { rules: FirewallRule[] }>(`/api/v1/instances/${instanceId}/firewall`);
    if (Array.isArray(data)) return data;
    return (data as any).rules || [];
  }

  public async applyFirewallRules(instanceId: string, rules: FirewallRule[]) {
    return this.request(`/api/v1/instances/${instanceId}/firewall`, {
      method: 'PUT',
      body: JSON.stringify({ rules }),
    });
  }

  // --- Guest Files ---
  public async listGuestFiles(instanceId: string, path: string = '/'): Promise<GuestFile[]> {
    const data = await this.request<{ files: GuestFile[] }>(`/api/v1/instances/${instanceId}/files?path=${encodeURIComponent(path)}`);
    return data.files || [];
  }

  public async writeGuestFile(instanceId: string, path: string, content: string, isDir: boolean = false) {
    return this.request(`/api/v1/instances/${instanceId}/files`, {
      method: 'POST',
      body: JSON.stringify({ path, content, isDir }),
    });
  }

  public async deleteGuestFile(instanceId: string, path: string) {
    return this.request(`/api/v1/instances/${instanceId}/files?path=${encodeURIComponent(path)}`, {
      method: 'DELETE',
    });
  }

  // --- Backups & Snapshots ---
  public async listInstanceBackups(instanceId: string): Promise<Backup[]> {
    const data = await this.request<{ backups: Backup[] }>(`/api/v1/instances/${instanceId}/backups`);
    return data.backups || [];
  }

  public async createInstanceBackup(instanceId: string, name?: string): Promise<Backup> {
    return this.request<Backup>(`/api/v1/instances/${instanceId}/backups`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  }

  public async restoreInstanceBackup(instanceId: string, backupId: string) {
    return this.request(`/api/v1/instances/${instanceId}/backups/${backupId}/restore`, {
      method: 'POST',
    });
  }

  public async deleteInstanceBackup(instanceId: string, backupId: string) {
    return this.request(`/api/v1/instances/${instanceId}/backups/${backupId}`, {
      method: 'DELETE',
    });
  }

  public async listSnapshots(instanceId: string): Promise<Snapshot[]> {
    const data = await this.request<{ snapshots: Snapshot[] }>(`/api/v1/instances/${instanceId}/snapshots`);
    return data.snapshots || [];
  }

  public async createSnapshot(instanceId: string, name: string, stateful: boolean = false): Promise<Snapshot> {
    return this.request<Snapshot>(`/api/v1/instances/${instanceId}/snapshots`, {
      method: 'POST',
      body: JSON.stringify({ name, stateful }),
    });
  }

  public async restoreSnapshot(instanceId: string, snapshotId: string) {
    return this.request(`/api/v1/instances/${instanceId}/snapshots/${snapshotId}/restore`, {
      method: 'POST',
    });
  }

  public async deleteSnapshot(instanceId: string, snapshotId: string) {
    return this.request(`/api/v1/instances/${instanceId}/snapshots/${snapshotId}`, {
      method: 'DELETE',
    });
  }

  // --- Templates ---
  public async listTemplates(): Promise<OSTemplate[]> {
    const data = await this.request<{ templates: OSTemplate[] }>('/api/v1/templates');
    return data.templates || [];
  }

  public async getTemplate(id: string): Promise<OSTemplate> {
    return this.request<OSTemplate>(`/api/v1/templates/${id}`);
  }

  // --- ADMIN APIs ---
  public async listNodes(): Promise<Node[]> {
    const data = await this.request<Node[] | { data: Node[] }>('/api/v1/nodes');
    if (Array.isArray(data)) return data;
    return (data as any).data || [];
  }

  public async getNode(id: string): Promise<Node> {
    const data = await this.request<{ node?: Node } | Node>(`/api/v1/nodes/${id}`);
    return (data as any).node || (data as any);
  }

  public async createEnrollmentToken(locationId: string, ttlSeconds: number = 3600) {
    return this.request<{ enrollmentToken: string; expiresAt: string }>('/api/v1/nodes/enrollment-tokens', {
      method: 'POST',
      body: JSON.stringify({ locationId, ttlSeconds }),
    });
  }

  public async toggleNodeMaintenance(nodeId: string, inMaintenance: boolean) {
    return this.request(`/api/v1/nodes/${nodeId}/maintenance`, {
      method: 'POST',
      body: JSON.stringify({ inMaintenance }),
    });
  }

  public async revokeNode(nodeId: string) {
    return this.request(`/api/v1/nodes/${nodeId}/revoke`, { method: 'POST' });
  }

  public async pingNode(nodeId: string) {
    return this.request(`/api/v1/nodes/${nodeId}/commands/ping`, { method: 'POST' });
  }

  public async listStoragePools(nodeId?: string): Promise<StoragePool[]> {
    const url = nodeId ? `/api/v1/storage/pools?nodeId=${nodeId}` : '/api/v1/storage/pools';
    const data = await this.request<StoragePool[] | { pools: StoragePool[] }>(url);
    if (Array.isArray(data)) return data;
    return (data as any).pools || [];
  }

  public async createStoragePool(pool: {
    nodeId: string;
    name: string;
    driver: string;
    totalBytes: number;
  }): Promise<StoragePool> {
    return this.request<StoragePool>('/api/v1/storage/pools', {
      method: 'POST',
      body: JSON.stringify(pool),
    });
  }

  public async deleteStoragePool(id: string) {
    return this.request(`/api/v1/storage/pools/${id}`, { method: 'DELETE' });
  }

  public async listIPAMPools(): Promise<IPAMPool[]> {
    const data = await this.request<IPAMPool[] | { pools: IPAMPool[] }>('/api/v1/ipam/pools');
    if (Array.isArray(data)) return data;
    return (data as any).pools || [];
  }

  public async createIPAMPool(pool: {
    name: string;
    cidr: string;
    ipVersion: number;
    gateway: string;
    dnsServers: string[];
    vlanId?: number;
  }): Promise<IPAMPool> {
    return this.request<IPAMPool>('/api/v1/ipam/pools', {
      method: 'POST',
      body: JSON.stringify(pool),
    });
  }

  public async deleteIPAMPool(id: string) {
    return this.request(`/api/v1/ipam/pools/${id}`, { method: 'DELETE' });
  }

  public async listAuditLogs(limit: number = 50, offset: number = 0): Promise<{ logs: AuditLog[]; total: number }> {
    return this.request<{ logs: AuditLog[]; total: number }>(`/api/v1/audit/logs?limit=${limit}&offset=${offset}`);
  }

  public async verifyAuditChain(limit: number = 1000): Promise<{ valid: boolean; verifiedCount: number }> {
    return this.request<{ valid: boolean; verifiedCount: number }>(`/api/v1/audit/verify?limit=${limit}`);
  }

  public async exportAuditLogs(format: 'json' | 'csv'): Promise<string> {
    return this.request<string>(`/api/v1/audit/export?format=${format}`);
  }

  public async listSIEMDestinations(): Promise<SIEMDestination[]> {
    const data = await this.request<{ destinations: SIEMDestination[] }>('/api/v1/audit/siem');
    return data.destinations || [];
  }

  public async createSIEMDestination(dest: {
    name: string;
    transportType: string;
    format: string;
    endpointUrl?: string;
    syslogHost?: string;
    syslogPort?: number;
  }): Promise<SIEMDestination> {
    return this.request<SIEMDestination>('/api/v1/audit/siem', {
      method: 'POST',
      body: JSON.stringify(dest),
    });
  }

  public async deleteSIEMDestination(id: string) {
    return this.request(`/api/v1/audit/siem/${id}`, { method: 'DELETE' });
  }

  public async createOSTemplate(template: Partial<OSTemplate>): Promise<OSTemplate> {
    return this.request<OSTemplate>('/api/v1/admin/templates', {
      method: 'POST',
      body: JSON.stringify(template),
    });
  }

  public async deleteOSTemplate(id: string) {
    return this.request(`/api/v1/admin/templates/${id}`, { method: 'DELETE' });
  }

  public async listImageArtifacts(): Promise<any[]> {
    const data = await this.request<{ artifacts: any[] }>('/api/v1/admin/images');
    return data.artifacts || [];
  }

  public async registerImageArtifact(artifact: any): Promise<any> {
    return this.request('/api/v1/admin/images', {
      method: 'POST',
      body: JSON.stringify(artifact),
    });
  }

  public async verifyImageArtifact(id: string): Promise<{ valid: boolean }> {
    return this.request<{ valid: boolean }>(`/api/v1/admin/images/${id}/verify`, { method: 'POST' });
  }

  public async syncImageToNode(imageId: string, nodeId: string): Promise<any> {
    return this.request(`/api/v1/admin/images/${imageId}/sync`, {
      method: 'POST',
      body: JSON.stringify({ nodeId }),
    });
  }

  public async retireImageArtifact(id: string): Promise<any> {
    return this.request(`/api/v1/admin/images/${id}/retire`, { method: 'POST' });
  }

  // --- Customer Billing API ---

  public async listBillingPlans(): Promise<BillingPlan[]> {
    const data = await this.request<{ plans: BillingPlan[] }>('/api/v1/billing/plans');
    return data.plans || [];
  }

  public async getSubscription(): Promise<{ subscription: Subscription | null; plan?: BillingPlan }> {
    return this.request<{ subscription: Subscription | null; plan?: BillingPlan }>('/api/v1/billing/subscription');
  }

  public async subscribe(planId: string, billingCycle: 'monthly' | 'yearly' = 'monthly'): Promise<Subscription> {
    return this.request<Subscription>('/api/v1/billing/subscription', {
      method: 'POST',
      body: JSON.stringify({ planId, billingCycle }),
    });
  }

  public async changePlan(newPlanId: string): Promise<Subscription> {
    return this.request<Subscription>('/api/v1/billing/subscription', {
      method: 'PATCH',
      body: JSON.stringify({ newPlanId }),
    });
  }

  public async cancelSubscription(): Promise<void> {
    await this.request('/api/v1/billing/subscription', { method: 'DELETE' });
  }

  public async getQuotas(): Promise<{ quotas: QuotaSet; plan: BillingPlan }> {
    return this.request<{ quotas: QuotaSet; plan: BillingPlan }>('/api/v1/billing/quotas');
  }

  public async getUsage(start?: string, end?: string): Promise<UsageAggregate> {
    const params = new URLSearchParams();
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    const qs = params.toString();
    return this.request<UsageAggregate>(`/api/v1/billing/usage${qs ? '?' + qs : ''}`);
  }

  public async listInvoices(limit: number = 50, offset: number = 0): Promise<Invoice[]> {
    const data = await this.request<{ invoices: Invoice[] }>(`/api/v1/billing/invoices?limit=${limit}&offset=${offset}`);
    return data.invoices || [];
  }

  public async getInvoice(id: string): Promise<Invoice> {
    return this.request<Invoice>(`/api/v1/billing/invoices/${id}`);
  }

  // --- Admin Billing API ---

  public async adminListPlans(): Promise<BillingPlan[]> {
    const data = await this.request<{ plans: BillingPlan[] }>('/api/v1/admin/billing/plans');
    return data.plans || [];
  }

  public async adminCreatePlan(plan: Partial<BillingPlan>): Promise<BillingPlan> {
    return this.request<BillingPlan>('/api/v1/admin/billing/plans', {
      method: 'POST',
      body: JSON.stringify(plan),
    });
  }

  public async adminUpdatePlan(id: string, plan: Partial<BillingPlan>): Promise<BillingPlan> {
    return this.request<BillingPlan>(`/api/v1/admin/billing/plans/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(plan),
    });
  }

  public async adminDeletePlan(id: string): Promise<void> {
    await this.request(`/api/v1/admin/billing/plans/${id}`, { method: 'DELETE' });
  }

  public async adminListSubscriptions(): Promise<Subscription[]> {
    const data = await this.request<{ subscriptions: Subscription[] }>('/api/v1/admin/billing/subscriptions');
    return data.subscriptions || [];
  }

  public async adminGetUsage(tenantId?: string, start?: string, end?: string): Promise<UsageAggregate> {
    const params = new URLSearchParams();
    if (tenantId) params.set('tenantId', tenantId);
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    const qs = params.toString();
    return this.request<UsageAggregate>(`/api/v1/admin/billing/usage${qs ? '?' + qs : ''}`);
  }

  public async adminListInvoices(limit: number = 50, offset: number = 0): Promise<Invoice[]> {
    const data = await this.request<{ invoices: Invoice[] }>(`/api/v1/admin/billing/invoices?limit=${limit}&offset=${offset}`);
    return data.invoices || [];
  }

  public async adminVoidInvoice(id: string): Promise<void> {
    await this.request(`/api/v1/admin/billing/invoices/${id}/void`, { method: 'POST' });
  }

  public async adminRegenerateInvoice(tenantId: string, start?: string, end?: string): Promise<Invoice> {
    const params = new URLSearchParams({ tenantId });
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    return this.request<Invoice>(`/api/v1/admin/billing/invoices/regenerate?${params.toString()}`, { method: 'POST' });
  }

  // --- Notifications API ---

  public async listNotifications(unreadOnly: boolean = false, severity?: string, limit: number = 50, offset: number = 0): Promise<{ notifications: NotificationItem[]; total: number }> {
    const params = new URLSearchParams({ limit: limit.toString(), offset: offset.toString() });
    if (unreadOnly) params.set('unreadOnly', 'true');
    if (severity) params.set('severity', severity);
    return this.request<{ notifications: NotificationItem[]; total: number }>(`/api/v1/notifications?${params.toString()}`);
  }

  public async getUnreadNotificationCount(): Promise<number> {
    const data = await this.request<{ unreadCount: number }>('/api/v1/notifications/unread-count');
    return data.unreadCount || 0;
  }

  public async markNotificationRead(id: string): Promise<void> {
    await this.request(`/api/v1/notifications/${id}/read`, { method: 'POST' });
  }

  public async markAllNotificationsRead(): Promise<number> {
    const data = await this.request<{ markedRead: number }>('/api/v1/notifications/read-all', { method: 'POST' });
    return data.markedRead || 0;
  }

  public async getNotificationPreferences(): Promise<NotificationPreference[]> {
    const data = await this.request<{ preferences: NotificationPreference[] }>('/api/v1/notifications/preferences');
    return data.preferences || [];
  }

  public async setNotificationPreference(pref: NotificationPreference): Promise<NotificationPreference> {
    const data = await this.request<{ preference: NotificationPreference }>('/api/v1/notifications/preferences', {
      method: 'PUT',
      body: JSON.stringify(pref),
    });
    return data.preference;
  }

  // --- Webhooks API ---

  public async listWebhooks(): Promise<WebhookEndpoint[]> {
    const data = await this.request<{ webhooks: WebhookEndpoint[] }>('/api/v1/webhooks');
    return data.webhooks || [];
  }

  public async createWebhook(input: { name: string; url: string; description?: string; eventTypes: string[]; active?: boolean }): Promise<{ endpoint: WebhookEndpoint; secret: string }> {
    return this.request<{ endpoint: WebhookEndpoint; secret: string }>('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  public async getWebhook(id: string): Promise<WebhookEndpoint> {
    const data = await this.request<{ webhook: WebhookEndpoint }>(`/api/v1/webhooks/${id}`);
    return data.webhook;
  }

  public async updateWebhook(id: string, input: Partial<{ name: string; url: string; description: string; eventTypes: string[]; active: boolean }>): Promise<WebhookEndpoint> {
    const data = await this.request<{ webhook: WebhookEndpoint }>(`/api/v1/webhooks/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(input),
    });
    return data.webhook;
  }

  public async deleteWebhook(id: string): Promise<void> {
    await this.request(`/api/v1/webhooks/${id}`, { method: 'DELETE' });
  }

  public async rotateWebhookSecret(id: string): Promise<string> {
    const data = await this.request<{ secret: string }>(`/api/v1/webhooks/${id}/rotate-secret`, { method: 'POST' });
    return data.secret;
  }

  public async testWebhook(id: string): Promise<WebhookDelivery> {
    const data = await this.request<{ delivery: WebhookDelivery }>(`/api/v1/webhooks/${id}/test`, { method: 'POST' });
    return data.delivery;
  }

  public async listWebhookDeliveries(id: string, limit: number = 50, offset: number = 0): Promise<{ deliveries: WebhookDelivery[]; total: number }> {
    return this.request<{ deliveries: WebhookDelivery[]; total: number }>(`/api/v1/webhooks/${id}/deliveries?limit=${limit}&offset=${offset}`);
  }

  // --- Admin Events & Delivery Monitoring ---

  public async adminListEvents(params?: { tenantId?: string; type?: string; resourceType?: string; limit?: number; offset?: number }): Promise<{ events: AuroraEvent[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.tenantId) query.set('tenantId', params.tenantId);
    if (params?.type) query.set('type', params.type);
    if (params?.resourceType) query.set('resourceType', params.resourceType);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ events: AuroraEvent[]; total: number }>(`/api/v1/admin/events?${query.toString()}`);
  }

  public async adminListDeliveries(params?: { tenantId?: string; webhookId?: string; status?: string; limit?: number; offset?: number }): Promise<{ deliveries: WebhookDelivery[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.tenantId) query.set('tenantId', params.tenantId);
    if (params?.webhookId) query.set('webhookId', params.webhookId);
    if (params?.status) query.set('status', params.status);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ deliveries: WebhookDelivery[]; total: number }>(`/api/v1/admin/webhooks/deliveries?${query.toString()}`);
  }

  // --- Phase 15: Jobs & Worker Operations ---

  public async listJobs(params?: { status?: string; type?: string; limit?: number; offset?: number }): Promise<{ jobs: Job[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.status) query.set('status', params.status);
    if (params?.type) query.set('type', params.type);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ jobs: Job[]; total: number }>(`/api/v1/jobs?${query.toString()}`);
  }

  public async adminListJobs(params?: { tenantId?: string; status?: string; type?: string; limit?: number; offset?: number }): Promise<{ jobs: Job[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.tenantId) query.set('tenantId', params.tenantId);
    if (params?.status) query.set('status', params.status);
    if (params?.type) query.set('type', params.type);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ jobs: Job[]; total: number }>(`/api/v1/admin/jobs?${query.toString()}`);
  }

  public async getJob(id: string): Promise<Job> {
    return this.request<Job>(`/api/v1/jobs/${id}`);
  }

  public async cancelJob(id: string, reason?: string): Promise<void> {
    const query = reason ? `?reason=${encodeURIComponent(reason)}` : '';
    await this.request(`/api/v1/jobs/${id}/cancel${query}`, { method: 'POST' });
  }

  public async retryJob(id: string): Promise<Job> {
    return this.request<Job>(`/api/v1/jobs/${id}/retry`, { method: 'POST' });
  }

  // --- Phase 15: Workload Migrations & Node Operations ---

  public async listMigrations(params?: { tenantId?: string; instanceId?: string; sourceNodeId?: string; destNodeId?: string; status?: string; limit?: number; offset?: number }): Promise<{ migrations: WorkloadMigration[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.tenantId) query.set('tenantId', params.tenantId);
    if (params?.instanceId) query.set('instanceId', params.instanceId);
    if (params?.sourceNodeId) query.set('sourceNodeId', params.sourceNodeId);
    if (params?.destNodeId) query.set('destNodeId', params.destNodeId);
    if (params?.status) query.set('status', params.status);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ migrations: WorkloadMigration[]; total: number }>(`/api/v1/admin/migrations?${query.toString()}`);
  }

  public async getMigration(id: string): Promise<WorkloadMigration> {
    return this.request<WorkloadMigration>(`/api/v1/admin/migrations/${id}`);
  }

  public async migrateInstance(instanceId: string, destNodeId?: string, type: 'live' | 'cold' = 'cold'): Promise<WorkloadMigration> {
    return this.request<WorkloadMigration>(`/api/v1/admin/instances/${instanceId}/migrate`, {
      method: 'POST',
      body: JSON.stringify({ destNodeId, type }),
    });
  }

  public async drainNode(nodeId: string): Promise<void> {
    await this.request(`/api/v1/admin/nodes/${nodeId}/drain`, { method: 'POST' });
  }

  public async undrainNode(nodeId: string): Promise<void> {
    await this.request(`/api/v1/admin/nodes/${nodeId}/undrain`, { method: 'POST' });
  }

  public async evacuateNode(nodeId: string, destNodeId?: string): Promise<{ nodeId: string; totalWorkloads: number; migratedCount: number; failedCount: number; migrations: WorkloadMigration[]; errors?: string[] }> {
    return this.request<{ nodeId: string; totalWorkloads: number; migratedCount: number; failedCount: number; migrations: WorkloadMigration[]; errors?: string[] }>(`/api/v1/admin/nodes/${nodeId}/evacuate`, {
      method: 'POST',
      body: JSON.stringify({ destNodeId }),
    });
  }

  // --- Phase 16: Disaster Recovery, Backups, Key Rotation & Diagnostics ---

  public async listBackups(params?: { tenantId?: string; resourceType?: string; status?: string; limit?: number; offset?: number }): Promise<{ backups: BackupRecord[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.tenantId) query.set('tenantId', params.tenantId);
    if (params?.resourceType) query.set('resourceType', params.resourceType);
    if (params?.status) query.set('status', params.status);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ backups: BackupRecord[]; total: number }>(`/api/v1/backups?${query.toString()}`);
  }

  public async createBackup(data: { resourceType: string; resourceId?: string; type: 'full' | 'incremental' | 'point_in_time'; metadata?: Record<string, any>; retentionDays?: number }): Promise<BackupRecord> {
    return this.request<BackupRecord>('/api/v1/backups', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  public async getBackup(id: string): Promise<BackupRecord> {
    return this.request<BackupRecord>(`/api/v1/backups/${id}`);
  }

  public async verifyBackup(id: string): Promise<{ status: string; checksum: string }> {
    return this.request<{ status: string; checksum: string }>(`/api/v1/backups/${id}/verify`, { method: 'POST' });
  }

  public async deleteBackup(id: string): Promise<void> {
    await this.request(`/api/v1/backups/${id}`, { method: 'DELETE' });
  }

  public async listAdminBackups(params?: { tenantId?: string; resourceType?: string; status?: string; limit?: number; offset?: number }): Promise<{ backups: BackupRecord[]; total: number }> {
    const query = new URLSearchParams();
    if (params?.tenantId) query.set('tenantId', params.tenantId);
    if (params?.resourceType) query.set('resourceType', params.resourceType);
    if (params?.status) query.set('status', params.status);
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    return this.request<{ backups: BackupRecord[]; total: number }>(`/api/v1/admin/recovery/backups?${query.toString()}`);
  }

  public async createAdminBackup(data: { tenantId?: string; resourceType: string; resourceId?: string; type: 'full' | 'incremental' | 'point_in_time'; metadata?: Record<string, any>; retentionDays?: number }): Promise<BackupRecord> {
    return this.request<BackupRecord>('/api/v1/admin/recovery/backups', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  public async verifyAdminBackup(id: string): Promise<{ status: string; checksum: string }> {
    return this.request<{ status: string; checksum: string }>(`/api/v1/admin/recovery/backups/${id}/verify`, { method: 'POST' });
  }

  public async dryRunRecovery(backupId: string): Promise<RestorePlan> {
    return this.request<RestorePlan>('/api/v1/admin/recovery/dry-run', {
      method: 'POST',
      body: JSON.stringify({ backupId }),
    });
  }

  public async restoreRecovery(backupId: string, confirmedDr = true): Promise<RestorePlan> {
    return this.request<RestorePlan>('/api/v1/admin/recovery/restore', {
      method: 'POST',
      body: JSON.stringify({ backupId, confirmedDr }),
    });
  }

  public async listRestorePlans(limit = 50, offset = 0): Promise<{ plans: RestorePlan[]; total: number }> {
    return this.request<{ plans: RestorePlan[]; total: number }>(`/api/v1/admin/recovery/plans?limit=${limit}&offset=${offset}`);
  }

  public async reconcileState(dryRun = false): Promise<ReconciliationReport> {
    return this.request<ReconciliationReport>('/api/v1/admin/reconcile', {
      method: 'POST',
      body: JSON.stringify({ dryRun }),
    });
  }

  public async getLatestReconciliation(): Promise<ReconciliationReport | null> {
    return this.request<ReconciliationReport | null>('/api/v1/admin/reconcile/latest');
  }

  public async listKeyRotations(type?: string, limit = 50, offset = 0): Promise<{ keys: KeyRotationRecord[]; total: number }> {
    const query = new URLSearchParams();
    if (type) query.set('type', type);
    query.set('limit', limit.toString());
    query.set('offset', offset.toString());
    return this.request<{ keys: KeyRotationRecord[]; total: number }>(`/api/v1/admin/keys?${query.toString()}`);
  }

  public async rotateKey(data: { type: string; gracePeriodSeconds?: number; reason?: string }): Promise<KeyRotationRecord> {
    return this.request<KeyRotationRecord>('/api/v1/admin/keys/rotate', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  public async revokeKey(id: string, reason?: string): Promise<KeyRotationRecord> {
    return this.request<KeyRotationRecord>(`/api/v1/admin/keys/${id}/revoke`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    });
  }

  public async getDiagnostics(): Promise<DiagnosticReport> {
    return this.request<DiagnosticReport>('/api/v1/admin/diagnostics');
  }

  public async getRunbooks(): Promise<{ runbooks: RunbookEntry[]; total: number }> {
    return this.request<{ runbooks: RunbookEntry[]; total: number }>('/api/v1/admin/diagnostics/runbooks');
  }
}

export interface BackupRecord {
  id: string;
  tenantId: string;
  resourceType: string;
  resourceId: string;
  type: 'full' | 'incremental' | 'point_in_time';
  status: 'pending' | 'running' | 'verified' | 'failed' | 'expired' | 'deleted';
  storageLocation: string;
  checksumSha256: string;
  sizeBytes: number;
  isProtectedPoint: boolean;
  retentionDays: number;
  metadata?: Record<string, any>;
  errorMessage?: string;
  createdAt: string;
  verifiedAt?: string;
  expiresAt?: string;
}

export interface RestoreAction {
  id: string;
  resourceType: string;
  resourceId: string;
  operation: string;
  status: 'pending' | 'in_progress' | 'succeeded' | 'failed' | 'skipped';
  details?: Record<string, any>;
  error?: string;
}

export interface RestorePlan {
  id: string;
  backupId: string;
  status: 'draft' | 'simulating' | 'ready' | 'in_progress' | 'succeeded' | 'failed';
  dryRun: boolean;
  actions: RestoreAction[];
  discrepanciesFound: number;
  repairsSucceeded: number;
  repairsFailed: number;
  auditHashVerified: boolean;
  estimatedDurationSeconds: number;
  actualDurationMs: number;
  errorMessage?: string;
  createdAt: string;
  completedAt?: string;
}

export interface Discrepancy {
  resourceType: string;
  resourceId: string;
  expected: string;
  actual: string;
  severity: 'info' | 'safe_auto_repair' | 'requires_manual_intervention' | 'critical';
  reason: string;
  autoRepaired: boolean;
  actionTaken?: string;
}

export interface ReconciliationReport {
  id: string;
  triggerSource: string;
  dryRun: boolean;
  totalDiscrepancies: number;
  orphanedInstances: number;
  missingNodes: number;
  staleJobsCount: number;
  abandonedMigrations: number;
  inconsistentQuotas: number;
  repairedCount: number;
  manualActionNeeded: number;
  durationMs: number;
  discrepancies: Discrepancy[];
  createdAt: string;
}

export interface KeyRotationRecord {
  id: string;
  keyId: string;
  type: 'jwt_signing' | 'webhook_secret' | 'database_encryption' | 'mtls_intermediate_ca' | 'backup_encryption';
  version: number;
  status: 'active' | 'grace_period' | 'revoked' | 'expired';
  algorithm: string;
  rotatedBy: string;
  reason?: string;
  revocationReason?: string;
  createdAt: string;
  gracePeriodExpiresAt?: string;
  revokedAt?: string;
}

export interface SubsystemStatus {
  name: string;
  status: 'healthy' | 'degraded' | 'unhealthy';
  latencyMs: number;
  message: string;
  lastChecked: string;
  metadata?: Record<string, any>;
}

export interface RunbookEntry {
  id: string;
  title: string;
  category: string;
  severity: string;
  symptoms: string[];
  rootCauses: string[];
  resolutionSteps: string[];
  verificationCommand: string;
  automatedRemediationAvailable: boolean;
}

export interface DiagnosticReport {
  timestamp: string;
  overallStatus: 'healthy' | 'degraded' | 'critical';
  subsystems: Record<string, SubsystemStatus>;
  activeAlerts: number;
  runbooks: RunbookEntry[];
}

export const api = new ApiClient();
