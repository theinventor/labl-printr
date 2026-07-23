import { BrowserRouter, NavLink, Route, Routes } from 'react-router-dom';
import { ToastProvider } from './ui';
import { PrintPage } from './pages/PrintPage';
import { JobsPage } from './pages/JobsPage';
import { PrintersPage } from './pages/PrintersPage';

function Wordmark() {
  return (
    <div className="flex items-center gap-2.5">
      <svg viewBox="0 0 100 100" className="size-7 rounded-md">
        <rect width="100" height="100" rx="18" fill="#f7f4ee" />
        <rect x="18" y="26" width="8" height="48" fill="#0d0e10" />
        <rect x="32" y="26" width="4" height="48" fill="#0d0e10" />
        <rect x="42" y="26" width="12" height="48" fill="#ff4f30" />
        <rect x="60" y="26" width="6" height="48" fill="#0d0e10" />
        <rect x="72" y="26" width="10" height="48" fill="#0d0e10" />
      </svg>
      <span className="font-mono text-[17px] font-medium tracking-tight">
        labl<span className="text-accent">-</span>printr
      </span>
    </div>
  );
}

const navCls = ({ isActive }: { isActive: boolean }) =>
  `rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
    isActive ? 'bg-panel-2 text-fg' : 'text-fg-dim hover:text-fg'
  }`;

export default function App() {
  return (
    <BrowserRouter>
      <ToastProvider>
        <div className="mx-auto flex min-h-dvh max-w-6xl flex-col px-4 sm:px-6">
          <header className="flex items-center justify-between py-5">
            <Wordmark />
            <nav className="flex items-center gap-1">
              <NavLink to="/" end className={navCls}>
                Print
              </NavLink>
              <NavLink to="/jobs" className={navCls}>
                Jobs
              </NavLink>
              <NavLink to="/printers" className={navCls}>
                Printers
              </NavLink>
              <a
                href="/designer/"
                className="ml-2 rounded-md border border-edge px-3 py-1.5 text-sm font-medium text-fg-dim transition-colors hover:border-edge-2 hover:text-fg"
              >
                Designer ↗
              </a>
            </nav>
          </header>
          <main className="flex-1 pb-16">
            <Routes>
              <Route path="/" element={<PrintPage />} />
              <Route path="/jobs" element={<JobsPage />} />
              <Route path="/printers" element={<PrintersPage />} />
            </Routes>
          </main>
        </div>
      </ToastProvider>
    </BrowserRouter>
  );
}
