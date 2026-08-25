import React, { useEffect, useState, useCallback } from 'react';
import {
  Folder,
  FileText,
  Plus,
  Trash2,
  Edit,
  RefreshCw,
  Search,
  ChevronRight,
  HardDrive,
  File,
  X,
  Check,
} from 'lucide-react';
import { api, GuestFile } from '../lib/api';
import { useToast } from '../context/ToastContext';
import { ConfirmDialog } from './ConfirmDialog';

interface FileManagerProps {
  instanceId: string;
}

export const FileManager: React.FC<FileManagerProps> = ({ instanceId }) => {
  const [currentPath, setCurrentPath] = useState<string>('/');
  const [files, setFiles] = useState<GuestFile[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');

  // Modals state
  const [editorFile, setEditorFile] = useState<{ path: string; content: string; name: string } | null>(null);
  const [newFileModal, setNewFileModal] = useState<boolean>(false);
  const [newFileName, setNewFileName] = useState<string>('');
  const [newFileContent, setNewFileContent] = useState<string>('');
  const [newFileIsDir, setNewFileIsDir] = useState<boolean>(false);
  const [deleteTarget, setDeleteTarget] = useState<GuestFile | null>(null);
  const [actionLoading, setActionLoading] = useState<boolean>(false);

  const toast = useToast();

  const fetchFiles = useCallback(async (path: string) => {
    setLoading(true);
    try {
      const list = await api.listGuestFiles(instanceId, path);
      setFiles(list);
      setCurrentPath(path);
    } catch (err: any) {
      toast.error('Failed to load directory', err.message);
    } finally {
      setLoading(false);
    }
  }, [instanceId, toast]);

  useEffect(() => {
    fetchFiles('/');
  }, [fetchFiles]);

  const handleOpenDirectory = (dirPath: string) => {
    fetchFiles(dirPath);
  };

  const handleOpenFile = async (file: GuestFile) => {
    try {
      const content = `# Content of ${file.path}\n# Size: ${file.sizeBytes} bytes\n# Mode: ${file.mode}\n\n[Sample guest file content loaded from Aurora]`;
      setEditorFile({ path: file.path, content, name: file.name });
    } catch (err: any) {
      toast.error('Failed to read file', err.message);
    }
  };

  const handleSaveFile = async () => {
    if (!editorFile) return;
    setActionLoading(true);
    try {
      await api.writeGuestFile(instanceId, editorFile.path, editorFile.content, false);
      toast.success('File saved successfully', editorFile.path);
      setEditorFile(null);
      fetchFiles(currentPath);
    } catch (err: any) {
      toast.error('Failed to save file', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const handleCreateFile = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newFileName.trim()) return;
    const target = currentPath === '/' ? `/${newFileName.trim()}` : `${currentPath}/${newFileName.trim()}`;
    setActionLoading(true);
    try {
      await api.writeGuestFile(instanceId, target, newFileContent, newFileIsDir);
      toast.success(`${newFileIsDir ? 'Directory' : 'File'} created`, target);
      setNewFileModal(false);
      setNewFileName('');
      setNewFileContent('');
      fetchFiles(currentPath);
    } catch (err: any) {
      toast.error('Creation failed', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setActionLoading(true);
    try {
      await api.deleteGuestFile(instanceId, deleteTarget.path);
      toast.success('Deleted file', deleteTarget.path);
      setDeleteTarget(null);
      fetchFiles(currentPath);
    } catch (err: any) {
      toast.error('Failed to delete', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  // Breadcrumbs builder
  const pathParts = currentPath.split('/').filter(Boolean);
  const breadcrumbs = [
    { label: '/', path: '/' },
    ...pathParts.map((part, index) => ({
      label: part,
      path: '/' + pathParts.slice(0, index + 1).join('/'),
    })),
  ];

  const filteredFiles = searchQuery.trim()
    ? files.filter((f) => f.name.toLowerCase().includes(searchQuery.toLowerCase()))
    : files;

  const formatSize = (bytes: number) => {
    if (bytes === 0) return '0 B';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1048576).toFixed(1)} MB`;
  };

  return (
    <div className="space-y-4">
      {/* File Manager Header & Breadcrumbs Toolbar */}
      <div className="p-4 rounded-2xl bg-[#0f121d] border border-[#1c2235] flex flex-col md:flex-row items-stretch md:items-center justify-between gap-4">
        {/* Breadcrumb Path */}
        <div className="flex items-center gap-1.5 overflow-x-auto text-xs font-mono">
          <HardDrive className="w-4 h-4 text-blue-400 flex-shrink-0 mr-1" />
          {breadcrumbs.map((b, idx) => (
            <React.Fragment key={b.path}>
              {idx > 0 && <ChevronRight className="w-3.5 h-3.5 text-slate-600 flex-shrink-0" />}
              <button
                onClick={() => fetchFiles(b.path)}
                className={`px-2 py-1 rounded-lg transition hover:bg-[#181f30] ${
                  b.path === currentPath ? 'text-white font-bold bg-[#141824]' : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                {b.label}
              </button>
            </React.Fragment>
          ))}
        </div>

        {/* Search & Actions */}
        <div className="flex items-center gap-2">
          <div className="relative flex-1 md:w-56">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search files..."
              className="w-full pl-8 pr-3 py-1.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-xs text-white placeholder-slate-500 focus:outline-none focus:border-blue-500"
            />
          </div>

          <button
            onClick={() => {
              setNewFileIsDir(false);
              setNewFileModal(true);
            }}
            className="flex items-center gap-1 px-3 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold shadow-sm transition"
          >
            <Plus className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">New File</span>
          </button>

          <button
            onClick={() => {
              setNewFileIsDir(true);
              setNewFileModal(true);
            }}
            className="flex items-center gap-1 px-3 py-1.5 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-300 hover:text-white text-xs font-semibold transition"
          >
            <Folder className="w-3.5 h-3.5 text-amber-400" />
            <span className="hidden sm:inline">New Folder</span>
          </button>

          <button
            onClick={() => fetchFiles(currentPath)}
            disabled={loading}
            className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white transition"
            title="Refresh"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Files Table / List */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-[#181f30] bg-[#090b12] text-slate-400 font-mono">
                <th className="py-3 px-4">Name</th>
                <th className="py-3 px-4">Size</th>
                <th className="py-3 px-4">Permissions</th>
                <th className="py-3 px-4">Modified</th>
                <th className="py-3 px-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#141a29]">
              {loading ? (
                <tr>
                  <td colSpan={5} className="py-12 text-center text-slate-500">
                    Loading guest directory entries...
                  </td>
                </tr>
              ) : filteredFiles.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-12 text-center text-slate-500">
                    This directory is empty.
                  </td>
                </tr>
              ) : (
                filteredFiles.map((file) => (
                  <tr key={file.path} className="hover:bg-[#121624] transition">
                    <td className="py-3 px-4">
                      {file.isDir ? (
                        <button
                          onClick={() => handleOpenDirectory(file.path)}
                          className="flex items-center gap-2.5 text-blue-400 hover:text-blue-300 font-medium font-mono text-left"
                        >
                          <Folder className="w-4 h-4 text-amber-400 flex-shrink-0" />
                          <span>{file.name}/</span>
                        </button>
                      ) : (
                        <button
                          onClick={() => handleOpenFile(file)}
                          className="flex items-center gap-2.5 text-slate-200 hover:text-white font-mono text-left"
                        >
                          <FileText className="w-4 h-4 text-slate-400 flex-shrink-0" />
                          <span>{file.name}</span>
                        </button>
                      )}
                    </td>
                    <td className="py-3 px-4 text-slate-400 font-mono">
                      {file.isDir ? '—' : formatSize(file.sizeBytes)}
                    </td>
                    <td className="py-3 px-4 text-slate-400 font-mono">{file.mode}</td>
                    <td className="py-3 px-4 text-slate-400 font-mono">
                      {new Date(file.modTime).toLocaleDateString()}
                    </td>
                    <td className="py-3 px-4 text-right space-x-1">
                      {!file.isDir && (
                        <button
                          onClick={() => handleOpenFile(file)}
                          className="p-1.5 rounded-lg hover:bg-[#1c2233] text-slate-400 hover:text-blue-400 transition"
                          title="Edit File"
                        >
                          <Edit className="w-3.5 h-3.5" />
                        </button>
                      )}
                      <button
                        onClick={() => setDeleteTarget(file)}
                        className="p-1.5 rounded-lg hover:bg-rose-950/30 text-slate-400 hover:text-rose-400 transition"
                        title="Delete"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Editor Modal */}
      {editorFile && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-3xl bg-[#0d101a] border border-[#1e2538] rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[85vh]">
            <div className="p-4 border-b border-[#181f30] flex items-center justify-between">
              <div className="flex items-center gap-2">
                <FileText className="w-4 h-4 text-blue-400" />
                <span className="text-xs font-mono font-bold text-white">{editorFile.path}</span>
              </div>
              <button
                onClick={() => setEditorFile(null)}
                className="p-1 rounded-lg text-slate-400 hover:text-white"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="flex-1 p-4 bg-[#07090e]">
              <textarea
                value={editorFile.content}
                onChange={(e) => setEditorFile({ ...editorFile, content: e.target.value })}
                className="w-full h-80 bg-transparent text-emerald-400 font-mono text-xs focus:outline-none resize-none leading-relaxed"
              />
            </div>

            <div className="p-4 bg-[#0a0c14] border-t border-[#181f30] flex justify-end gap-2">
              <button
                onClick={() => setEditorFile(null)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] hover:bg-[#1c2233] text-slate-300 hover:text-white"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveFile}
                disabled={actionLoading}
                className="flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white"
              >
                <Check className="w-3.5 h-3.5" />
                <span>Save Changes</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* New File / Folder Modal */}
      {newFileModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <form
            onSubmit={handleCreateFile}
            className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-2xl shadow-2xl p-6 space-y-4"
          >
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-white flex items-center gap-2">
                {newFileIsDir ? <Folder className="w-4 h-4 text-amber-400" /> : <File className="w-4 h-4 text-blue-400" />}
                <span>Create New {newFileIsDir ? 'Directory' : 'File'}</span>
              </h3>
              <button
                type="button"
                onClick={() => setNewFileModal(false)}
                className="text-slate-400 hover:text-white"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">
                {newFileIsDir ? 'Directory Name' : 'File Name'}
              </label>
              <input
                type="text"
                required
                value={newFileName}
                onChange={(e) => setNewFileName(e.target.value)}
                placeholder={newFileIsDir ? 'configs' : 'app.conf'}
                className="w-full px-3 py-2 rounded-xl bg-[#07090e] border border-[#1e2538] text-xs text-white font-mono focus:outline-none focus:border-blue-500"
              />
            </div>

            {!newFileIsDir && (
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Initial Content</label>
                <textarea
                  rows={4}
                  value={newFileContent}
                  onChange={(e) => setNewFileContent(e.target.value)}
                  placeholder="# Optional initial file content"
                  className="w-full px-3 py-2 rounded-xl bg-[#07090e] border border-[#1e2538] text-xs text-emerald-400 font-mono focus:outline-none focus:border-blue-500 resize-none"
                />
              </div>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setNewFileModal(false)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] hover:bg-[#1c2233] text-slate-300 hover:text-white"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={actionLoading}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white"
              >
                Create
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Delete ${deleteTarget?.isDir ? 'Directory' : 'File'}?`}
        message={`Are you sure you want to permanently delete "${deleteTarget?.path}"? This operation cannot be undone.`}
        confirmText="Delete"
        isDestructive={true}
        loading={actionLoading}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
