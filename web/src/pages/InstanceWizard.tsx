import React, { useState, useEffect } from 'react';
import {
  Layers,
  Cpu,
  Network,
  Terminal,
  CheckCircle2,
  ArrowRight,
  ArrowLeft,
  Loader2,
  AlertTriangle,
  CreditCard,
} from 'lucide-react';
import { OSTemplate, api, QuotaSet, BillingPlan } from '../lib/api';
import { useToast } from '../context/ToastContext';
import { useJobs } from '../context/JobsContext';

interface InstanceWizardProps {
  templates: OSTemplate[];
  initialTemplateSlug?: string;
  navigate: (path: string) => void;
}

export const InstanceWizard: React.FC<InstanceWizardProps> = ({
  templates,
  initialTemplateSlug,
  navigate,
}) => {
  const [step, setStep] = useState<number>(1);
  const [loading, setLoading] = useState<boolean>(false);
  const [quotas, setQuotas] = useState<QuotaSet>({});
  const [plan, setPlan] = useState<BillingPlan | null>(null);

  useEffect(() => {
    api.getQuotas().then((res) => {
      setQuotas(res.quotas || {});
      setPlan(res.plan || null);
    }).catch(() => {});
  }, []);

  // Step 1: Template selection
  const [selectedTemplate, setSelectedTemplate] = useState<OSTemplate | null>(() => {
    if (initialTemplateSlug) {
      return templates.find((t) => t.slug === initialTemplateSlug) || templates[0] || null;
    }
    return templates[0] || null;
  });
  const [selectedType, setSelectedType] = useState<'container' | 'virtual-machine'>('container');

  // Step 2: Resources
  const [instanceName, setInstanceName] = useState<string>('vps-workload-01');
  const [cpuCores, setCpuCores] = useState<number>(2);
  const [memoryGb, setMemoryGb] = useState<number>(2);
  const [storageGb, setStorageGb] = useState<number>(20);

  // Step 3: Networking
  const [enableIpv4, setEnableIpv4] = useState<boolean>(true);
  const [enableIpv6, setEnableIpv6] = useState<boolean>(true);

  // Step 4: Cloud-Init
  const [ciHostname, setCiHostname] = useState<string>('aurora-vps-01');
  const [ciUser, setCiUser] = useState<string>('aurora');
  const [ciSshKey, setCiSshKey] = useState<string>('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleSSHKeyForAuroraCloud admin@aurora');
  const [ciPackages, setCiPackages] = useState<string>('curl, htop, ufw, nginx');

  const toast = useToast();
  const { addJob, updateJob } = useJobs();

  const handleDeploy = async () => {
    setLoading(true);
    const jobId = addJob({
      type: 'instance_provision',
      title: `Provision ${instanceName}`,
      targetId: 'new',
      targetName: instanceName,
    });

    try {
      const pkgs = ciPackages.split(',').map((p) => p.trim()).filter(Boolean);
      const newInst = await api.createInstance({
        name: instanceName,
        type: selectedType,
        cpuCores,
        memoryBytes: memoryGb * 1073741824,
        storageBytes: storageGb * 1073741824,
        templateSlug: selectedTemplate?.slug || 'ubuntu-24.04',
        cloudInit: {
          hostname: ciHostname || instanceName,
          timezone: 'UTC',
          users: [
            {
              name: ciUser || 'aurora',
              groups: 'sudo, users',
              sudo: 'ALL=(ALL) NOPASSWD:ALL',
              shell: '/bin/bash',
              sshAuthorizedKeys: ciSshKey ? [ciSshKey] : [],
              lockPasswd: true,
            },
          ],
          packages: pkgs,
          writeFiles: [
            {
              path: '/etc/motd',
              content: `Welcome to Project Aurora Cloud Instance (${instanceName})!\n`,
              permissions: '0644',
            },
          ],
          runcmd: ['systemctl enable --now nginx 2>/dev/null || true'],
        },
        startAfterCreate: true,
      });

      updateJob(jobId, { status: 'completed', targetId: newInst.id });
      toast.success('Instance provisioned successfully!', newInst.name);
      navigate(`/instances/${newInst.id}`);
    } catch (err: any) {
      updateJob(jobId, { status: 'failed', errorMessage: err.message });
      toast.error('Provisioning failed', err.message);
    } finally {
      setLoading(false);
    }
  };

  const steps = [
    { num: 1, title: 'Operating System', icon: Layers },
    { num: 2, title: 'Compute & Sizing', icon: Cpu },
    { num: 3, title: 'Networking', icon: Network },
    { num: 4, title: 'Cloud-Init Engine', icon: Terminal },
    { num: 5, title: 'Review & Deploy', icon: CheckCircle2 },
  ];

  return (
    <div className="max-w-4xl mx-auto space-y-8 animate-in fade-in duration-200">
      {/* Wizard Step Progress Bar */}
      <div className="p-4 sm:p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235]">
        <div className="flex items-center justify-between">
          {steps.map((s, idx) => {
            const Icon = s.icon;
            const isCompleted = step > s.num;
            const isCurrent = step === s.num;
            return (
              <React.Fragment key={s.num}>
                {idx > 0 && (
                  <div
                    className={`flex-1 h-0.5 mx-2 ${
                      step > idx ? 'bg-blue-600' : 'bg-[#1e2538]'
                    }`}
                  />
                )}
                <div
                  onClick={() => s.num < step && setStep(s.num)}
                  className={`flex flex-col items-center gap-1.5 cursor-pointer ${
                    isCurrent
                      ? 'text-blue-400 font-bold'
                      : isCompleted
                      ? 'text-emerald-400'
                      : 'text-slate-500'
                  }`}
                >
                  <div
                    className={`w-8 h-8 rounded-xl flex items-center justify-center text-xs transition ${
                      isCurrent
                        ? 'bg-blue-600 text-white shadow-lg shadow-blue-600/30'
                        : isCompleted
                        ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                        : 'bg-[#141824] text-slate-500 border border-[#232a3d]'
                    }`}
                  >
                    <Icon className="w-4 h-4" />
                  </div>
                  <span className="hidden sm:inline text-[11px] font-mono tracking-tight">
                    {s.title}
                  </span>
                </div>
              </React.Fragment>
            );
          })}
        </div>
      </div>

      {/* Step Contents */}
      <div className="p-6 sm:p-8 rounded-3xl bg-[#0f121d] border border-[#1c2235] shadow-2xl">
        {/* STEP 1: OS SELECTION */}
        {step === 1 && (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-bold text-white">Select OS Template & Workload Type</h3>
              <p className="text-xs text-slate-400 mt-1">
                Choose an official distribution image optimized for Incus containers and virtual machines.
              </p>
            </div>

            {/* Type selector */}
            <div className="grid grid-cols-2 gap-4">
              <div
                onClick={() => setSelectedType('container')}
                className={`p-4 rounded-2xl border cursor-pointer transition ${
                  selectedType === 'container'
                    ? 'bg-blue-600/15 border-blue-500 shadow-lg shadow-blue-500/10'
                    : 'bg-[#090b12] border-[#181f30] hover:border-slate-700'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold text-sm text-white">System Container (LXC)</span>
                  <span className="text-[10px] px-2 py-0.5 rounded font-mono bg-blue-500/10 text-blue-400">
                    Fastest
                  </span>
                </div>
                <p className="text-xs text-slate-400 mt-2 leading-relaxed">
                  Near-zero overhead, instantaneous startup, shared kernel isolation.
                </p>
              </div>

              <div
                onClick={() => setSelectedType('virtual-machine')}
                className={`p-4 rounded-2xl border cursor-pointer transition ${
                  selectedType === 'virtual-machine'
                    ? 'bg-blue-600/15 border-blue-500 shadow-lg shadow-blue-500/10'
                    : 'bg-[#090b12] border-[#181f30] hover:border-slate-700'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold text-sm text-white">Virtual Machine (KVM)</span>
                  <span className="text-[10px] px-2 py-0.5 rounded font-mono bg-purple-500/10 text-purple-400">
                    Full Isolation
                  </span>
                </div>
                <p className="text-xs text-slate-400 mt-2 leading-relaxed">
                  Hardware-assisted virtualization, custom kernel modules, UEFI & VNC support.
                </p>
              </div>
            </div>

            {/* OS Cards Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4">
              {templates.map((tmpl) => {
                const isSelected = selectedTemplate?.id === tmpl.id;
                return (
                  <div
                    key={tmpl.id}
                    onClick={() => setSelectedTemplate(tmpl)}
                    className={`p-4 rounded-2xl border cursor-pointer transition flex flex-col justify-between ${
                      isSelected
                        ? 'bg-blue-600/15 border-blue-500 shadow-lg shadow-blue-500/10'
                        : 'bg-[#090b12] border-[#181f30] hover:border-slate-700'
                    }`}
                  >
                    <div>
                      <div className="flex items-center justify-between">
                        <span className="font-bold text-xs text-white">{tmpl.name}</span>
                        {tmpl.cloudInitSupported && (
                          <span className="text-[9px] px-1.5 py-0.5 rounded font-mono bg-emerald-500/10 text-emerald-400">
                            Cloud-Init
                          </span>
                        )}
                      </div>
                      <p className="text-[11px] text-slate-400 mt-2 line-clamp-2">{tmpl.description}</p>
                    </div>

                    <div className="mt-4 pt-3 border-t border-[#141a29] flex items-center justify-between text-[10px] font-mono text-slate-400">
                      <span>{tmpl.distribution}</span>
                      <span>{tmpl.supportedArchitectures.join(', ')}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* STEP 2: RESOURCES */}
        {step === 2 && (
          <div className="space-y-6">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div>
                <h3 className="text-lg font-bold text-white">Size Compute & Hardware Limits</h3>
                <p className="text-xs text-slate-400 mt-1">
                  Configure vCPU allocation, dedicated memory, and NVMe root disk capacity.
                </p>
              </div>
              {plan && (
                <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-blue-500/10 border border-blue-500/20 text-xs text-blue-300">
                  <CreditCard className="w-3.5 h-3.5 text-blue-400" />
                  <span>
                    Plan: <strong>{plan.name}</strong> (Usage: {quotas['vcpu_hours']?.currentUsage || 0}/{plan.maxVcpu} vCPU, {quotas['ram_gb_hours']?.currentUsage || 0}/{plan.maxMemoryMb / 1024} GB RAM)
                  </span>
                </div>
              )}
            </div>

            {plan && cpuCores > plan.maxVcpu && (
              <div className="p-3 bg-amber-500/10 border border-amber-500/30 rounded-xl flex items-center gap-2.5 text-xs text-amber-300">
                <AlertTriangle className="w-4 h-4 flex-shrink-0" />
                <span>Requested vCPU ({cpuCores}) exceeds plan limit ({plan.maxVcpu}). Upgrade plan in Billing or lower selection.</span>
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Instance Name / Hostname
                </label>
                <input
                  type="text"
                  value={instanceName}
                  onChange={(e) => setInstanceName(e.target.value)}
                  className="w-full px-4 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              {/* Sliders */}
              <div className="p-5 rounded-2xl bg-[#090b12] border border-[#181f30] space-y-4">
                <div>
                  <div className="flex justify-between text-xs font-semibold mb-1">
                    <span className="text-slate-300">vCPU Allocation</span>
                    <span className="text-blue-400 font-mono">{cpuCores} Cores</span>
                  </div>
                  <input
                    type="range"
                    min={1}
                    max={16}
                    value={cpuCores}
                    onChange={(e) => setCpuCores(parseInt(e.target.value))}
                    className="w-full"
                  />
                </div>

                <div>
                  <div className="flex justify-between text-xs font-semibold mb-1">
                    <span className="text-slate-300">RAM (Memory)</span>
                    <span className="text-purple-400 font-mono">{memoryGb} GB</span>
                  </div>
                  <input
                    type="range"
                    min={1}
                    max={64}
                    value={memoryGb}
                    onChange={(e) => setMemoryGb(parseInt(e.target.value))}
                    className="w-full"
                  />
                </div>

                <div>
                  <div className="flex justify-between text-xs font-semibold mb-1">
                    <span className="text-slate-300">Storage (NVMe)</span>
                    <span className="text-emerald-400 font-mono">{storageGb} GB</span>
                  </div>
                  <input
                    type="range"
                    min={10}
                    max={500}
                    value={storageGb}
                    onChange={(e) => setStorageGb(parseInt(e.target.value))}
                    className="w-full"
                  />
                </div>
              </div>
            </div>
          </div>
        )}

        {/* STEP 3: NETWORKING */}
        {step === 3 && (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-bold text-white">Network & IP Addressing</h3>
              <p className="text-xs text-slate-400 mt-1">
                Configure IP address allocation and automated security filtering.
              </p>
            </div>

            <div className="space-y-4">
              <label className="flex items-center gap-3 p-4 rounded-2xl bg-[#090b12] border border-[#181f30] cursor-pointer">
                <input
                  type="checkbox"
                  checked={enableIpv4}
                  onChange={(e) => setEnableIpv4(e.target.checked)}
                  className="rounded text-blue-600"
                />
                <div>
                  <div className="text-xs font-bold text-white">Allocate Public Dedicated IPv4</div>
                  <div className="text-[11px] text-slate-400 font-mono mt-0.5">
                    High-speed connectivity with automatic reverse DNS support.
                  </div>
                </div>
              </label>

              <label className="flex items-center gap-3 p-4 rounded-2xl bg-[#090b12] border border-[#181f30] cursor-pointer">
                <input
                  type="checkbox"
                  checked={enableIpv6}
                  onChange={(e) => setEnableIpv6(e.target.checked)}
                  className="rounded text-blue-600"
                />
                <div>
                  <div className="text-xs font-bold text-white">Allocate Global Routed /64 IPv6</div>
                  <div className="text-[11px] text-slate-400 font-mono mt-0.5">
                    Next-generation dual-stack IPv6 block attached to primary vNIC.
                  </div>
                </div>
              </label>
            </div>
          </div>
        )}

        {/* STEP 4: CLOUD-INIT */}
        {step === 4 && (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-bold text-white">Cloud-Init First-Boot Engine</h3>
              <p className="text-xs text-slate-400 mt-1">
                Automate guest users, SSH authorized keys, and software package installation.
              </p>
            </div>

            <div className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    System Hostname
                  </label>
                  <input
                    type="text"
                    value={ciHostname}
                    onChange={(e) => setCiHostname(e.target.value)}
                    className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Default Sudo User
                  </label>
                  <input
                    type="text"
                    value={ciUser}
                    onChange={(e) => setCiUser(e.target.value)}
                    className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  SSH Public Key (OpenSSH / ed25519)
                </label>
                <input
                  type="text"
                  value={ciSshKey}
                  onChange={(e) => setCiSshKey(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Pre-installed Packages (comma separated)
                </label>
                <input
                  type="text"
                  value={ciPackages}
                  onChange={(e) => setCiPackages(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>
            </div>
          </div>
        )}

        {/* STEP 5: REVIEW & DEPLOY */}
        {step === 5 && (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-bold text-white">Review Configuration & Deploy</h3>
              <p className="text-xs text-slate-400 mt-1">
                Verify instance parameters before initiating hypervisor provisioning.
              </p>
            </div>

            <div className="p-5 rounded-2xl bg-[#090b12] border border-[#181f30] space-y-3 font-mono text-xs">
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Instance Name:</span>
                <span className="text-white font-bold">{instanceName}</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Workload Type:</span>
                <span className="text-blue-400 font-bold uppercase">{selectedType}</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">OS Template:</span>
                <span className="text-emerald-400 font-bold">{selectedTemplate?.name}</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Compute Sizing:</span>
                <span className="text-purple-400">{cpuCores} vCPU • {memoryGb} GB RAM • {storageGb} GB NVMe</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Networking:</span>
                <span className="text-slate-300">Dual-Stack IPv4/IPv6</span>
              </div>
              <div className="flex justify-between py-1.5">
                <span className="text-slate-400">Cloud-Init:</span>
                <span className="text-emerald-400">User "{ciUser}" with SSH Injection</span>
              </div>
            </div>
          </div>
        )}

        {/* Wizard Controls */}
        <div className="flex items-center justify-between pt-6 border-t border-[#181f30] mt-8">
          {step > 1 ? (
            <button
              onClick={() => setStep(step - 1)}
              className="flex items-center gap-1.5 px-4 py-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-300 text-xs font-semibold transition"
            >
              <ArrowLeft className="w-4 h-4" />
              <span>Previous</span>
            </button>
          ) : (
            <div />
          )}

          {step < 5 ? (
            <button
              onClick={() => setStep(step + 1)}
              className="flex items-center gap-1.5 px-5 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition"
            >
              <span>Continue</span>
              <ArrowRight className="w-4 h-4" />
            </button>
          ) : (
            <button
              onClick={handleDeploy}
              disabled={loading}
              className="flex items-center gap-2 px-6 py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold shadow-lg shadow-emerald-600/25 transition"
            >
              {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <CheckCircle2 className="w-4 h-4" />}
              <span>Launch Workload</span>
            </button>
          )}
        </div>
      </div>
    </div>
  );
};
