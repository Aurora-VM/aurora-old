import React, { useState, useEffect } from 'react';
import {
  CreditCard,
  CheckCircle,
  AlertTriangle,
  FileText,
  Zap,
  TrendingUp,
  Cpu,
  HardDrive,
  Activity,
  Layers,
  ArrowRight,
  ShieldCheck,
  RefreshCw,
  XCircle,
} from 'lucide-react';
import { api, BillingPlan, Subscription, QuotaSet, UsageAggregate, Invoice } from '../lib/api';

export const Billing: React.FC = () => {
  const [plans, setPlans] = useState<BillingPlan[]>([]);
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [currentPlan, setCurrentPlan] = useState<BillingPlan | undefined>(undefined);
  const [quotas, setQuotas] = useState<QuotaSet>({});
  const [usage, setUsage] = useState<UsageAggregate | null>(null);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);
  const [selectedInvoice, setSelectedInvoice] = useState<Invoice | null>(null);

  const fetchBillingData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [plansData, subData, quotaData, usageData, invoicesData] = await Promise.all([
        api.listBillingPlans(),
        api.getSubscription(),
        api.getQuotas(),
        api.getUsage(),
        api.listInvoices(),
      ]);

      setPlans(plansData);
      setSubscription(subData.subscription);
      setCurrentPlan(subData.plan);
      setQuotas(quotaData.quotas || {});
      setUsage(usageData);
      setInvoices(invoicesData);
    } catch (err: any) {
      setError(err.message || 'Failed to load billing details');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchBillingData();
  }, []);

  const handleSubscribe = async (planId: string) => {
    try {
      setActionLoading(true);
      setError(null);
      if (subscription && subscription.status === 'active') {
        await api.changePlan(planId);
        setSuccessMsg('Plan updated successfully!');
      } else {
        await api.subscribe(planId, 'monthly');
        setSuccessMsg('Subscribed to plan successfully!');
      }
      await fetchBillingData();
    } catch (err: any) {
      setError(err.message || 'Failed to process subscription');
    } finally {
      setActionLoading(false);
    }
  };

  const handleCancelSubscription = async () => {
    if (!window.confirm('Are you sure you want to cancel your subscription? Your instances will remain active until the end of the billing period.')) {
      return;
    }
    try {
      setActionLoading(true);
      setError(null);
      await api.cancelSubscription();
      setSuccessMsg('Subscription canceled successfully.');
      await fetchBillingData();
    } catch (err: any) {
      setError(err.message || 'Failed to cancel subscription');
    } finally {
      setActionLoading(false);
    }
  };

  const formatEUR = (cents: number) => {
    return `€${(cents / 100).toFixed(2)}`;
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-aurora-accent"></div>
      </div>
    );
  }

  const vcpuQuota = quotas['vcpu_hours'] || { currentUsage: 0, limit: currentPlan?.maxVcpu || 4 };
  const ramQuota = quotas['ram_gb_hours'] || { currentUsage: 0, limit: (currentPlan?.maxMemoryMb || 4096) / 1024 };
  const storageQuota = quotas['storage_gb_months'] || { currentUsage: 0, limit: (currentPlan?.maxStorageMb || 40960) / 1024 };
  const instQuota = quotas['instance_count'] || { currentUsage: 0, limit: currentPlan?.maxInstances || 5 };

  return (
    <div className="space-y-8 animate-fadeIn">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-gray-800 pb-5">
        <div>
          <h1 className="text-2xl font-bold text-gray-100 flex items-center gap-3">
            <CreditCard className="w-7 h-7 text-aurora-accent" />
            Billing & Resource Plans
          </h1>
          <p className="text-gray-400 text-sm mt-1">
            Manage your cluster subscription, allocated resource quotas, live metered usage, and invoices.
          </p>
        </div>
        <button
          onClick={fetchBillingData}
          disabled={loading || actionLoading}
          className="flex items-center gap-2 px-3 py-1.5 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded-md border border-gray-700 transition"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          Refresh Billing
        </button>
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

      {/* Current Subscription Card */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 bg-gray-900 border border-gray-800 rounded-xl p-6 relative overflow-hidden shadow-lg">
          <div className="flex items-start justify-between">
            <div>
              <span className="text-xs uppercase tracking-wider text-aurora-accent font-semibold flex items-center gap-1.5 mb-2">
                <Zap className="w-4 h-4" />
                Active Subscription
              </span>
              <h2 className="text-2xl font-extrabold text-white">
                {currentPlan?.name || 'Starter Plan'}
              </h2>
              <p className="text-gray-400 text-sm mt-1 max-w-md">
                {currentPlan?.description || 'Standard cloud compute infrastructure with included quotas and overage protection.'}
              </p>
            </div>
            <div className="text-right">
              <div className="text-3xl font-black text-white">
                {formatEUR(currentPlan?.monthlyPriceMinor || 500)}
                <span className="text-xs font-medium text-gray-400"> / mo</span>
              </div>
              <span
                className={`inline-block mt-2 px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wider ${
                  subscription?.status === 'active'
                    ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                    : 'bg-yellow-500/20 text-yellow-400 border border-yellow-500/30'
                }`}
              >
                {subscription?.status || 'Default Active'}
              </span>
            </div>
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mt-6 pt-6 border-t border-gray-800/80">
            <div>
              <span className="text-xs text-gray-500 block">Included vCPUs</span>
              <span className="text-sm font-semibold text-gray-200">{currentPlan?.includedVcpu || 1} Cores</span>
            </div>
            <div>
              <span className="text-xs text-gray-500 block">Included RAM</span>
              <span className="text-sm font-semibold text-gray-200">{(currentPlan?.includedMemoryMb || 1024) / 1024} GB</span>
            </div>
            <div>
              <span className="text-xs text-gray-500 block">Included Storage</span>
              <span className="text-sm font-semibold text-gray-200">{(currentPlan?.includedStorageMb || 10240) / 1024} GB NVMe</span>
            </div>
            <div>
              <span className="text-xs text-gray-500 block">Bandwidth</span>
              <span className="text-sm font-semibold text-gray-200">{(currentPlan?.includedBandwidthGb || 1000).toLocaleString()} GB/mo</span>
            </div>
          </div>

          {subscription && subscription.status === 'active' && (
            <div className="mt-6 flex justify-end">
              <button
                onClick={handleCancelSubscription}
                disabled={actionLoading}
                className="text-xs text-red-400 hover:text-red-300 font-medium transition"
              >
                Cancel Subscription
              </button>
            </div>
          )}
        </div>

        {/* Quota Summary Card */}
        <div className="bg-gray-900 border border-gray-800 rounded-xl p-6 shadow-lg flex flex-col justify-between">
          <div>
            <h3 className="text-sm font-semibold text-gray-200 flex items-center gap-2 mb-4">
              <Activity className="w-4 h-4 text-aurora-accent" />
              Allocated Quotas & Limits
            </h3>
            <div className="space-y-4">
              {/* vCPU */}
              <div>
                <div className="flex justify-between text-xs mb-1">
                  <span className="text-gray-400 flex items-center gap-1.5">
                    <Cpu className="w-3.5 h-3.5 text-blue-400" /> vCPU Cores
                  </span>
                  <span className="text-gray-200 font-medium">
                    {vcpuQuota.currentUsage} / {vcpuQuota.limit} Cores
                  </span>
                </div>
                <div className="w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-blue-500 h-1.5 rounded-full transition-all duration-300"
                    style={{ width: `${Math.min(100, (vcpuQuota.currentUsage / (vcpuQuota.limit || 1)) * 100)}%` }}
                  />
                </div>
              </div>

              {/* Memory */}
              <div>
                <div className="flex justify-between text-xs mb-1">
                  <span className="text-gray-400 flex items-center gap-1.5">
                    <Zap className="w-3.5 h-3.5 text-purple-400" /> Memory (GB)
                  </span>
                  <span className="text-gray-200 font-medium">
                    {ramQuota.currentUsage} / {ramQuota.limit} GB
                  </span>
                </div>
                <div className="w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-purple-500 h-1.5 rounded-full transition-all duration-300"
                    style={{ width: `${Math.min(100, (ramQuota.currentUsage / (ramQuota.limit || 1)) * 100)}%` }}
                  />
                </div>
              </div>

              {/* Storage */}
              <div>
                <div className="flex justify-between text-xs mb-1">
                  <span className="text-gray-400 flex items-center gap-1.5">
                    <HardDrive className="w-3.5 h-3.5 text-amber-400" /> Storage (GB)
                  </span>
                  <span className="text-gray-200 font-medium">
                    {storageQuota.currentUsage} / {storageQuota.limit} GB
                  </span>
                </div>
                <div className="w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-amber-500 h-1.5 rounded-full transition-all duration-300"
                    style={{ width: `${Math.min(100, (storageQuota.currentUsage / (storageQuota.limit || 1)) * 100)}%` }}
                  />
                </div>
              </div>

              {/* Instances */}
              <div>
                <div className="flex justify-between text-xs mb-1">
                  <span className="text-gray-400 flex items-center gap-1.5">
                    <Layers className="w-3.5 h-3.5 text-emerald-400" /> Instances
                  </span>
                  <span className="text-gray-200 font-medium">
                    {instQuota.currentUsage} / {instQuota.limit}
                  </span>
                </div>
                <div className="w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-emerald-500 h-1.5 rounded-full transition-all duration-300"
                    style={{ width: `${Math.min(100, (instQuota.currentUsage / (instQuota.limit || 1)) * 100)}%` }}
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="mt-4 pt-3 border-t border-gray-800 text-[11px] text-gray-500">
            Resource allocations automatically adjust when creating, resizing, or deleting instances.
          </div>
        </div>
      </div>

      {/* Available Plans Grid */}
      <div>
        <h2 className="text-lg font-bold text-gray-100 flex items-center gap-2 mb-4">
          <Layers className="w-5 h-5 text-aurora-accent" />
          Choose or Upgrade Plan
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {plans.map((plan) => {
            const isCurrent = currentPlan?.id === plan.id;
            return (
              <div
                key={plan.id}
                className={`bg-gray-900 border rounded-xl p-6 flex flex-col justify-between transition-all ${
                  isCurrent
                    ? 'border-aurora-accent ring-1 ring-aurora-accent shadow-aurora-accent/10 shadow-lg'
                    : 'border-gray-800 hover:border-gray-700'
                }`}
              >
                <div>
                  <div className="flex justify-between items-start mb-2">
                    <h3 className="text-lg font-bold text-white">{plan.name}</h3>
                    {isCurrent && (
                      <span className="bg-aurora-accent/20 text-aurora-accent text-[10px] font-bold px-2 py-0.5 rounded uppercase tracking-wider border border-aurora-accent/40">
                        Current
                      </span>
                    )}
                  </div>
                  <p className="text-gray-400 text-xs min-h-[36px]">{plan.description}</p>

                  <div className="my-5">
                    <span className="text-3xl font-black text-white">{formatEUR(plan.monthlyPriceMinor)}</span>
                    <span className="text-gray-400 text-xs"> / month</span>
                  </div>

                  <ul className="space-y-2.5 text-xs text-gray-300 border-t border-gray-800 pt-4">
                    <li className="flex items-center gap-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400 flex-shrink-0" />
                      <span><strong>{plan.includedVcpu}</strong> Included vCPUs (Max {plan.maxVcpu})</span>
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400 flex-shrink-0" />
                      <span><strong>{plan.includedMemoryMb / 1024} GB</strong> RAM (Max {plan.maxMemoryMb / 1024} GB)</span>
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400 flex-shrink-0" />
                      <span><strong>{plan.includedStorageMb / 1024} GB</strong> NVMe Storage</span>
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400 flex-shrink-0" />
                      <span><strong>{plan.includedIpv4}</strong> Dedicated IPv4 Address</span>
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400 flex-shrink-0" />
                      <span><strong>{plan.includedSnapshots}</strong> Volume Snapshots</span>
                    </li>
                    <li className="flex items-center gap-2">
                      <ShieldCheck className="w-4 h-4 text-aurora-accent flex-shrink-0" />
                      <span>Automated DDoS & Firewall Isolation</span>
                    </li>
                  </ul>
                </div>

                <div className="mt-6 pt-4 border-t border-gray-800">
                  <button
                    onClick={() => handleSubscribe(plan.id)}
                    disabled={isCurrent || actionLoading}
                    className={`w-full py-2 px-4 rounded-lg text-xs font-semibold transition flex items-center justify-center gap-2 ${
                      isCurrent
                        ? 'bg-gray-800 text-gray-500 cursor-not-allowed'
                        : 'bg-aurora-accent hover:bg-aurora-accent-hover text-white shadow-md'
                    }`}
                  >
                    {isCurrent ? 'Current Plan' : 'Select Plan'}
                    {!isCurrent && <ArrowRight className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Metered Usage Telemetry */}
      {usage && (
        <div className="bg-gray-900 border border-gray-800 rounded-xl p-6 shadow-lg">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-bold text-gray-100 flex items-center gap-2">
              <TrendingUp className="w-5 h-5 text-aurora-accent" />
              Current Billing Cycle Metered Usage
            </h2>
            <span className="text-xs text-gray-400">
              Period: {new Date(usage.periodStart).toLocaleDateString()} — {new Date(usage.periodEnd).toLocaleDateString()}
            </span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {Object.entries(usage.metrics || {}).map(([metricKey, metricVal]) => (
              <div key={metricKey} className="bg-gray-950 border border-gray-800/80 rounded-lg p-4">
                <span className="text-xs text-gray-400 capitalize block mb-1">
                  {metricKey.replace(/_/g, ' ')}
                </span>
                <span className="text-xl font-bold text-white">
                  {metricVal.totalQuantity.toLocaleString()}
                </span>
                <span className="text-xs text-gray-500 ml-1.5">{metricVal.unit}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Invoices History Table */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl p-6 shadow-lg">
        <h2 className="text-lg font-bold text-gray-100 flex items-center gap-2 mb-4">
          <FileText className="w-5 h-5 text-aurora-accent" />
          Invoices & Billing History
        </h2>

        {invoices.length === 0 ? (
          <div className="text-center py-8 text-gray-500 text-sm">
            No invoices generated yet for this account.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm text-gray-300">
              <thead className="bg-gray-950/60 text-gray-400 text-xs uppercase border-b border-gray-800">
                <tr>
                  <th className="px-4 py-3">Invoice Number</th>
                  <th className="px-4 py-3">Billing Period</th>
                  <th className="px-4 py-3">Amount</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {invoices.map((inv) => (
                  <tr key={inv.id} className="hover:bg-gray-800/40 transition">
                    <td className="px-4 py-3 font-mono font-medium text-white">
                      {inv.invoiceNumber}
                    </td>
                    <td className="px-4 py-3 text-xs text-gray-400">
                      {new Date(inv.periodStart).toLocaleDateString()} — {new Date(inv.periodEnd).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 font-semibold text-gray-100">
                      {formatEUR(inv.totalMinor)}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-block px-2 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wider ${
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
                      <button
                        onClick={() => setSelectedInvoice(inv)}
                        className="text-xs text-aurora-accent hover:underline font-medium"
                      >
                        View Details
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Invoice Details Modal */}
      {selectedInvoice && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-fadeIn">
          <div className="bg-gray-900 border border-gray-800 rounded-xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex justify-between items-start border-b border-gray-800 pb-3">
              <div>
                <h3 className="text-lg font-bold text-white flex items-center gap-2">
                  <FileText className="w-5 h-5 text-aurora-accent" />
                  Invoice {selectedInvoice.invoiceNumber}
                </h3>
                <span className="text-xs text-gray-400">
                  Issued on {new Date(selectedInvoice.createdAt).toLocaleDateString()}
                </span>
              </div>
              <button
                onClick={() => setSelectedInvoice(null)}
                className="text-gray-400 hover:text-white transition"
              >
                <XCircle className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-2 max-h-60 overflow-y-auto">
              {selectedInvoice.lines?.map((line) => (
                <div key={line.id} className="flex justify-between text-xs py-1.5 border-b border-gray-800/40">
                  <div className="text-gray-300">
                    <p className="font-medium">{line.description}</p>
                    <span className="text-gray-500">Qty: {line.quantity}</span>
                  </div>
                  <div className="text-right text-gray-200 font-medium">
                    {formatEUR(line.amountMinor)}
                  </div>
                </div>
              ))}
            </div>

            <div className="border-t border-gray-800 pt-3 space-y-1.5 text-xs">
              <div className="flex justify-between text-gray-400">
                <span>Subtotal</span>
                <span>{formatEUR(selectedInvoice.subtotalMinor)}</span>
              </div>
              <div className="flex justify-between text-gray-400">
                <span>VAT / Tax (19%)</span>
                <span>{formatEUR(selectedInvoice.taxMinor)}</span>
              </div>
              <div className="flex justify-between text-sm font-bold text-white pt-2 border-t border-gray-800">
                <span>Total</span>
                <span>{formatEUR(selectedInvoice.totalMinor)}</span>
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <button
                onClick={() => setSelectedInvoice(null)}
                className="px-4 py-1.5 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded-lg transition"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
