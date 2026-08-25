import React, { useState, useEffect } from 'react';
import {
  CreditCard,
  Plus,
  RefreshCw,
  AlertTriangle,
  CheckCircle,
  FileText,
  Users,
  TrendingUp,
  XCircle,
  Layers,
  Search,
  Slash,
} from 'lucide-react';
import { api, BillingPlan, Subscription, Invoice, UsageAggregate } from '../../lib/api';

export const AdminBilling: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'plans' | 'subscriptions' | 'invoices' | 'usage'>('plans');
  const [plans, setPlans] = useState<BillingPlan[]>([]);
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [usage, setUsage] = useState<UsageAggregate | null>(null);
  const [tenantUsageFilter, setTenantUsageFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  // Create Plan Modal State
  const [showCreatePlanModal, setShowCreatePlanModal] = useState(false);
  const [newPlan, setNewPlan] = useState({
    name: '',
    slug: '',
    description: '',
    currency: 'EUR',
    monthlyPriceMinor: 2000,
    yearlyPriceMinor: 20000,
    includedVcpu: 2,
    includedMemoryMb: 4096,
    includedStorageMb: 40960,
    includedIpv4: 1,
    includedSnapshots: 5,
    includedBackups: 2,
    includedBandwidthGb: 2000,
    maxInstances: 10,
    maxVcpu: 16,
    maxMemoryMb: 32768,
    maxStorageMb: 327680,
  });

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);
      if (activeTab === 'plans') {
        const p = await api.adminListPlans();
        setPlans(p);
      } else if (activeTab === 'subscriptions') {
        const s = await api.adminListSubscriptions();
        setSubscriptions(s);
      } else if (activeTab === 'invoices') {
        const invs = await api.adminListInvoices();
        setInvoices(invs);
      } else if (activeTab === 'usage') {
        const u = await api.adminGetUsage(tenantUsageFilter || undefined);
        setUsage(u);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load billing data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [activeTab]);

  const handleCreatePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError(null);
      await api.adminCreatePlan({
        ...newPlan,
        features: { snapshots: true, cloudinit: true, monitoring: true },
      });
      setSuccessMsg(`Plan ${newPlan.name} created successfully!`);
      setShowCreatePlanModal(false);
      await loadData();
    } catch (err: any) {
      setError(err.message || 'Failed to create plan');
    }
  };

  const handleDeactivatePlan = async (id: string) => {
    if (!window.confirm('Are you sure you want to deactivate this plan?')) return;
    try {
      await api.adminDeletePlan(id);
      setSuccessMsg('Plan deactivated successfully');
      await loadData();
    } catch (err: any) {
      setError(err.message || 'Failed to deactivate plan');
    }
  };

  const handleVoidInvoice = async (id: string) => {
    if (!window.confirm('Are you sure you want to void this invoice?')) return;
    try {
      await api.adminVoidInvoice(id);
      setSuccessMsg('Invoice marked as void');
      await loadData();
    } catch (err: any) {
      setError(err.message || 'Failed to void invoice');
    }
  };

  const formatEUR = (cents: number) => `€${(cents / 100).toFixed(2)}`;

  return (
    <div className="space-y-6 animate-fadeIn">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-gray-800 pb-5">
        <div>
          <h1 className="text-2xl font-bold text-gray-100 flex items-center gap-3">
            <CreditCard className="w-7 h-7 text-aurora-accent" />
            Admin Infrastructure Billing & Quotas
          </h1>
          <p className="text-gray-400 text-sm mt-1">
            Configure system plans, manage customer subscriptions, audit usage meters, and manage platform invoices.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {activeTab === 'plans' && (
            <button
              onClick={() => setShowCreatePlanModal(true)}
              className="flex items-center gap-2 px-3 py-1.5 bg-aurora-accent hover:bg-aurora-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition"
            >
              <Plus className="w-4 h-4" />
              Create Plan
            </button>
          )}
          <button
            onClick={loadData}
            disabled={loading}
            className="flex items-center gap-2 px-3 py-1.5 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded-lg border border-gray-700 transition"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
        </div>
      </div>

      {/* Notifications */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 text-red-400 p-4 rounded-lg flex items-center gap-3 text-sm">
          <AlertTriangle className="w-5 h-5 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}
      {successMsg && (
        <div className="bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 p-4 rounded-lg flex items-center gap-3 text-sm">
          <CheckCircle className="w-5 h-5 flex-shrink-0" />
          <span>{successMsg}</span>
        </div>
      )}

      {/* Tabs */}
      <div className="flex border-b border-gray-800 gap-6 text-sm font-medium">
        <button
          onClick={() => setActiveTab('plans')}
          className={`pb-3 flex items-center gap-2 transition ${
            activeTab === 'plans'
              ? 'text-aurora-accent border-b-2 border-aurora-accent font-semibold'
              : 'text-gray-400 hover:text-gray-200'
          }`}
        >
          <Layers className="w-4 h-4" />
          Billing Plans ({plans.length})
        </button>
        <button
          onClick={() => setActiveTab('subscriptions')}
          className={`pb-3 flex items-center gap-2 transition ${
            activeTab === 'subscriptions'
              ? 'text-aurora-accent border-b-2 border-aurora-accent font-semibold'
              : 'text-gray-400 hover:text-gray-200'
          }`}
        >
          <Users className="w-4 h-4" />
          Subscriptions ({subscriptions.length})
        </button>
        <button
          onClick={() => setActiveTab('invoices')}
          className={`pb-3 flex items-center gap-2 transition ${
            activeTab === 'invoices'
              ? 'text-aurora-accent border-b-2 border-aurora-accent font-semibold'
              : 'text-gray-400 hover:text-gray-200'
          }`}
        >
          <FileText className="w-4 h-4" />
          Invoices ({invoices.length})
        </button>
        <button
          onClick={() => setActiveTab('usage')}
          className={`pb-3 flex items-center gap-2 transition ${
            activeTab === 'usage'
              ? 'text-aurora-accent border-b-2 border-aurora-accent font-semibold'
              : 'text-gray-400 hover:text-gray-200'
          }`}
        >
          <TrendingUp className="w-4 h-4" />
          Cross-Tenant Usage
        </button>
      </div>

      {/* Tab: Plans */}
      {activeTab === 'plans' && (
        <div className="bg-gray-900 border border-gray-800 rounded-xl overflow-hidden shadow-lg">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm text-gray-300">
              <thead className="bg-gray-950/60 text-gray-400 text-xs uppercase border-b border-gray-800">
                <tr>
                  <th className="px-4 py-3">Plan Name</th>
                  <th className="px-4 py-3">Slug</th>
                  <th className="px-4 py-3">Monthly / Yearly</th>
                  <th className="px-4 py-3">Included Specs</th>
                  <th className="px-4 py-3">Max Allocations</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {plans.map((p) => (
                  <tr key={p.id} className="hover:bg-gray-800/40 transition">
                    <td className="px-4 py-3 font-semibold text-white">
                      {p.name}
                      <span className="block text-xs text-gray-500 font-normal">{p.description}</span>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-aurora-accent">{p.slug}</td>
                    <td className="px-4 py-3 text-xs text-gray-200">
                      {formatEUR(p.monthlyPriceMinor)} / {formatEUR(p.yearlyPriceMinor)}
                    </td>
                    <td className="px-4 py-3 text-xs text-gray-400">
                      {p.includedVcpu} vCPU, {p.includedMemoryMb / 1024} GB RAM, {p.includedStorageMb / 1024} GB Disk
                    </td>
                    <td className="px-4 py-3 text-xs text-gray-400">
                      Max {p.maxVcpu} vCPU, {p.maxMemoryMb / 1024} GB RAM, {p.maxInstances} Instances
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-block px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                          p.active
                            ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                            : 'bg-red-500/20 text-red-400 border border-red-500/30'
                        }`}
                      >
                        {p.active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      {p.active && (
                        <button
                          onClick={() => handleDeactivatePlan(p.id)}
                          className="text-xs text-red-400 hover:text-red-300 font-medium"
                        >
                          Deactivate
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Tab: Subscriptions */}
      {activeTab === 'subscriptions' && (
        <div className="bg-gray-900 border border-gray-800 rounded-xl overflow-hidden shadow-lg">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm text-gray-300">
              <thead className="bg-gray-950/60 text-gray-400 text-xs uppercase border-b border-gray-800">
                <tr>
                  <th className="px-4 py-3">Subscription ID</th>
                  <th className="px-4 py-3">Tenant / User ID</th>
                  <th className="px-4 py-3">Plan ID</th>
                  <th className="px-4 py-3">Cycle</th>
                  <th className="px-4 py-3">Current Period End</th>
                  <th className="px-4 py-3">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {subscriptions.map((s) => (
                  <tr key={s.id} className="hover:bg-gray-800/40 transition">
                    <td className="px-4 py-3 font-mono text-xs text-gray-300">{s.id}</td>
                    <td className="px-4 py-3 font-mono text-xs text-aurora-accent">{s.userId}</td>
                    <td className="px-4 py-3 font-mono text-xs text-gray-400">{s.planId}</td>
                    <td className="px-4 py-3 text-xs capitalize text-gray-200">{s.billingCycle}</td>
                    <td className="px-4 py-3 text-xs text-gray-400">
                      {new Date(s.currentPeriodEnd).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-block px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                          s.status === 'active'
                            ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                            : 'bg-yellow-500/20 text-yellow-400 border border-yellow-500/30'
                        }`}
                      >
                        {s.status}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Tab: Invoices */}
      {activeTab === 'invoices' && (
        <div className="bg-gray-900 border border-gray-800 rounded-xl overflow-hidden shadow-lg">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm text-gray-300">
              <thead className="bg-gray-950/60 text-gray-400 text-xs uppercase border-b border-gray-800">
                <tr>
                  <th className="px-4 py-3">Invoice Number</th>
                  <th className="px-4 py-3">Tenant ID</th>
                  <th className="px-4 py-3">Period</th>
                  <th className="px-4 py-3">Total</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {invoices.map((inv) => (
                  <tr key={inv.id} className="hover:bg-gray-800/40 transition">
                    <td className="px-4 py-3 font-mono text-xs font-medium text-white">{inv.invoiceNumber}</td>
                    <td className="px-4 py-3 font-mono text-xs text-aurora-accent">{inv.userId}</td>
                    <td className="px-4 py-3 text-xs text-gray-400">
                      {new Date(inv.periodStart).toLocaleDateString()} — {new Date(inv.periodEnd).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-xs font-semibold text-gray-100">{formatEUR(inv.totalMinor)}</td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-block px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                          inv.status === 'paid'
                            ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                            : inv.status === 'open'
                            ? 'bg-blue-500/20 text-blue-400 border border-blue-500/30'
                            : 'bg-gray-500/20 text-gray-400 border border-gray-500/30'
                        }`}
                      >
                        {inv.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      {inv.status !== 'void' && (
                        <button
                          onClick={() => handleVoidInvoice(inv.id)}
                          className="text-xs text-red-400 hover:text-red-300 font-medium flex items-center gap-1 ml-auto"
                        >
                          <Slash className="w-3.5 h-3.5" />
                          Void
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Tab: Cross-Tenant Usage */}
      {activeTab === 'usage' && (
        <div className="space-y-4">
          <div className="flex gap-3 max-w-md">
            <input
              type="text"
              placeholder="Filter by Tenant ID..."
              value={tenantUsageFilter}
              onChange={(e) => setTenantUsageFilter(e.target.value)}
              className="flex-1 bg-gray-950 border border-gray-800 rounded-lg px-3 py-2 text-xs text-white placeholder-gray-500 focus:outline-none focus:border-aurora-accent"
            />
            <button
              onClick={loadData}
              className="px-3 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded-lg flex items-center gap-1.5 transition"
            >
              <Search className="w-3.5 h-3.5" />
              Filter
            </button>
          </div>

          {usage && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
              {Object.entries(usage.metrics || {}).map(([mKey, mVal]) => (
                <div key={mKey} className="bg-gray-900 border border-gray-800 rounded-xl p-5 shadow-lg">
                  <span className="text-xs text-gray-400 capitalize block mb-1">{mKey.replace(/_/g, ' ')}</span>
                  <span className="text-2xl font-bold text-white">{mVal.totalQuantity.toLocaleString()}</span>
                  <span className="text-xs text-gray-500 ml-1.5">{mVal.unit}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Create Plan Modal */}
      {showCreatePlanModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-fadeIn">
          <form
            onSubmit={handleCreatePlan}
            className="bg-gray-900 border border-gray-800 rounded-xl max-w-xl w-full p-6 shadow-2xl space-y-4 max-h-[90vh] overflow-y-auto"
          >
            <div className="flex justify-between items-center border-b border-gray-800 pb-3">
              <h3 className="text-lg font-bold text-white flex items-center gap-2">
                <Plus className="w-5 h-5 text-aurora-accent" />
                Create Billing Plan
              </h3>
              <button
                type="button"
                onClick={() => setShowCreatePlanModal(false)}
                className="text-gray-400 hover:text-white transition"
              >
                <XCircle className="w-5 h-5" />
              </button>
            </div>

            <div className="grid grid-cols-2 gap-4 text-xs">
              <div className="col-span-2">
                <label className="text-gray-400 block mb-1">Plan Name</label>
                <input
                  type="text"
                  required
                  value={newPlan.name}
                  onChange={(e) => setNewPlan({ ...newPlan, name: e.target.value })}
                  placeholder="e.g. GPU AI Workstation"
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div>
                <label className="text-gray-400 block mb-1">Slug (unique)</label>
                <input
                  type="text"
                  required
                  value={newPlan.slug}
                  onChange={(e) => setNewPlan({ ...newPlan, slug: e.target.value })}
                  placeholder="e.g. gpu-workstation"
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div>
                <label className="text-gray-400 block mb-1">Monthly Price (EUR cents)</label>
                <input
                  type="number"
                  required
                  value={newPlan.monthlyPriceMinor}
                  onChange={(e) => setNewPlan({ ...newPlan, monthlyPriceMinor: parseInt(e.target.value) || 0 })}
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div className="col-span-2">
                <label className="text-gray-400 block mb-1">Description</label>
                <input
                  type="text"
                  value={newPlan.description}
                  onChange={(e) => setNewPlan({ ...newPlan, description: e.target.value })}
                  placeholder="Short description of this tier"
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div>
                <label className="text-gray-400 block mb-1">Included vCPUs</label>
                <input
                  type="number"
                  value={newPlan.includedVcpu}
                  onChange={(e) => setNewPlan({ ...newPlan, includedVcpu: parseInt(e.target.value) || 1 })}
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div>
                <label className="text-gray-400 block mb-1">Max vCPUs</label>
                <input
                  type="number"
                  value={newPlan.maxVcpu}
                  onChange={(e) => setNewPlan({ ...newPlan, maxVcpu: parseInt(e.target.value) || 4 })}
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div>
                <label className="text-gray-400 block mb-1">Included RAM (MB)</label>
                <input
                  type="number"
                  value={newPlan.includedMemoryMb}
                  onChange={(e) => setNewPlan({ ...newPlan, includedMemoryMb: parseInt(e.target.value) || 1024 })}
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>

              <div>
                <label className="text-gray-400 block mb-1">Included NVMe Disk (MB)</label>
                <input
                  type="number"
                  value={newPlan.includedStorageMb}
                  onChange={(e) => setNewPlan({ ...newPlan, includedStorageMb: parseInt(e.target.value) || 10240 })}
                  className="w-full bg-gray-950 border border-gray-800 rounded-lg p-2.5 text-white"
                />
              </div>
            </div>

            <div className="flex justify-end gap-3 pt-3 border-t border-gray-800">
              <button
                type="button"
                onClick={() => setShowCreatePlanModal(false)}
                className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded-lg transition"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-2 bg-aurora-accent hover:bg-aurora-accent-hover text-white text-xs font-semibold rounded-lg transition"
              >
                Create Plan
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
};
