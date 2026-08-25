import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  Terminal as TerminalIcon,
  Maximize2,
  Minimize2,
  RefreshCw,
  Trash2,
  ZoomIn,
  ZoomOut,
  ShieldCheck,
} from 'lucide-react';
import { api } from '../lib/api';

interface TerminalViewProps {
  instanceId: string;
  instanceName: string;
  onClose?: () => void;
}

export const TerminalView: React.FC<TerminalViewProps> = ({
  instanceId,
  instanceName,
}) => {
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');
  const [output, setOutput] = useState<string[]>([]);
  const [fontSize, setFontSize] = useState<number>(13);
  const [isFullscreen, setIsFullscreen] = useState<boolean>(false);
  const cols = 80;
  const rows = 24;

  const containerRef = useRef<HTMLDivElement>(null);
  const terminalBodyRef = useRef<HTMLDivElement>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const inputBufferRef = useRef<string>('');

  const appendOutput = useCallback((text: string) => {
    setOutput((prev) => {
      const lines = text.split('\n');
      const next = [...prev];
      for (let i = 0; i < lines.length; i++) {
        if (i === 0 && next.length > 0 && !prev[prev.length - 1].endsWith('\r')) {
          next[next.length - 1] += lines[i];
        } else {
          next.push(lines[i]);
        }
      }
      // Limit buffer to 1000 lines
      if (next.length > 1000) {
        return next.slice(next.length - 1000);
      }
      return next;
    });
  }, []);

  const sendResize = useCallback((c: number, r: number) => {
    if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
      // Send binary resize frame: [1][uint16 cols][uint16 rows]
      const buf = new ArrayBuffer(5);
      const view = new DataView(buf);
      view.setUint8(0, 1); // Resize type
      view.setUint16(1, c, false); // Big endian cols
      view.setUint16(3, r, false); // Big endian rows
      socketRef.current.send(buf);
    }
  }, []);

  const sendInput = useCallback((data: string) => {
    if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
      const encoder = new TextEncoder();
      const raw = encoder.encode(data);
      const frame = new Uint8Array(1 + raw.length);
      frame[0] = 2; // Data type
      frame.set(raw, 1);
      socketRef.current.send(frame.buffer);
    }
  }, []);

  const connect = useCallback(() => {
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }

    setStatus('connecting');
    setOutput([`[Connecting to Aurora Secure PTY Gateway for ${instanceName}...]`]);

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const token = api.getToken();
    const wsUrl = `${protocol}//${host}/api/v1/instances/${instanceId}/console/exec?token=${encodeURIComponent(
      token || ''
    )}&cols=${cols}&rows=${rows}`;

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    socketRef.current = ws;

    ws.onopen = () => {
      setStatus('connected');
      appendOutput(`\r\n\x1b[32m[Connected to Aurora PTY Console (80x24)]\x1b[0m\r\n`);
      sendResize(cols, rows);
    };

    ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        const view = new DataView(event.data);
        if (view.byteLength > 0) {
          const type = view.getUint8(0);
          if (type === 2) {
            // PTY Data
            const decoder = new TextDecoder('utf-8');
            const str = decoder.decode(new Uint8Array(event.data, 1));
            appendOutput(str);
          }
        }
      } else if (typeof event.data === 'string') {
        appendOutput(event.data);
      }
    };

    ws.onerror = () => {
      setStatus('error');
      appendOutput(`\r\n\x1b[31m[WebSocket Connection Error]\x1b[0m\r\n`);
    };

    ws.onclose = () => {
      setStatus('disconnected');
      appendOutput(`\r\n\x1b[33m[Session Disconnected]\x1b[0m\r\n`);
    };
  }, [instanceId, instanceName, cols, rows, appendOutput, sendResize]);

  useEffect(() => {
    connect();
    return () => {
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }
    };
  }, [connect]);

  // Auto scroll to bottom
  useEffect(() => {
    if (terminalBodyRef.current) {
      terminalBodyRef.current.scrollTop = terminalBodyRef.current.scrollHeight;
    }
  }, [output]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter') {
      sendInput(inputBufferRef.current + '\n');
      inputBufferRef.current = '';
    } else if (e.key === 'Backspace') {
      inputBufferRef.current = inputBufferRef.current.slice(0, -1);
      sendInput('\x7f');
    } else if (e.key === 'c' && e.ctrlKey) {
      e.preventDefault();
      sendInput('\x03'); // Ctrl+C
    } else if (e.key === 'd' && e.ctrlKey) {
      e.preventDefault();
      sendInput('\x04'); // Ctrl+D
    } else if (e.key.length === 1 && !e.ctrlKey && !e.altKey && !e.metaKey) {
      inputBufferRef.current += e.key;
      sendInput(e.key);
    }
  };

  const clearTerminal = () => {
    setOutput([]);
  };

  return (
    <div
      ref={containerRef}
      className={`flex flex-col bg-[#07090e] border border-[#1e2538] rounded-2xl overflow-hidden shadow-2xl ${
        isFullscreen ? 'fixed inset-0 z-50 rounded-none' : 'w-full h-[520px]'
      }`}
    >
      {/* Terminal Title Bar */}
      <div className="bg-[#0b0e17] border-b border-[#181f30] px-4 py-2.5 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5">
            <span
              className={`w-2.5 h-2.5 rounded-full ${
                status === 'connected'
                  ? 'bg-emerald-400 animate-pulse'
                  : status === 'connecting'
                  ? 'bg-amber-400 animate-spin'
                  : 'bg-rose-400'
              }`}
            />
            <span className="text-xs font-semibold text-white flex items-center gap-1.5">
              <TerminalIcon className="w-3.5 h-3.5 text-blue-400" />
              <span>{instanceName}</span>
            </span>
          </div>

          <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-[#121624] border border-[#1e2538] text-slate-400">
            {cols}x{rows}
          </span>
          <span className="hidden sm:flex items-center gap-1 text-[10px] text-emerald-400 font-mono">
            <ShieldCheck className="w-3 h-3" />
            <span>mTLS Encrypted</span>
          </span>
        </div>

        {/* Action Controls */}
        <div className="flex items-center gap-1.5">
          {/* Quick Ctrl+C and Ctrl+D */}
          <button
            onClick={() => sendInput('\x03')}
            className="px-2 py-1 rounded bg-[#141824] hover:bg-[#1e2538] text-[10px] font-mono text-slate-300 hover:text-white border border-[#232a3d] transition"
            title="Send SIGINT (Ctrl+C)"
          >
            ^C
          </button>
          <button
            onClick={() => sendInput('\x04')}
            className="px-2 py-1 rounded bg-[#141824] hover:bg-[#1e2538] text-[10px] font-mono text-slate-300 hover:text-white border border-[#232a3d] transition"
            title="Send EOF (Ctrl+D)"
          >
            ^D
          </button>

          {/* Font Controls */}
          <button
            onClick={() => setFontSize((f) => Math.max(10, f - 1))}
            className="p-1 rounded hover:bg-[#141824] text-slate-400 hover:text-white"
            title="Decrease Font Size"
          >
            <ZoomOut className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => setFontSize((f) => Math.min(20, f + 1))}
            className="p-1 rounded hover:bg-[#141824] text-slate-400 hover:text-white"
            title="Increase Font Size"
          >
            <ZoomIn className="w-3.5 h-3.5" />
          </button>

          {/* Clear */}
          <button
            onClick={clearTerminal}
            className="p-1 rounded hover:bg-[#141824] text-slate-400 hover:text-white"
            title="Clear Terminal Buffer"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>

          {/* Reconnect */}
          <button
            onClick={connect}
            className="p-1 rounded hover:bg-[#141824] text-slate-400 hover:text-white"
            title="Reconnect Console"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>

          {/* Fullscreen */}
          <button
            onClick={() => setIsFullscreen(!isFullscreen)}
            className="p-1 rounded hover:bg-[#141824] text-slate-400 hover:text-white"
            title={isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'}
          >
            {isFullscreen ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* Terminal Screen Body */}
      <div
        ref={terminalBodyRef}
        tabIndex={0}
        onKeyDown={handleKeyDown}
        style={{ fontSize: `${fontSize}px` }}
        className="flex-1 p-4 font-mono text-emerald-400 bg-[#07090e] overflow-y-auto outline-none focus:ring-1 focus:ring-blue-500/40 select-text leading-tight whitespace-pre-wrap"
      >
        {output.map((line, i) => (
          <div key={i}>{line}</div>
        ))}
      </div>
    </div>
  );
};
