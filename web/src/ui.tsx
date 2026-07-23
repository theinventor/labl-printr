import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';

// ---- Toasts

type Toast = { id: number; kind: 'ok' | 'err'; text: string };
const ToastCtx = createContext<(kind: Toast['kind'], text: string) => void>(() => {});
let toastSeq = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((kind: Toast['kind'], text: string) => {
    const id = toastSeq++;
    setToasts((t) => [...t, { id, kind, text }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3600);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed bottom-5 left-1/2 z-50 flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`rounded-lg border px-4 py-2 font-mono text-sm shadow-xl backdrop-blur ${
              t.kind === 'ok'
                ? 'border-ok/40 bg-panel/90 text-ok'
                : 'border-accent/40 bg-panel/90 text-accent-hi'
            }`}
          >
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export const useToast = () => useContext(ToastCtx);

// ---- Bits

export function Spinner({ className = '' }: { className?: string }) {
  return (
    <span
      className={`inline-block size-4 animate-spin rounded-full border-2 border-edge-2 border-t-fg ${className}`}
    />
  );
}

export function StateChip({ state }: { state: string }) {
  const styles: Record<string, string> = {
    done: 'bg-ok/10 text-ok border-ok/30',
    failed: 'bg-accent/10 text-accent-hi border-accent/30',
    printing: 'bg-warn/10 text-warn border-warn/30',
    queued: 'bg-edge/40 text-fg-dim border-edge-2',
    canceled: 'bg-edge/40 text-fg-dim border-edge-2',
  };
  return (
    <span
      className={`inline-block rounded-full border px-2 py-0.5 font-mono text-[11px] uppercase tracking-wider ${styles[state] ?? styles.queued}`}
    >
      {state}
    </span>
  );
}

export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="block">
      <span className="mb-1.5 flex items-baseline justify-between text-[13px] font-medium text-fg-dim">
        {label}
        {hint && <span className="font-mono text-[11px] text-fg-dim/60">{hint}</span>}
      </span>
      {children}
    </label>
  );
}

export const inputCls =
  'w-full rounded-lg border border-edge bg-panel-2 px-3 py-2 text-[15px] text-fg placeholder:text-fg-dim/40 outline-none transition-colors focus:border-accent/60 focus:ring-2 focus:ring-accent/15';
