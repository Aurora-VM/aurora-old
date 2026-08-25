import React, { useEffect, useRef, useState } from 'react';
import { Monitor, Maximize2, Minimize2, ShieldCheck } from 'lucide-react';
import { api } from '../lib/api';

interface VNCViewProps {
  instanceId: string;
  instanceName: string;
}

export const VNCView: React.FC<VNCViewProps> = ({ instanceId, instanceName }) => {
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected'>('connecting');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    setStatus('connecting');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/instances/${instanceId}/console/vnc?token=${encodeURIComponent(
      api.getToken() || ''
    )}`;

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    socketRef.current = ws;

    ws.onopen = () => {
      setStatus('connected');
    };

    ws.onmessage = () => {
      // Frame rendering simulation
    };

    ws.onclose = () => {
      setStatus('disconnected');
    };

    return () => {
      ws.close();
    };
  }, [instanceId]);

  return (
    <div
      className={`flex flex-col bg-[#07090e] border border-[#1e2538] rounded-2xl overflow-hidden shadow-2xl ${
        isFullscreen ? 'fixed inset-0 z-50 rounded-none' : 'w-full h-[520px]'
      }`}
    >
      {/* VNC Toolbar */}
      <div className="bg-[#0b0e17] border-b border-[#181f30] px-4 py-2.5 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5">
            <span
              className={`w-2.5 h-2.5 rounded-full ${
                status === 'connected'
                  ? 'bg-emerald-400'
                  : status === 'connecting'
                  ? 'bg-amber-400 animate-spin'
                  : 'bg-rose-400'
              }`}
            />
            <span className="text-xs font-semibold text-white flex items-center gap-1.5">
              <Monitor className="w-3.5 h-3.5 text-blue-400" />
              <span>VNC Remote Display ({instanceName})</span>
            </span>
          </div>

          <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-[#121624] border border-[#1e2538] text-slate-400">
            1920x1080 32-bit
          </span>
          <span className="hidden sm:flex items-center gap-1 text-[10px] text-emerald-400 font-mono">
            <ShieldCheck className="w-3 h-3" />
            <span>TLS Tunnel</span>
          </span>
        </div>

        <div className="flex items-center gap-1.5">
          <button
            onClick={() => setIsFullscreen(!isFullscreen)}
            className="p-1 rounded hover:bg-[#141824] text-slate-400 hover:text-white"
          >
            {isFullscreen ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* Screen Canvas */}
      <div className="flex-1 flex items-center justify-center bg-black relative">
        <canvas ref={canvasRef} width={800} height={450} className="max-w-full max-h-full" />
        <div className="absolute inset-0 flex flex-col items-center justify-center text-slate-500 space-y-2">
          <Monitor className="w-12 h-12 text-slate-600 animate-pulse" />
          <span className="text-xs font-mono">Virtual Machine Display Stream Active</span>
        </div>
      </div>
    </div>
  );
};
