import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, type Preview, type Printer, type Template } from '../api';
import { Field, Spinner, inputCls, useToast } from '../ui';

const templateIcons: Record<string, string> = {
  inventory: 'M4 5h9v4H4zM4 11h6v1.5H4zM4 14h6v1.5H4zM15 5h5v5h-5zM15 12h5v1.5h-5z',
  'large-print': 'M3 6h18v3H3zM5 11h14v2.5H5zM7 16h10v2H7z',
  'small-print': 'M4 7h16v1.5H4zM4 11h12v1.5H4zM4 15h14v1.5H4z',
  packing: 'M3 5h18v4H3zM4 12h1.5v1.5H4zM7 12h13v1.5H7zM4 16h1.5v1.5H4zM7 16h13v1.5H7z',
};

function TemplateCard({
  t,
  active,
  onClick,
  onDelete,
}: {
  t: Template;
  active: boolean;
  onClick: () => void;
  onDelete?: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  return (
    <button
      onClick={onClick}
      className={`group w-full rounded-xl border p-4 text-left transition-all ${
        active
          ? 'border-accent/60 bg-panel-2 shadow-[0_0_0_1px_var(--color-accent)]/20'
          : 'border-edge bg-panel hover:border-edge-2 hover:bg-panel-2'
      }`}
    >
      <div className="mb-2 flex items-center justify-between">
        <svg viewBox="0 0 24 24" className={`size-6 ${active ? 'fill-accent' : 'fill-fg-dim group-hover:fill-fg'}`}>
          <path d={templateIcons[t.id] ?? 'M4 4h16v16H4z'} />
        </svg>
        {!t.builtin && (
          <span className="flex items-center gap-1.5">
            <span className="rounded-full border border-edge-2 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-fg-dim">
              custom
            </span>
            {onDelete && (
              <span
                role="button"
                tabIndex={0}
                onClick={(e) => {
                  e.stopPropagation();
                  if (confirming) {
                    onDelete();
                  } else {
                    setConfirming(true);
                    setTimeout(() => setConfirming(false), 3000);
                  }
                }}
                className={`rounded-full border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider transition-colors ${
                  confirming
                    ? 'border-accent/60 bg-accent/10 text-accent-hi'
                    : 'border-edge-2 text-fg-dim opacity-0 group-hover:opacity-100 hover:text-accent-hi'
                }`}
              >
                {confirming ? 'sure?' : '✕'}
              </span>
            )}
          </span>
        )}
      </div>
      <div className="text-[15px] font-semibold">{t.name}</div>
      <div className="mt-0.5 text-[12.5px] leading-snug text-fg-dim">{t.description}</div>
    </button>
  );
}

export function PrintPage() {
  const toast = useToast();
  const [templates, setTemplates] = useState<Template[]>([]);
  const [printers, setPrinters] = useState<Printer[]>([]);
  const [selected, setSelected] = useState<string>('');
  const [vars, setVars] = useState<Record<string, string>>({});
  const [printerId, setPrinterId] = useState<number | undefined>();
  const [copies, setCopies] = useState(1);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState('');
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [printing, setPrinting] = useState(false);
  const [showZpl, setShowZpl] = useState(false);
  const [actualSize, setActualSize] = useState(false);
  const debounceRef = useRef<number>(0);

  const template = useMemo(() => templates.find((t) => t.id === selected), [templates, selected]);

  useEffect(() => {
    api.templates().then((list) => {
      setTemplates(list);
      if (list.length && !selected) setSelected(list[0].id);
    });
    api.printers().then((list) => {
      setPrinters(list);
      const def = list.find((p) => p.isDefault) ?? list[0];
      if (def) setPrinterId(def.id);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Live preview: debounce keystrokes, only render once required fields exist.
  const refreshPreview = useCallback(() => {
    if (!template) return;
    const missing = template.fields.filter((f) => f.required && !vars[f.key]?.trim());
    if (missing.length) {
      setPreview(null);
      setPreviewErr('');
      return;
    }
    setLoadingPreview(true);
    api
      .preview({ templateId: template.id, vars, printerId })
      .then((p) => {
        setPreview(p);
        setPreviewErr('');
      })
      .catch((e: Error) => setPreviewErr(e.message))
      .finally(() => setLoadingPreview(false));
  }, [template, vars, printerId]);

  useEffect(() => {
    window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(refreshPreview, 350);
    return () => window.clearTimeout(debounceRef.current);
  }, [refreshPreview]);

  const canPrint = !!preview && !loadingPreview && !printing;

  const print = async () => {
    if (!template || !canPrint) return;
    setPrinting(true);
    try {
      const job = await api.createJob({
        templateId: template.id,
        vars,
        printerId,
        copies,
        source: 'web',
        idempotencyKey: crypto.randomUUID(),
      });
      toast('ok', `Job ${job.id} queued on ${job.printerName}`);
    } catch (e) {
      toast('err', (e as Error).message);
    } finally {
      setPrinting(false);
    }
  };

  const setVar = (key: string, value: string) => setVars((v) => ({ ...v, [key]: value }));

  const dpi = (preview?.dpmm || 8) * 25.4;
  const widthIn = preview ? preview.widthDots / dpi : 2.4;
  const lengthIn = preview ? preview.lengthDots / dpi : 0;
  // Actual size: printer dots-per-inch rendered at CSS 96px-per-inch.
  const previewCssWidth = actualSize ? `${(preview?.widthDots ?? 487) / dpi}in` : '100%';

  return (
    <div className="grid gap-6 lg:grid-cols-[260px_minmax(0,1fr)_minmax(0,1.2fr)]">
      {/* Template picker */}
      <section className="space-y-2.5">
        <h2 className="px-1 font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-fg-dim">
          Template
        </h2>
        {templates.map((t) => (
          <TemplateCard
            key={t.id}
            t={t}
            active={t.id === selected}
            onClick={() => {
              setSelected(t.id);
              setVars({});
              setPreview(null);
            }}
            onDelete={
              t.builtin
                ? undefined
                : async () => {
                    try {
                      await api.deleteTemplate(t.id);
                      const list = await api.templates();
                      setTemplates(list);
                      if (selected === t.id && list.length) {
                        setSelected(list[0].id);
                        setVars({});
                        setPreview(null);
                      }
                      toast('ok', `Deleted template "${t.name}"`);
                    } catch (e) {
                      toast('err', (e as Error).message);
                    }
                  }
            }
          />
        ))}
        {templates.length === 0 && <Spinner />}
      </section>

      {/* Form */}
      <section>
        <h2 className="mb-2.5 px-1 font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-fg-dim">
          Content
        </h2>
        <div className="space-y-4 rounded-xl border border-edge bg-panel p-5">
          {template?.fields.map((f) => (
            <Field key={f.key} label={f.label} hint={f.required ? undefined : 'optional'}>
              {f.type === 'textarea' ? (
                <textarea
                  className={`${inputCls} min-h-24 resize-y`}
                  placeholder={f.placeholder}
                  value={vars[f.key] ?? ''}
                  onChange={(e) => setVar(f.key, e.target.value)}
                />
              ) : (
                <input
                  type={f.type === 'url' ? 'url' : 'text'}
                  className={inputCls}
                  placeholder={f.placeholder}
                  value={vars[f.key] ?? ''}
                  onChange={(e) => setVar(f.key, e.target.value)}
                />
              )}
            </Field>
          ))}
          {template && (
            <div className="grid grid-cols-2 gap-4 border-t border-edge pt-4">
              <Field label="Printer">
                <select
                  className={inputCls}
                  value={printerId ?? ''}
                  onChange={(e) => setPrinterId(Number(e.target.value))}
                >
                  {printers.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Copies">
                <div className="flex items-center rounded-lg border border-edge bg-panel-2">
                  <button
                    className="px-3.5 py-2 text-lg text-fg-dim transition-colors hover:text-fg disabled:opacity-30"
                    disabled={copies <= 1}
                    onClick={() => setCopies((c) => Math.max(1, c - 1))}
                  >
                    −
                  </button>
                  <span className="flex-1 text-center font-mono text-[15px]">{copies}</span>
                  <button
                    className="px-3.5 py-2 text-lg text-fg-dim transition-colors hover:text-fg"
                    onClick={() => setCopies((c) => Math.min(99, c + 1))}
                  >
                    +
                  </button>
                </div>
              </Field>
            </div>
          )}
          <button
            onClick={print}
            disabled={!canPrint}
            className="w-full rounded-lg bg-accent py-3 text-[15px] font-semibold text-white shadow-[0_4px_0_0_#b8330f] transition-all hover:bg-accent-hi active:translate-y-[2px] active:shadow-[0_2px_0_0_#b8330f] disabled:cursor-not-allowed disabled:opacity-40 disabled:shadow-none"
          >
            {printing ? 'Printing…' : `Print label${copies > 1 ? ` ×${copies}` : ''}`}
          </button>
        </div>
      </section>

      {/* Preview */}
      <section>
        <div className="mb-2.5 flex items-center justify-between px-1">
          <h2 className="font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-fg-dim">
            Preview
          </h2>
          <div className="flex items-center gap-3 font-mono text-[11px] text-fg-dim">
            {preview && (
              <span>
                {widthIn.toFixed(1)}″ × {lengthIn.toFixed(2)}″
              </span>
            )}
            <button
              onClick={() => setActualSize((a) => !a)}
              className={`rounded px-2 py-0.5 transition-colors ${actualSize ? 'bg-panel-2 text-fg' : 'hover:text-fg'}`}
            >
              1:1
            </button>
            <button
              onClick={() => setShowZpl((z) => !z)}
              className={`rounded px-2 py-0.5 transition-colors ${showZpl ? 'bg-panel-2 text-fg' : 'hover:text-fg'}`}
            >
              ZPL
            </button>
          </div>
        </div>
        <div className="dotgrid flex min-h-72 items-start justify-center rounded-xl border border-edge bg-ink p-6">
          {preview ? (
            <div className="paper overflow-hidden" style={{ width: previewCssWidth, maxWidth: '100%' }}>
              <div className="paper-perf" />
              <img
                src={`data:image/png;base64,${preview.png}`}
                alt="Label preview"
                className="block w-full"
                style={{ imageRendering: actualSize ? 'auto' : 'crisp-edges' }}
              />
              <div className="paper-perf" />
            </div>
          ) : (
            <div className="flex h-60 flex-col items-center justify-center gap-3 text-fg-dim">
              {loadingPreview ? (
                <Spinner />
              ) : previewErr ? (
                <span className="max-w-64 text-center font-mono text-[13px] text-accent-hi">{previewErr}</span>
              ) : (
                <>
                  <svg viewBox="0 0 24 24" className="size-10 fill-edge-2">
                    <path d="M6 3h12v6H6zM4 10h16v7h-3v4H7v-4H4zm5 5v4h6v-4z" />
                  </svg>
                  <span className="text-[13px]">Fill in the fields — the label appears here live</span>
                </>
              )}
            </div>
          )}
        </div>
        {showZpl && preview && (
          <pre className="mt-3 max-h-56 overflow-auto rounded-xl border border-edge bg-panel p-4 font-mono text-[12px] leading-relaxed text-fg-dim">
            {preview.zpl}
          </pre>
        )}
      </section>
    </div>
  );
}
