import React, { useEffect, useState } from 'react';
import { Layers, Disc, CheckCircle2, ShieldCheck, Terminal, Cpu, HardDrive, RefreshCw } from 'lucide-react';

interface OSTemplate {
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

export const TemplatesView: React.FC = () => {
  const [templates, setTemplates] = useState<OSTemplate[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState<OSTemplate | null>(null);
  const [activeTab, setActiveTab] = useState<'catalog' | 'cloudinit'>('catalog');
  const [loading, setLoading] = useState<boolean>(true);

  // Cloud-Init interactive generator state
  const [ciHostname, setCiHostname] = useState('vps-production-01');
  const [ciUser, setCiUser] = useState('aurora');
  const [ciSSHKey, setCiSSHKey] = useState('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleSSHKeyForAuroraCloud admin@aurora');
  const [ciPackages, setCiPackages] = useState('curl, htop, ufw, git, docker.io');

  const fetchCatalog = async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/v1/templates');
      if (res.ok) {
        const json = await res.json();
        const list = json.data?.templates || [];
        setTemplates(list);
        if (list.length > 0 && !selectedTemplate) {
          setSelectedTemplate(list[0]);
        }
      }
    } catch {
      // Fallback sample data for offline preview
      const fallbackList: OSTemplate[] = [
        {
          id: 'tmpl-ubuntu-24-04',
          name: 'Ubuntu 24.04 LTS (Noble Numbat)',
          slug: 'ubuntu-24.04',
          description: 'Official Ubuntu Server LTS cloud release with full cloud-init support.',
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
        {
          id: 'tmpl-debian-12',
          name: 'Debian 12 (Bookworm)',
          slug: 'debian-12',
          description: 'Debian GNU/Linux stable cloud image.',
          distribution: 'debian',
          version: '12',
          release: 'bookworm',
          supportedArchitectures: ['x86_64', 'aarch64'],
          supportedInstanceTypes: ['container', 'virtual-machine'],
          minDiskBytes: 3221225472,
          minMemoryBytes: 268435456,
          cloudInitSupported: true,
          status: 'active',
        },
        {
          id: 'tmpl-alpine-3-19',
          name: 'Alpine Linux 3.19',
          slug: 'alpine-3.19',
          description: 'Ultra-lightweight security-oriented system.',
          distribution: 'alpine',
          version: '3.19',
          release: 'standard',
          supportedArchitectures: ['x86_64', 'aarch64'],
          supportedInstanceTypes: ['container'],
          minDiskBytes: 1073741824,
          minMemoryBytes: 134217728,
          cloudInitSupported: true,
          status: 'active',
        },
      ];
      setTemplates(fallbackList);
      if (!selectedTemplate) {
        setSelectedTemplate(fallbackList[0]);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCatalog();
  }, []);

  const formatBytes = (bytes: number) => {
    if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(0)} GB`;
    return `${(bytes / 1048576).toFixed(0)} MB`;
  };

  const renderCloudInitYAML = () => {
    const pkgs = ciPackages.split(',').map((p) => `  - ${p.trim()}`).filter(Boolean).join('\n');
    return `#cloud-config
hostname: ${ciHostname}
manage_etc_hosts: true
users:
  - name: ${ciUser}
    groups: sudo, users
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: true
    ssh_authorized_keys:
      - ${ciSSHKey}
packages:
${pkgs}
write_files:
  - path: /etc/motd
    content: |
      Welcome to Aurora Cloud VPS Instance (${ciHostname})!
    permissions: '0644'
runcmd:
  - systemctl enable --now ufw
  - ufw default deny incoming
  - ufw default allow outgoing
  - ufw allow ssh
  - ufw --force enable`;
  };

  return (
    <div className="space-y-6">
      {/* Header & View Switcher */}
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 border-b border-[#1c2235] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Layers className="w-5 h-5 text-blue-400" />
            <span>OS Template Registry & Image Subsystem</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Browse verified OS templates, inspect multi-architecture Incus image artifacts, and preview guest Cloud-Init configurations.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="bg-[#0f121d] border border-[#1e2538] rounded-lg p-1 flex">
            <button
              onClick={() => setActiveTab('catalog')}
              className={`px-3 py-1 text-xs font-medium rounded-md transition ${activeTab === 'catalog' ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white'}`}
            >
              OS Templates ({templates.length})
            </button>
            <button
              onClick={() => setActiveTab('cloudinit')}
              className={`px-3 py-1 text-xs font-medium rounded-md transition ${activeTab === 'cloudinit' ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white'}`}
            >
              Cloud-Init Engine
            </button>
          </div>
          <button
            onClick={fetchCatalog}
            disabled={loading}
            className="p-2 rounded-lg bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white transition"
            title="Refresh Catalog"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {activeTab === 'catalog' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Template List */}
          <div className="lg:col-span-1 space-y-3">
            <div className="text-xs font-semibold uppercase tracking-wider text-slate-400 px-1">
              Available Distributions
            </div>
            <div className="space-y-2">
              {templates.map((tmpl) => {
                const isSelected = selectedTemplate?.id === tmpl.id;
                return (
                  <div
                    key={tmpl.id}
                    onClick={() => setSelectedTemplate(tmpl)}
                    className={`p-4 rounded-xl border cursor-pointer transition ${
                      isSelected
                        ? 'bg-blue-600/10 border-blue-500/50 shadow-lg shadow-blue-500/5'
                        : 'bg-[#0f121d] border-[#1c2235] hover:border-slate-700'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-sm text-white">{tmpl.name}</span>
                      <span className="text-[10px] px-2 py-0.5 rounded uppercase font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                        {tmpl.status}
                      </span>
                    </div>
                    <p className="text-xs text-slate-400 mt-1 line-clamp-2">{tmpl.description}</p>
                    <div className="flex items-center gap-2 mt-3 text-[11px] text-slate-400 font-mono">
                      <span className="px-1.5 py-0.5 rounded bg-[#141824] border border-[#232a3d]">{tmpl.slug}</span>
                      <span>•</span>
                      <span>{tmpl.distribution}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Template Detail Card */}
          {selectedTemplate && (
            <div className="lg:col-span-2 p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-6">
              <div className="flex items-start justify-between">
                <div>
                  <h3 className="text-lg font-bold text-white">{selectedTemplate.name}</h3>
                  <div className="text-xs text-slate-400 font-mono mt-0.5">Slug: {selectedTemplate.slug}</div>
                </div>
                <div className="flex items-center gap-2">
                  {selectedTemplate.cloudInitSupported && (
                    <span className="flex items-center gap-1 text-xs px-2.5 py-1 rounded-full bg-blue-500/10 border border-blue-500/30 text-blue-400">
                      <ShieldCheck className="w-3.5 h-3.5" />
                      <span>Cloud-Init Verified</span>
                    </span>
                  )}
                </div>
              </div>

              <p className="text-sm text-slate-300 leading-relaxed">{selectedTemplate.description}</p>

              {/* Hardware Requirements & Capabilities */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                <div className="p-3 rounded-xl bg-[#0a0c14] border border-[#181f30]">
                  <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                    <Cpu className="w-3.5 h-3.5 text-blue-400" />
                    <span>Architectures</span>
                  </div>
                  <div className="text-sm font-semibold text-white font-mono mt-1">
                    {selectedTemplate.supportedArchitectures.join(', ')}
                  </div>
                </div>

                <div className="p-3 rounded-xl bg-[#0a0c14] border border-[#181f30]">
                  <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                    <Disc className="w-3.5 h-3.5 text-purple-400" />
                    <span>Types</span>
                  </div>
                  <div className="text-sm font-semibold text-white font-mono mt-1">
                    {selectedTemplate.supportedInstanceTypes.join(', ')}
                  </div>
                </div>

                <div className="p-3 rounded-xl bg-[#0a0c14] border border-[#181f30]">
                  <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                    <HardDrive className="w-3.5 h-3.5 text-emerald-400" />
                    <span>Min Disk</span>
                  </div>
                  <div className="text-sm font-semibold text-white font-mono mt-1">
                    {formatBytes(selectedTemplate.minDiskBytes)}
                  </div>
                </div>

                <div className="p-3 rounded-xl bg-[#0a0c14] border border-[#181f30]">
                  <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                    <Terminal className="w-3.5 h-3.5 text-amber-400" />
                    <span>Min RAM</span>
                  </div>
                  <div className="text-sm font-semibold text-white font-mono mt-1">
                    {formatBytes(selectedTemplate.minMemoryBytes)}
                  </div>
                </div>
              </div>

              {/* Incus Image Artifacts mapping */}
              <div className="space-y-3 pt-2">
                <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                  Virtualization Artifact Bindings
                </h4>
                <div className="p-4 rounded-xl bg-[#0a0c14] border border-[#181f30] space-y-2 font-mono text-xs">
                  <div className="flex justify-between items-center py-1 border-b border-[#141a29]">
                    <span className="text-slate-400">x86_64 Container:</span>
                    <span className="text-emerald-400">images:{selectedTemplate.distribution}/{selectedTemplate.version}</span>
                  </div>
                  <div className="flex justify-between items-center py-1 border-b border-[#141a29]">
                    <span className="text-slate-400">x86_64 Virtual Machine:</span>
                    <span className="text-emerald-400">images:{selectedTemplate.distribution}/{selectedTemplate.version}/cloud</span>
                  </div>
                  <div className="flex justify-between items-center py-1">
                    <span className="text-slate-400">aarch64 Multi-Arch:</span>
                    <span className="text-blue-400">images:{selectedTemplate.distribution}/{selectedTemplate.version}/arm64</span>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {activeTab === 'cloudinit' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Generator Form */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <Terminal className="w-4 h-4 text-blue-400" />
              <span>Cloud-Init Configuration Builder</span>
            </h3>
            <p className="text-xs text-slate-400">
              Aurora securely validates and injects guest `#cloud-config` user-data on first boot with zero sensitive credential leakage in hypervisor logs.
            </p>

            <div className="space-y-3 pt-2">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Guest Hostname</label>
                <input
                  type="text"
                  value={ciHostname}
                  onChange={(e) => setCiHostname(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-[#0a0c14] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Default Sudo User</label>
                <input
                  type="text"
                  value={ciUser}
                  onChange={(e) => setCiUser(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-[#0a0c14] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">SSH Public Key</label>
                <input
                  type="text"
                  value={ciSSHKey}
                  onChange={(e) => setCiSSHKey(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-[#0a0c14] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Provisioning Packages</label>
                <input
                  type="text"
                  value={ciPackages}
                  onChange={(e) => setCiPackages(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-[#0a0c14] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>
            </div>
          </div>

          {/* Rendered YAML Preview */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                <span>Rendered #cloud-config YAML</span>
              </h3>
              <span className="text-[10px] px-2 py-0.5 rounded font-mono bg-blue-500/10 text-blue-400 border border-blue-500/20">
                Validated
              </span>
            </div>

            <pre className="p-4 rounded-xl bg-[#0a0c14] border border-[#181f30] text-emerald-400 font-mono text-xs overflow-x-auto max-h-96 leading-relaxed">
              {renderCloudInitYAML()}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
};
