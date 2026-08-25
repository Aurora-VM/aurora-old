import React, { useEffect, useState, useRef } from 'react';
import {
  Search,
  Server,
  PlusCircle,
  Layers,
  User,
  ArrowRight,
  Shield,
  Network,
  HardDrive,
  FileCheck,
  Activity,
  CreditCard,
  Bell,
  Webhook,
  ShieldCheck,
  LifeBuoy,
} from 'lucide-react';
import { Instance, OSTemplate } from '../lib/api';
import { useAuth } from '../context/AuthContext';

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  navigate: (path: string) => void;
  instances: Instance[];
  templates: OSTemplate[];
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({
  isOpen,
  onClose,
  navigate,
  instances,
  templates,
}) => {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const { user } = useAuth();

  const isAdmin =
    user?.roles?.some((r) => r === 'superadmin' || r === 'admin') ||
    user?.permissions?.includes('*') ||
    user?.permissions?.includes('node:read');

  useEffect(() => {
    if (isOpen) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  if (!isOpen) return null;

  // Build command suggestions list
  const systemActions = [
    {
      id: 'action-create-instance',
      title: 'Create New Instance',
      subtitle: 'Deploy a new container or virtual machine',
      icon: PlusCircle,
      action: () => navigate('/instances/new'),
    },
    {
      id: 'action-view-instances',
      title: 'View All Instances',
      subtitle: 'Manage active workloads, power state, and networking',
      icon: Server,
      action: () => navigate('/instances'),
    },
    {
      id: 'action-templates',
      title: 'OS Template Registry',
      subtitle: 'Browse official Ubuntu, Debian, Alpine images and Cloud-Init builder',
      icon: Layers,
      action: () => navigate('/templates'),
    },
    {
      id: 'action-backups',
      title: 'Backups & Recovery Points',
      subtitle: 'Encrypted snapshots, SHA-256 integrity verification, and point-in-time restore',
      icon: ShieldCheck,
      action: () => navigate('/backups'),
    },
    {
      id: 'action-billing',
      title: 'Billing & Subscriptions',
      subtitle: 'View active plan, quotas, metered usage, and invoice history',
      icon: CreditCard,
      action: () => navigate('/billing'),
    },
    {
      id: 'action-notifications',
      title: 'Notification Center',
      subtitle: 'Activity alerts, state updates, and security notices',
      icon: Bell,
      action: () => navigate('/notifications'),
    },
    {
      id: 'action-webhooks',
      title: 'Webhooks & Event Subscriptions',
      subtitle: 'Configure HMAC-signed webhook endpoints and inspect deliveries',
      icon: Webhook,
      action: () => navigate('/webhooks'),
    },
    {
      id: 'action-account',
      title: 'Account Settings & API Keys',
      subtitle: 'Manage credentials, 2FA, sessions, and access tokens',
      icon: User,
      action: () => navigate('/account'),
    },
    ...(isAdmin
      ? [
          {
            id: 'admin-nodes',
            title: 'Admin: Hypervisor Fleet',
            subtitle: 'Manage physical and virtual Incus hypervisor nodes',
            icon: Shield,
            action: () => navigate('/admin/nodes'),
          },
          {
            id: 'admin-recovery',
            title: 'Admin: Disaster Recovery & Hardening',
            subtitle: 'Full cluster backups, 4-step DR wizard, state reconciliation, and key lifecycle',
            icon: LifeBuoy,
            action: () => navigate('/admin/recovery'),
          },
          {
            id: 'admin-instances',
            title: 'Admin: Cross-Tenant Workloads',
            subtitle: 'Inspect and control workloads across all customer tenants',
            icon: Server,
            action: () => navigate('/admin/instances'),
          },
          {
            id: 'admin-billing',
            title: 'Admin: Billing & Plan Management',
            subtitle: 'Configure pricing plans, inspect subscriptions and invoices',
            icon: CreditCard,
            action: () => navigate('/admin/billing'),
          },
          {
            id: 'admin-events',
            title: 'Admin: Platform Event & Delivery Hub',
            subtitle: 'Inspect real-time event bus and webhook dead-letter queues',
            icon: Activity,
            action: () => navigate('/admin/events'),
          },
          {
            id: 'admin-ipam',
            title: 'Admin: IPAM Subnets',
            subtitle: 'Manage IPv4/IPv6 address pools, gateways, and VLANs',
            icon: Network,
            action: () => navigate('/admin/ipam'),
          },
          {
            id: 'admin-storage',
            title: 'Admin: Storage Pools',
            subtitle: 'Inspect ZFS, Btrfs, LVM-Thin, and directory storage',
            icon: HardDrive,
            action: () => navigate('/admin/storage'),
          },
          {
            id: 'admin-audit',
            title: 'Admin: Tamper-Proof Audit Ledger',
            subtitle: 'Explore SHA-256 hash-chained security event logs',
            icon: FileCheck,
            action: () => navigate('/admin/audit'),
          },
          {
            id: 'admin-monitoring',
            title: 'Admin: Infrastructure Telemetry',
            subtitle: 'Cluster resource saturation and capacity trends',
            icon: Activity,
            action: () => navigate('/admin/monitoring'),
          },
        ]
      : []),
  ];

  const instanceItems = instances.map((inst) => ({
    id: `inst-${inst.id}`,
    title: inst.name,
    subtitle: `Status: ${inst.status} • IP: ${inst.ipv4Address || 'pending'} • Type: ${inst.type}`,
    icon: Server,
    action: () => navigate(`/instances/${inst.id}`),
  }));

  const templateItems = templates.map((tmpl) => ({
    id: `tmpl-${tmpl.id}`,
    title: tmpl.name,
    subtitle: `Template: ${tmpl.slug} • Arch: ${tmpl.supportedArchitectures.join(', ')}`,
    icon: Layers,
    action: () => navigate(`/instances/new?template=${tmpl.slug}`),
  }));

  const allItems = [...systemActions, ...instanceItems, ...templateItems];

  const filteredItems = query.trim()
    ? allItems.filter(
        (item) =>
          item.title.toLowerCase().includes(query.toLowerCase()) ||
          item.subtitle.toLowerCase().includes(query.toLowerCase())
      )
    : allItems;

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((prev) => (prev + 1 < filteredItems.length ? prev + 1 : 0));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((prev) => (prev - 1 >= 0 ? prev - 1 : filteredItems.length - 1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (filteredItems[selectedIndex]) {
        filteredItems[selectedIndex].action();
        onClose();
      }
    }
  };

  return (
    <div
      onClick={onClose}
      className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-start justify-center pt-20 p-4"
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-xl bg-[#0d101a] border border-[#1e2538] rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[70vh] animate-in fade-in zoom-in-95 duration-150"
      >
        {/* Search Input Bar */}
        <div className="flex items-center px-4 py-3.5 border-b border-[#181f30] gap-3">
          <Search className="w-4 h-4 text-blue-400 flex-shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
            onKeyDown={handleKeyDown}
            placeholder="Type a command, instance name, or template..."
            className="w-full bg-transparent text-sm text-white placeholder-slate-500 focus:outline-none"
          />
          <kbd className="hidden sm:inline-block px-2 py-0.5 rounded bg-[#141824] border border-[#232a3d] text-[10px] font-mono text-slate-400">
            ESC
          </kbd>
        </div>

        {/* Results List */}
        <div className="flex-1 overflow-y-auto p-2 space-y-1">
          {filteredItems.length === 0 ? (
            <div className="p-8 text-center text-xs text-slate-500">
              No matching commands or instances found.
            </div>
          ) : (
            filteredItems.map((item, idx) => {
              const Icon = item.icon;
              const isSelected = idx === selectedIndex;
              return (
                <div
                  key={item.id}
                  onClick={() => {
                    item.action();
                    onClose();
                  }}
                  onMouseEnter={() => setSelectedIndex(idx)}
                  className={`flex items-center justify-between p-2.5 rounded-xl cursor-pointer transition ${
                    isSelected
                      ? 'bg-blue-600/15 border border-blue-500/30 text-white'
                      : 'text-slate-300 hover:bg-[#141824] border border-transparent'
                  }`}
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div
                      className={`p-2 rounded-lg ${
                        isSelected ? 'bg-blue-600 text-white' : 'bg-[#141824] text-slate-400'
                      }`}
                    >
                      <Icon className="w-4 h-4" />
                    </div>
                    <div className="truncate">
                      <div className="text-xs font-semibold">{item.title}</div>
                      <div className="text-[11px] text-slate-400 truncate">{item.subtitle}</div>
                    </div>
                  </div>
                  {isSelected && (
                    <ArrowRight className="w-3.5 h-3.5 text-blue-400 flex-shrink-0 ml-2" />
                  )}
                </div>
              );
            })
          )}
        </div>

        {/* Footer info */}
        <div className="px-4 py-2 bg-[#090b12] border-t border-[#181f30] flex items-center justify-between text-[11px] text-slate-500 font-mono">
          <span>Navigate: ↑ ↓</span>
          <span>Select: ↵</span>
          <span>Close: ESC</span>
        </div>
      </div>
    </div>
  );
};
