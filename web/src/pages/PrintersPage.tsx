import { useEffect, useState } from 'react';
import {
  api,
  type Discovered,
  type MediaOption,
  type Printer,
  type PrinterStatus,
  type TrayPrint,
} from '../api';
import { Field, Spinner, inputCls, useToast } from '../ui';

function StatusDot({ st }: { st?: PrinterStatus }) {
  if (!st) return <span className="size-2.5 rounded-full bg-edge-2" />;
  const color = st.ready ? 'bg-ok' : st.reachable ? 'bg-warn' : 'bg-accent';
  return <span className={`size-2.5 rounded-full ${color} shadow-[0_0_8px] shadow-current`} />;
}

function statusText(st?: PrinterStatus): string {
  if (!st) return 'checking…';
  if (st.ready) return 'ready';
  if (!st.reachable) return st.detail || 'unreachable';
  return st.detail || 'not ready';
}

export function PrintersPage() {
  const toast = useToast();
  const [printers, setPrinters] = useState<Printer[] | null>(null);
  const [statuses, setStatuses] = useState<Record<number, PrinterStatus>>({});
  const [tray, setTray] = useState<TrayPrint[]>([]);
  const [discovering, setDiscovering] = useState(false);
  const [found, setFound] = useState<Discovered[] | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [addForm, setAddForm] = useState({ name: '', host: '', dpi: '203', type: 'network' });
  const [mediaByKind, setMediaByKind] = useState<Record<string, MediaOption[]>>({});

  const load = async () => {
    const list = await api.printers();
    setPrinters(list);
    list.forEach((p) =>
      api.printerStatus(p.id).then((st) => setStatuses((s) => ({ ...s, [p.id]: st }))),
    );
  };

  const changeMedia = async (id: number, mediaId: string) => {
    try {
      await api.setPrinterMedia(id, mediaId);
      toast('ok', 'Loaded media updated');
      load();
    } catch (e) {
      toast('err', (e as Error).message);
    }
  };

  useEffect(() => {
    load();
    api.media().then((all) => {
      const grouped: Record<string, MediaOption[]> = {};
      for (const m of all) (grouped[m.kind] ??= []).push(m);
      setMediaByKind(grouped);
    });
    api.tray(24).then(setTray);
    const t = setInterval(() => api.tray(24).then(setTray), 5000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const discover = async () => {
    setDiscovering(true);
    setFound(null);
    try {
      setFound(await api.discover());
    } catch (e) {
      toast('err', (e as Error).message);
    } finally {
      setDiscovering(false);
    }
  };

  const addPrinter = async (name: string, host: string, dpi: string, type: string) => {
    try {
      // Brother QL geometry is fixed (300dpi, 696-dot 62mm); the server fills
      // in the dot math. Zebra needs the dpi choice.
      // Brother and DYMO geometry come from the loaded media, filled server-side.
      const body =
        type === 'brother' || type === 'dymo'
          ? { name, host, kind: type as 'brother' | 'dymo' }
          : {
              name,
              host,
              kind: 'network' as const,
              port: 9100,
              dpmm: dpi === '300' ? 12 : 8,
              widthDots: dpi === '300' ? 720 : 487,
              leftShift: dpi === '300' ? 280 : 172,
            };
      await api.createPrinter(body);
      toast('ok', `Added ${name}`);
      setShowAdd(false);
      setFound(null);
      load();
    } catch (e) {
      toast('err', (e as Error).message);
    }
  };

  const setDefault = async (id: number) => {
    await api.setDefault(id);
    load();
  };

  const [confirmRemove, setConfirmRemove] = useState<number | null>(null);
  const remove = async (p: Printer) => {
    if (confirmRemove !== p.id) {
      setConfirmRemove(p.id);
      setTimeout(() => setConfirmRemove((c) => (c === p.id ? null : c)), 3000);
      return;
    }
    setConfirmRemove(null);
    try {
      await api.deletePrinter(p.id);
      load();
    } catch (e) {
      toast('err', (e as Error).message);
    }
  };

  if (!printers) return <Spinner />;

  return (
    <div className="space-y-8">
      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-fg-dim">
            Printers
          </h2>
          <div className="flex gap-2">
            <button
              onClick={discover}
              disabled={discovering}
              className="rounded-lg border border-edge px-3.5 py-2 text-[13px] font-medium text-fg-dim transition-colors hover:border-edge-2 hover:text-fg disabled:opacity-50"
            >
              {discovering ? 'Scanning…' : 'Scan network'}
            </button>
            <button
              onClick={() => setShowAdd((s) => !s)}
              className="rounded-lg border border-edge px-3.5 py-2 text-[13px] font-medium text-fg-dim transition-colors hover:border-edge-2 hover:text-fg"
            >
              Add manually
            </button>
          </div>
        </div>

        {found && (
          <div className="mb-4 rounded-xl border border-edge bg-panel p-4">
            {found.length === 0 ? (
              <p className="text-[13.5px] text-fg-dim">
                No printers answered the discovery broadcast. The ZD421 needs its network module
                online and on this subnet — check it's powered and connected, then scan again.
              </p>
            ) : (
              <div className="space-y-2">
                {found.map((d) => (
                  <div key={d.ip} className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <span className="font-mono text-[14px]">{d.ip}</span>
                      <span className="ml-3 truncate font-mono text-[12px] text-fg-dim">
                        {d.info.slice(0, 4).join(' · ')}
                      </span>
                    </div>
                    <button
                      onClick={() => addPrinter(d.info[0] || `Zebra ${d.ip}`, d.ip, '203', 'network')}
                      className="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-semibold text-white hover:bg-accent-hi"
                    >
                      Add
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {showAdd && (
          <form
            className="mb-4 grid gap-4 rounded-xl border border-edge bg-panel p-4 sm:grid-cols-[1fr_1fr_auto_auto_auto]"
            onSubmit={(e) => {
              e.preventDefault();
              addPrinter(addForm.name, addForm.host, addForm.dpi, addForm.type);
            }}
          >
            <Field label="Name">
              <input
                className={inputCls}
                placeholder={addForm.type === 'brother' ? 'QL-820NWB — shop' : 'ZD421 — shop'}
                value={addForm.name}
                onChange={(e) => setAddForm((f) => ({ ...f, name: e.target.value }))}
                required
              />
            </Field>
            <Field label="Host / IP">
              <input
                className={inputCls}
                placeholder="10.0.0.50 or printer.local"
                value={addForm.host}
                onChange={(e) => setAddForm((f) => ({ ...f, host: e.target.value }))}
                required
              />
            </Field>
            <Field label="Type">
              <select
                className={inputCls}
                value={addForm.type}
                onChange={(e) => setAddForm((f) => ({ ...f, type: e.target.value }))}
              >
                <option value="network">Zebra (ZPL)</option>
                <option value="brother">Brother QL</option>
                <option value="dymo">DYMO (CUPS)</option>
              </select>
            </Field>
            <Field label="Resolution">
              <select
                className={`${inputCls} ${addForm.type !== 'network' ? 'opacity-40' : ''}`}
                value={addForm.type !== 'network' ? '300' : addForm.dpi}
                disabled={addForm.type !== 'network'}
                onChange={(e) => setAddForm((f) => ({ ...f, dpi: e.target.value }))}
              >
                <option value="203">203 dpi</option>
                <option value="300">300 dpi</option>
              </select>
            </Field>
            <button className="self-end rounded-lg bg-accent px-4 py-2 text-[14px] font-semibold text-white hover:bg-accent-hi">
              Add
            </button>
          </form>
        )}

        <div className="grid gap-3 sm:grid-cols-2">
          {printers.map((p) => (
            <div key={p.id} className="rounded-xl border border-edge bg-panel p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <StatusDot st={statuses[p.id]} />
                  <span className="text-[15px] font-semibold">{p.name}</span>
                  {p.isDefault && (
                    <span className="rounded-full border border-edge-2 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-fg-dim">
                      default
                    </span>
                  )}
                </div>
              </div>
              <div className="mt-1.5 font-mono text-[12.5px] text-fg-dim">
                {p.kind === 'virtual'
                  ? 'built-in · renders to the tray below'
                  : `${p.host}:${p.port}`}{' '}
                · {p.dpmm === 12 ? '300' : '203'} dpi · {(p.widthDots / (p.dpmm === 12 ? 300 : 203)).toFixed(1)}″
              </div>
              <div className="mt-1 font-mono text-[12.5px] text-fg-dim">
                status: {statusText(statuses[p.id])}
              </div>
              {p.kind !== 'virtual' && (mediaByKind[p.kind]?.length ?? 0) > 0 && (
                <label className="mt-3 block">
                  <span className="mb-1 block font-mono text-[10px] uppercase tracking-wider text-fg-dim">
                    Loaded media — update when you swap paper
                  </span>
                  <select
                    className={`${inputCls} py-1.5 text-[13px]`}
                    value={p.media}
                    onChange={(e) => changeMedia(p.id, e.target.value)}
                  >
                    {mediaByKind[p.kind].map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.name}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <div className="mt-3 flex gap-2">
                {!p.isDefault && (
                  <button
                    onClick={() => setDefault(p.id)}
                    className="rounded-md border border-edge px-2.5 py-1 text-[12px] text-fg-dim transition-colors hover:text-fg"
                  >
                    Make default
                  </button>
                )}
                {p.kind !== 'virtual' && (
                  <button
                    onClick={() => remove(p)}
                    className={`rounded-md border px-2.5 py-1 text-[12px] transition-colors ${
                      confirmRemove === p.id
                        ? 'border-accent/60 bg-accent/10 text-accent-hi'
                        : 'border-edge text-fg-dim hover:border-accent/40 hover:text-accent-hi'
                    }`}
                  >
                    {confirmRemove === p.id ? 'Really remove?' : 'Remove'}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </section>

      <section>
        <h2 className="mb-1 font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-fg-dim">
          Output tray
        </h2>
        <p className="mb-3 text-[13px] text-fg-dim">
          Everything the virtual printer has "printed" — including raw ZPL sent to port 9100.
        </p>
        {tray.length === 0 ? (
          <div className="rounded-xl border border-dashed border-edge p-8 text-center text-[13.5px] text-fg-dim">
            Nothing in the tray yet.
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
            {tray.map((t) => (
              <div key={t.id} className="paper overflow-hidden">
                <div className="paper-perf" />
                <img src={`/api/tray/${t.id}.png`} alt="" className="block w-full" loading="lazy" />
                <div className="paper-perf" />
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
