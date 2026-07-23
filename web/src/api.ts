export type Field = {
  key: string;
  label: string;
  type: 'text' | 'textarea' | 'url';
  required?: boolean;
  placeholder?: string;
};

export type Template = {
  id: string;
  name: string;
  description: string;
  fields: Field[];
  builtin: boolean;
};

export type Printer = {
  id: number;
  name: string;
  kind: 'network' | 'virtual';
  host?: string;
  port: number;
  dpmm: number;
  widthDots: number;
  leftShift: number;
  isDefault: boolean;
};

export type PrinterStatus = {
  ready: boolean;
  reachable: boolean;
  paperOut: boolean;
  paused: boolean;
  headOpen: boolean;
  formatsBuffered: number;
  detail?: string;
};

export type Job = {
  id: number;
  printerId: number;
  printerName?: string;
  templateId?: string;
  vars?: Record<string, string>;
  widthDots: number;
  lengthDots: number;
  copies: number;
  state: 'queued' | 'printing' | 'done' | 'failed' | 'canceled';
  error?: string;
  source: string;
  createdAt: string;
};

export type Preview = {
  png: string; // base64
  zpl: string;
  widthDots: number;
  lengthDots: number;
};

export type TrayPrint = { id: number; jobId?: number; createdAt: string };

export type Discovered = { ip: string; info: string[] };

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as { error?: string }).error ?? `HTTP ${res.status}`);
  }
  return data as T;
}

export const api = {
  templates: () => request<Template[]>('GET', '/api/templates'),
  deleteTemplate: (id: string) => request<unknown>('DELETE', `/api/templates/${id}`),
  preview: (body: { templateId: string; vars: Record<string, string>; printerId?: number }) =>
    request<Preview>('POST', '/api/preview', body),
  createJob: (body: {
    templateId: string;
    vars: Record<string, string>;
    printerId?: number;
    copies: number;
    source: string;
    idempotencyKey: string;
  }) => request<Job>('POST', '/api/jobs', body),
  jobs: (limit = 40) => request<Job[]>('GET', `/api/jobs?limit=${limit}`),
  reprint: (id: number) => request<Job>('POST', `/api/jobs/${id}/reprint`),
  printers: () => request<Printer[]>('GET', '/api/printers'),
  printerStatus: (id: number) => request<PrinterStatus>('GET', `/api/printers/${id}/status`),
  createPrinter: (body: Partial<Printer>) => request<Printer>('POST', '/api/printers', body),
  deletePrinter: (id: number) => request<unknown>('DELETE', `/api/printers/${id}`),
  setDefault: (id: number) => request<unknown>('POST', `/api/printers/${id}/default`),
  discover: () => request<Discovered[]>('POST', '/api/printers/discover'),
  tray: (limit = 30) => request<TrayPrint[]>('GET', `/api/tray?limit=${limit}`),
};
