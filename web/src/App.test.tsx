import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import App from './App';
import { api } from './lib/api';

describe('Aurora Web Portal (Customer & Admin)', () => {
  beforeEach(() => {
    api.clearTokens();
    window.history.pushState({}, '', '/');
    vi.restoreAllMocks();
  });

  it('renders sign-in form when unauthenticated', async () => {
    render(<App />);

    expect(screen.getByText('Project Aurora')).toBeDefined();
    expect(screen.getByText('Sign in to access your cloud workloads and console')).toBeDefined();
    expect(screen.getByPlaceholderText('admin or user@domain.com')).toBeDefined();
    expect(screen.getByText('Sign In')).toBeDefined();
  });

  it('renders Customer Dashboard when user is authenticated as tenant', async () => {
    api.setTokens('mock-valid-jwt-token');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'user-01',
                  username: 'customer-user',
                  email: 'user@tenant.local',
                  roles: ['customer'],
                  permissions: ['instance:read', 'instance:create'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/instances')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: [
                {
                  id: 'inst-prod-01',
                  userId: 'user-01',
                  nodeId: 'node-alpha-01',
                  name: 'vps-ubuntu-primary',
                  type: 'container',
                  status: 'running',
                  cpuCores: 4,
                  memoryBytes: 4294967296,
                  storageBytes: 42949672960,
                  image: 'images:ubuntu/24.04',
                  ipv4Address: '10.0.3.150',
                  ipv6Address: 'fd42:4242:4242::150',
                  createdAt: new Date().toISOString(),
                  updatedAt: new Date().toISOString(),
                },
              ],
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/templates')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                templates: [
                  {
                    id: 'tmpl-ubuntu-24-04',
                    name: 'Ubuntu 24.04 LTS (Noble Numbat)',
                    slug: 'ubuntu-24.04',
                    description: 'Canonical Ubuntu Server 24.04 LTS',
                    distribution: 'ubuntu',
                    version: '24.04',
                    release: 'noble',
                    supportedArchitectures: ['x86_64', 'aarch64'],
                    supportedInstanceTypes: ['container', 'virtual-machine'],
                    minDiskBytes: 5368709120,
                    minMemoryBytes: 536870912,
                    cloudInitSupported: true,
                    status: 'active',
                  },
                ],
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: {} }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('AURORA')).toBeDefined();
      expect(screen.getByText('Customer Workloads & Infrastructure')).toBeDefined();
      expect(screen.getByText('vps-ubuntu-primary')).toBeDefined();
    });
  });

  it('blocks customer tenant from accessing /admin with 403 Forbidden', async () => {
    api.setTokens('mock-customer-token');
    window.history.pushState({}, '', '/admin');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'user-02',
                  username: 'restricted-customer',
                  email: 'customer@domain.com',
                  roles: ['customer'],
                  permissions: ['instance:read'],
                },
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('403 — Unauthorized Access')).toBeDefined();
      expect(
        screen.getByText('Your account does not possess operator or hypervisor administration privileges.')
      ).toBeDefined();
    });
  });

  it('renders Admin Dashboard and Fleet for superadmin users', async () => {
    api.setTokens('mock-admin-token');
    window.history.pushState({}, '', '/admin');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'user-admin',
                  username: 'superadmin',
                  email: 'admin@aurora.local',
                  roles: ['superadmin'],
                  permissions: ['*'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/nodes')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: [
                {
                  id: 'node-alpha-01',
                  name: 'node-alpha-01',
                  fqdn: '127.0.0.1',
                  locationId: 'loc-us-east-1',
                  status: 'online',
                  maintenanceMode: false,
                  capabilities: { hypervisor: 'incus' },
                  lastHeartbeatAt: new Date().toISOString(),
                  enrolledAt: new Date().toISOString(),
                },
              ],
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/storage/pools')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                pools: [
                  {
                    id: 'pool-01',
                    nodeId: 'node-alpha-01',
                    name: 'nvme-zfs-pool',
                    driver: 'zfs',
                    totalBytes: 536870912000,
                    usedBytes: 107374182400,
                    freeBytes: 429496729600,
                    status: 'online',
                  },
                ],
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/ipam/pools')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                pools: [
                  {
                    id: 'ipam-01',
                    name: 'prod-ipv4',
                    cidr: '10.0.3.0/24',
                    ipVersion: 4,
                    gateway: '10.0.3.1',
                    dnsServers: ['1.1.1.1'],
                    totalIps: 254,
                    allocatedIps: 12,
                    status: 'active',
                  },
                ],
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/audit/logs')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                logs: [
                  {
                    id: 1,
                    actorId: 'admin-01',
                    action: 'node.enrolled',
                    tamperProofHash: 'a1b2c3d4e5f678901234567890abcdef',
                    createdAt: new Date().toISOString(),
                  },
                ],
                total: 1,
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('Cluster Health & Hypervisor Fleet')).toBeDefined();
      expect(screen.getByText('Hypervisor Nodes Status')).toBeDefined();
      expect(screen.getByText('Admin Fleet')).toBeDefined();
    });
  });

  it('renders Customer Billing & Plans page at /billing', async () => {
    api.setTokens('mock-valid-jwt-token');
    window.history.pushState({}, '', '/billing');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'user-01',
                  username: 'customer-user',
                  email: 'user@tenant.local',
                  roles: ['customer'],
                  permissions: ['billing:read', 'billing:manage'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/billing/plans')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                plans: [
                  {
                    id: 'plan-starter',
                    name: 'Starter Developer',
                    slug: 'starter',
                    description: 'Entry level compute',
                    currency: 'EUR',
                    monthlyPriceMinor: 500,
                    yearlyPriceMinor: 5000,
                    includedVcpu: 1,
                    includedMemoryMb: 1024,
                    includedStorageMb: 10240,
                    maxVcpu: 4,
                    maxMemoryMb: 8192,
                    maxInstances: 5,
                    active: true,
                  },
                ],
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/billing/subscription')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                subscription: {
                  id: 'sub-01',
                  userId: 'user-01',
                  planId: 'plan-starter',
                  status: 'active',
                  billingCycle: 'monthly',
                  currentPeriodStart: new Date().toISOString(),
                  currentPeriodEnd: new Date().toISOString(),
                },
                plan: {
                  id: 'plan-starter',
                  name: 'Starter Developer',
                  monthlyPriceMinor: 500,
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/billing/quotas')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                quotas: {
                  vcpu_hours: { metric: 'vcpu_hours', limit: 4, currentUsage: 1 },
                },
                plan: { id: 'plan-starter', name: 'Starter Developer' },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/billing/invoices')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: { invoices: [] },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('Billing & Resource Plans')).toBeDefined();
      expect(screen.getByText('Choose or Upgrade Plan')).toBeDefined();
    });
  });

  it('renders Admin Infrastructure Billing page at /admin/billing for superadmin', async () => {
    api.setTokens('mock-valid-jwt-token');
    window.history.pushState({}, '', '/admin/billing');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'admin-01',
                  username: 'superadmin',
                  email: 'admin@aurora.local',
                  roles: ['superadmin'],
                  permissions: ['*'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/admin/billing/plans')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                plans: [
                  {
                    id: 'plan-pro',
                    name: 'Pro Production',
                    slug: 'pro',
                    monthlyPriceMinor: 2000,
                    active: true,
                  },
                ],
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('Admin Infrastructure Billing & Quotas')).toBeDefined();
      expect(screen.getByText('Create Plan')).toBeDefined();
    });
  });

  it('renders Customer Notification Center at /notifications', async () => {
    api.setTokens('mock-valid-jwt-token');
    window.history.pushState({}, '', '/notifications');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'user-01',
                  username: 'customer-user',
                  email: 'user@tenant.local',
                  roles: ['customer'],
                  permissions: ['notification:read', 'notification:manage'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/notifications/unread-count')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, data: { unreadCount: 1 } }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/notifications')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                notifications: [
                  {
                    id: 'notif-01',
                    tenantId: 'user-01',
                    userId: 'user-01',
                    type: 'instance.created',
                    title: 'Instance Provisioned',
                    body: 'Instance vps-ubuntu-primary has been created.',
                    severity: 'success',
                    createdAt: new Date().toISOString(),
                  },
                ],
                total: 1,
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('Notification Center')).toBeDefined();
      expect(screen.getByText('Instance Provisioned')).toBeDefined();
    });
  });

  it('renders Webhooks Management at /webhooks', async () => {
    api.setTokens('mock-valid-jwt-token');
    window.history.pushState({}, '', '/webhooks');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'user-01',
                  username: 'customer-user',
                  email: 'user@tenant.local',
                  roles: ['customer'],
                  permissions: ['webhook:read', 'webhook:create'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/webhooks')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                webhooks: [
                  {
                    id: 'wh-01',
                    tenantId: 'user-01',
                    name: 'Slack Alerts Webhook',
                    url: 'https://hooks.slack.com/services/test',
                    active: true,
                    eventTypes: ['instance.*'],
                    failureCount: 0,
                    createdAt: new Date().toISOString(),
                    updatedAt: new Date().toISOString(),
                  },
                ],
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('Webhooks & Event Subscriptions')).toBeDefined();
      expect(screen.getByText('Slack Alerts Webhook')).toBeDefined();
      expect(screen.getByText('HMAC-SHA256 Webhook Verification')).toBeDefined();
    });
  });

  it('renders Admin Platform Events at /admin/events', async () => {
    api.setTokens('mock-valid-jwt-token');
    window.history.pushState({}, '', '/admin/events');

    vi.spyOn(global, 'fetch').mockImplementation((url) => {
      const urlStr = url.toString();
      if (urlStr.includes('/api/v1/auth/me')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                user: {
                  id: 'admin-01',
                  username: 'superadmin',
                  email: 'admin@aurora.local',
                  roles: ['superadmin'],
                  permissions: ['*'],
                },
              },
            }),
        } as Response);
      }
      if (urlStr.includes('/api/v1/admin/events')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              success: true,
              data: {
                events: [
                  {
                    id: 'ev-01',
                    tenantId: 'user-01',
                    type: 'instance.created',
                    resourceType: 'instance',
                    resourceId: 'inst-01',
                    timestamp: new Date().toISOString(),
                    payload: {},
                    version: '1.0',
                  },
                ],
                total: 1,
              },
            }),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: [] }),
      } as Response);
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText('Platform Event & Delivery Hub')).toBeDefined();
      expect(screen.getByText('Event Bus Active')).toBeDefined();
    });
  });
});
