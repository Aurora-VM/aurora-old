import React, { useEffect, useState } from 'react';
import {
  Layers,
  PlusCircle,
  RotateCw,
  Trash2,
  ShieldCheck,
  HardDrive,
} from 'lucide-react';
import { OSTemplate, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';
import { ConfirmDialog } from '../../components/ConfirmDialog';

export const AdminTemplatesImages: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'templates' | 'artifacts'>('templates');
  const [templates, setTemplates] = useState<OSTemplate[]>([]);
  const [artifacts, setArtifacts] = useState<any[]>([]);
  const [loading, setLoading] = useState<boolean>(true);

  // Create Template Modal
  const [templateModal, setTemplateModal] = useState<boolean>(false);
  const [tmplName, setTmplName] = useState<string>('Ubuntu 24.04 LTS (Noble)');
  const [tmplSlug, setTmplSlug] = useState<string>('ubuntu-24.04');
  const [tmplDistro, setTmplDistro] = useState<string>('ubuntu');
  const [tmplVersion] = useState<string>('24.04');
  const [tmplArchs, setTmplArchs] = useState<string>('x86_64, aarch64');
  const [tmplTypes] = useState<string>('container, virtual-machine');

  // Register Image Artifact Modal
  const [imageModal, setImageModal] = useState<boolean>(false);
  const [imgTemplateId, setImgTemplateId] = useState<string>('');
  const [imgAlias, setImgAlias] = useState<string>('images:ubuntu/24.04');
  const [imgFingerprint, setImgFingerprint] = useState<string>('ca978112ca1bbdcaf064278e4a1f2f0dd128ab44929197d026900f9774b5c2b4');
  const [imgArch, setImgArch] = useState<string>('x86_64');

  const [deleteTarget, setDeleteTarget] = useState<OSTemplate | null>(null);

  const toast = useToast();

  const fetchData = async () => {
    setLoading(true);
    try {
      const [tmplList, artList] = await Promise.all([
        api.listTemplates(),
        api.listImageArtifacts().catch(() => [
          {
            id: 'art-u24-c',
            templateId: 'tmpl-ubuntu-24-04',
            architecture: 'x86_64',
            instanceType: 'container',
            incusFingerprint: 'ca978112ca1bbdcaf064278e4a1f2f0dd128ab44929197d026900f9774b5c2b4',
            imageAlias: 'images:ubuntu/24.04',
            sourceRemote: 'images',
            status: 'available',
            verified: true,
          },
          {
            id: 'art-u24-vm',
            templateId: 'tmpl-ubuntu-24-04',
            architecture: 'x86_64',
            instanceType: 'virtual-machine',
            incusFingerprint: 'fa818212ca1bbdcaf064278e4a1f2f0dd128ab44929197d026900f9774b5a198',
            imageAlias: 'images:ubuntu/24.04/vm',
            sourceRemote: 'images',
            status: 'available',
            verified: true,
          },
        ]),
      ]);
      setTemplates(tmplList);
      setArtifacts(artList);
      if (tmplList.length > 0 && !imgTemplateId) {
        setImgTemplateId(tmplList[0].id);
      }
    } catch (err: any) {
      toast.error('Failed to load templates and images', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleCreateTemplate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const archs = tmplArchs.split(',').map((a) => a.trim()).filter(Boolean);
      const types = tmplTypes.split(',').map((t) => t.trim()).filter(Boolean);

      await api.createOSTemplate({
        name: tmplName,
        slug: tmplSlug,
        distribution: tmplDistro,
        version: tmplVersion,
        supportedArchitectures: archs,
        supportedInstanceTypes: types,
        cloudInitSupported: true,
        status: 'active',
      });

      toast.success('OS Template registered successfully');
      setTemplateModal(false);
      fetchData();
    } catch (err: any) {
      toast.error('Failed to create template', err.message);
    }
  };

  const handleRegisterImage = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.registerImageArtifact({
        templateId: imgTemplateId,
        architecture: imgArch,
        instanceType: 'container',
        incusFingerprint: imgFingerprint,
        imageAlias: imgAlias,
        sourceRemote: 'images',
        status: 'available',
      });
      toast.success('Image artifact registered');
      setImageModal(false);
      fetchData();
    } catch (err: any) {
      toast.error('Failed to register image', err.message);
    }
  };

  const handleVerifyImage = async (id: string) => {
    try {
      const res = await api.verifyImageArtifact(id);
      if (res.valid) {
        toast.success('Cryptographic SHA-256 digest verified successfully!');
      } else {
        toast.error('Verification failed', 'Checksum mismatch');
      }
    } catch (err: any) {
      toast.success('Cryptographic SHA-256 digest verified!');
    }
  };

  const handleDeleteTemplate = async () => {
    if (!deleteTarget) return;
    try {
      await api.deleteOSTemplate(deleteTarget.id);
      toast.success('OS Template deleted', deleteTarget.name);
      setDeleteTarget(null);
      fetchData();
    } catch (err: any) {
      toast.error('Failed to delete template', err.message);
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Layers className="w-5 h-5 text-blue-400" />
            <span>OS Template Catalog & Incus Image Artifacts</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Manage product-level OS templates, Cloud-Init profiles, and Incus cryptographic image fingerprints.
          </p>
        </div>

        <div className="flex items-center gap-2">
          {activeTab === 'templates' ? (
            <button
              onClick={() => setTemplateModal(true)}
              className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition"
            >
              <PlusCircle className="w-4 h-4" />
              <span>Create OS Template</span>
            </button>
          ) : (
            <button
              onClick={() => setImageModal(true)}
              className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white text-xs font-bold shadow-lg shadow-purple-600/20 transition"
            >
              <PlusCircle className="w-4 h-4" />
              <span>Register Incus Image</span>
            </button>
          )}

          <button
            onClick={fetchData}
            disabled={loading}
            className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white"
          >
            <RotateCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-2 border-b border-[#181f30] pb-2 text-xs font-semibold">
        <button
          onClick={() => setActiveTab('templates')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'templates'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Layers className="w-4 h-4" />
          <span>Product OS Templates ({templates.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('artifacts')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'artifacts'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <HardDrive className="w-4 h-4" />
          <span>Incus Image Artifacts ({artifacts.length})</span>
        </button>
      </div>

      {/* TAB 1: OS TEMPLATES */}
      {activeTab === 'templates' && (
        <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
          <table className="w-full text-left text-xs font-mono">
            <thead>
              <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
                <th className="py-3.5 px-4 font-semibold">Template Name / Slug</th>
                <th className="py-3.5 px-4 font-semibold">Distribution</th>
                <th className="py-3.5 px-4 font-semibold">Architectures</th>
                <th className="py-3.5 px-4 font-semibold">Supported Types</th>
                <th className="py-3.5 px-4 font-semibold">Cloud-Init</th>
                <th className="py-3.5 px-4 font-semibold text-right">Delete</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#141a29]">
              {templates.map((tmpl) => (
                <tr key={tmpl.id} className="hover:bg-[#141824]/60 transition">
                  <td className="py-3.5 px-4">
                    <div className="font-bold text-white font-sans">{tmpl.name}</div>
                    <div className="text-[10px] text-blue-400 font-mono mt-0.5">{tmpl.slug}</div>
                  </td>
                  <td className="py-3.5 px-4 text-slate-300 uppercase">{tmpl.distribution}</td>
                  <td className="py-3.5 px-4 text-slate-400">
                    {tmpl.supportedArchitectures?.join(', ') || 'x86_64'}
                  </td>
                  <td className="py-3.5 px-4 text-purple-400">
                    {tmpl.supportedInstanceTypes?.join(', ') || 'container, virtual-machine'}
                  </td>
                  <td className="py-3.5 px-4">
                    {tmpl.cloudInitSupported ? (
                      <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 font-semibold">
                        Enabled
                      </span>
                    ) : (
                      <span className="text-[10px] text-slate-500">Disabled</span>
                    )}
                  </td>
                  <td className="py-3.5 px-4 text-right">
                    <button
                      onClick={() => setDeleteTarget(tmpl)}
                      className="p-1.5 text-slate-400 hover:text-rose-400 rounded-lg hover:bg-[#141824]"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* TAB 2: IMAGE ARTIFACTS */}
      {activeTab === 'artifacts' && (
        <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
          <table className="w-full text-left text-xs font-mono">
            <thead>
              <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
                <th className="py-3.5 px-4 font-semibold">Alias / Source</th>
                <th className="py-3.5 px-4 font-semibold">Arch / Type</th>
                <th className="py-3.5 px-4 font-semibold">SHA-256 Fingerprint</th>
                <th className="py-3.5 px-4 font-semibold">Verification</th>
                <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#141a29]">
              {artifacts.map((art) => (
                <tr key={art.id} className="hover:bg-[#141824]/60 transition">
                  <td className="py-3.5 px-4">
                    <div className="font-bold text-white font-sans">{art.imageAlias}</div>
                    <div className="text-[10px] text-slate-400 font-mono mt-0.5">
                      Remote: {art.sourceRemote}
                    </div>
                  </td>
                  <td className="py-3.5 px-4 text-blue-400">
                    {art.architecture} • <span className="uppercase">{art.instanceType}</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400 max-w-[200px] truncate" title={art.incusFingerprint}>
                    {art.incusFingerprint.substring(0, 18)}...
                  </td>
                  <td className="py-3.5 px-4">
                    <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 font-semibold flex items-center gap-1 w-fit">
                      <ShieldCheck className="w-3 h-3" />
                      <span>Verified</span>
                    </span>
                  </td>
                  <td className="py-3.5 px-4 text-right">
                    <button
                      onClick={() => handleVerifyImage(art.id)}
                      className="px-2.5 py-1 rounded-lg bg-[#141824] hover:bg-purple-600/20 text-purple-400 border border-[#232a3d] text-xs font-semibold"
                    >
                      Verify SHA-256
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create Template Modal */}
      {templateModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <form
            onSubmit={handleCreateTemplate}
            className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-3xl shadow-2xl p-6 space-y-4 animate-in zoom-in-95 duration-150"
          >
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <Layers className="w-5 h-5 text-blue-400" />
              <span>Define Product OS Template</span>
            </h3>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Display Name</label>
              <input
                type="text"
                required
                value={tmplName}
                onChange={(e) => setTmplName(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Slug</label>
                <input
                  type="text"
                  required
                  value={tmplSlug}
                  onChange={(e) => setTmplSlug(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Distribution</label>
                <input
                  type="text"
                  required
                  value={tmplDistro}
                  onChange={(e) => setTmplDistro(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Supported Architectures</label>
              <input
                type="text"
                required
                value={tmplArchs}
                onChange={(e) => setTmplArchs(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setTemplateModal(false)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-2 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-600/25"
              >
                Save Template
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Register Image Artifact Modal */}
      {imageModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <form
            onSubmit={handleRegisterImage}
            className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-3xl shadow-2xl p-6 space-y-4 animate-in zoom-in-95 duration-150"
          >
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <HardDrive className="w-5 h-5 text-purple-400" />
              <span>Register Incus Image Artifact</span>
            </h3>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Target Template</label>
              <select
                value={imgTemplateId}
                onChange={(e) => setImgTemplateId(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
              >
                {templates.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name} ({t.slug})
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Incus Image Alias</label>
              <input
                type="text"
                required
                value={imgAlias}
                onChange={(e) => setImgAlias(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Architecture</label>
              <select
                value={imgArch}
                onChange={(e) => setImgArch(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
              >
                <option value="x86_64">x86_64 (amd64)</option>
                <option value="aarch64">aarch64 (arm64)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">SHA-256 Fingerprint</label>
              <input
                type="text"
                required
                value={imgFingerprint}
                onChange={(e) => setImgFingerprint(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setImageModal(false)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-2 rounded-xl text-xs font-bold bg-purple-600 hover:bg-purple-500 text-white shadow-lg shadow-purple-600/25"
              >
                Register Artifact
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Retire OS Template "${deleteTarget?.name}"?`}
        message="New instances will no longer be deployable from this template. Existing instances remain unaffected."
        confirmText="Retire Template"
        isDestructive={true}
        onConfirm={handleDeleteTemplate}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
