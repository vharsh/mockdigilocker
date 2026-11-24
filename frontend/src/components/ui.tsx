import { useCallback, useEffect, useRef, useState, type ReactNode, type InputHTMLAttributes, type SelectHTMLAttributes, type TextareaHTMLAttributes, type ButtonHTMLAttributes } from 'react';
import { runtime } from '../lib/api';

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(' ');
}

type Tone = 'green' | 'red' | 'amber' | 'slate' | 'blue' | 'violet';
const toneCls: Record<Tone, string> = {
  green: 'bg-emerald-50 text-emerald-700 ring-emerald-600/20',
  red: 'bg-rose-50 text-rose-700 ring-rose-600/20',
  amber: 'bg-amber-50 text-amber-800 ring-amber-600/20',
  slate: 'bg-slate-100 text-slate-700 ring-slate-500/20',
  blue: 'bg-sky-50 text-sky-700 ring-sky-600/20',
  violet: 'bg-violet-50 text-violet-700 ring-violet-600/20',
};

export function Badge({ tone = 'slate', children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return <span className={cx('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide ring-1 ring-inset whitespace-nowrap', toneCls[tone], className)}>{children}</span>;
}

type BtnVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'console' | 'docs';
const btnCls: Record<BtnVariant, string> = {
  primary: 'bg-brand-600 text-white hover:bg-brand-700 shadow-sm',
  secondary: 'bg-white text-slate-800 ring-1 ring-inset ring-slate-300 hover:bg-slate-50',
  ghost: 'text-slate-600 hover:bg-slate-100',
  danger: 'bg-rose-600 text-white hover:bg-rose-700',
  console: 'bg-amber-500 text-slate-950 hover:bg-amber-400 shadow-sm',
  docs: 'bg-teal-600 text-white hover:bg-teal-700',
};

export function Button({ variant = 'primary', size = 'md', loading, className, children, ...rest }:
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: BtnVariant; size?: 'sm' | 'md'; loading?: boolean }) {
  return (
    <button {...rest} disabled={rest.disabled || loading}
      className={cx('inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-lg font-semibold transition disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-600',
        size === 'sm' ? 'px-2.5 py-1.5 text-xs' : 'px-4 py-2 text-sm', btnCls[variant], className)}>
      {loading && <Spinner className="h-3.5 w-3.5" />}{children}
    </button>
  );
}

export function Spinner({ className = 'h-5 w-5' }: { className?: string }) {
  return <svg className={cx('animate-spin', className)} viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" strokeOpacity=".25" strokeWidth="4" /><path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" strokeWidth="4" strokeLinecap="round" /></svg>;
}

export function Card({ children, className, title, actions, dark }: { children: ReactNode; className?: string; title?: ReactNode; actions?: ReactNode; dark?: boolean }) {
  return (
    <section className={cx('rounded-xl border', dark ? 'border-slate-800 bg-console-900' : 'border-slate-200 bg-white shadow-sm', className)}>
      {(title || actions) && (
        <header className={cx('flex flex-wrap items-center justify-between gap-2 border-b px-5 py-3', dark ? 'border-slate-800' : 'border-slate-100')}>
          <h2 className={cx('text-sm font-semibold', dark ? 'text-slate-100' : 'text-slate-900')}>{title}</h2>
          <div className="flex flex-wrap items-center gap-2">{actions}</div>
        </header>
      )}
      <div className="p-5">{children}</div>
    </section>
  );
}

export function Field({ label, hint, children, className }: { label: string; hint?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label className={cx('flex flex-col gap-1.5', className)}>
      <span className="text-xs font-semibold text-slate-600">{label}</span>
      {children}
      {hint && <span className="text-xs text-slate-500">{hint}</span>}
    </label>
  );
}

const inputCls = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:bg-slate-100';
export const Input = (p: InputHTMLAttributes<HTMLInputElement>) => <input {...p} className={cx(inputCls, p.className)} />;
export const Select = (p: SelectHTMLAttributes<HTMLSelectElement>) => <select {...p} className={cx(inputCls, p.className)} />;
export const Textarea = (p: TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...p} className={cx(inputCls, 'font-mono text-xs', p.className)} />;

export function Alert({ tone = 'red', children, className }: { tone?: 'red' | 'amber' | 'green' | 'blue'; children: ReactNode; className?: string }) {
  const c = { red: 'bg-rose-50 text-rose-800 border-rose-200', amber: 'bg-amber-50 text-amber-900 border-amber-200', green: 'bg-emerald-50 text-emerald-800 border-emerald-200', blue: 'bg-sky-50 text-sky-800 border-sky-200' }[tone];
  return <div role={tone === 'red' ? 'alert' : undefined} className={cx('rounded-lg border px-3 py-2 text-sm', c, className)}>{children}</div>;
}

export function Empty({ title, children, icon = '◌' }: { title: string; children?: ReactNode; icon?: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-12 text-center">
      <div className="grid h-12 w-12 place-items-center rounded-full bg-slate-100 text-xl text-slate-400">{icon}</div>
      <p className="font-semibold text-slate-700">{title}</p>
      {children && <div className="max-w-md text-sm text-slate-500">{children}</div>}
    </div>
  );
}

export function Loading() {
  return <div className="flex justify-center py-16 text-slate-400"><Spinner /></div>;
}

export function Modal({ open, onClose, title, children, wide }: { open: boolean; onClose: () => void; title: string; children: ReactNode; wide?: boolean }) {
  useEffect(() => {
    if (!open) return;
    const k = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', k);
    return () => window.removeEventListener('keydown', k);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-950/50 p-4 pt-[6vh]" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div role="dialog" aria-modal="true" aria-label={title} className={cx('w-full rounded-xl bg-white shadow-2xl', wide ? 'max-w-4xl' : 'max-w-lg')}>
        <header className="flex items-center justify-between border-b border-slate-100 px-5 py-3">
          <h3 className="font-semibold text-slate-900">{title}</h3>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700" aria-label="Close">✕</button>
        </header>
        <div className="p-5">{children}</div>
      </div>
    </div>
  );
}

export function CopyButton({ text, label = 'Copy', className }: { text: string; label?: string; className?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button type="button" className={cx('rounded px-2 py-0.5 text-xs font-semibold text-slate-500 hover:bg-slate-200 hover:text-slate-800', className)}
      onClick={() => { navigator.clipboard?.writeText(text); setDone(true); setTimeout(() => setDone(false), 1200); }}>
      {done ? 'Copied' : label}
    </button>
  );
}

export function Code({ children, className, copy = true }: { children: string; className?: string; copy?: boolean }) {
  return (
    <div className={cx('group relative', className)}>
      <pre className="max-h-[420px] overflow-auto rounded-lg bg-slate-950 p-3 font-mono text-[12px] leading-5 text-slate-200 whitespace-pre-wrap break-all">{children}</pre>
      {copy && <CopyButton text={children} className="absolute right-2 top-2 bg-slate-800 text-slate-300 opacity-0 group-hover:opacity-100 hover:bg-slate-700 hover:text-white" />}
    </div>
  );
}

export function KV({ items, className }: { items: [ReactNode, ReactNode][]; className?: string }) {
  return (
    <dl className={cx('grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2', className)}>
      {items.map(([k, v], i) => (
        <div key={i} className="min-w-0">
          <dt className="text-[11px] font-semibold uppercase tracking-wide text-slate-500">{k}</dt>
          <dd className="mt-0.5 break-words text-sm text-slate-900">{v || '—'}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Tabs<T extends string>({ tabs, value, onChange, dark }: { tabs: { key: T; label: ReactNode }[]; value: T; onChange: (k: T) => void; dark?: boolean }) {
  return (
    <div role="tablist" className={cx('flex gap-1 overflow-x-auto rounded-lg p-1', dark ? 'bg-slate-800' : 'bg-slate-100')}>
      {tabs.map((t) => (
        <button key={t.key} role="tab" aria-selected={value === t.key} onClick={() => onChange(t.key)}
          className={cx('whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-semibold transition',
            value === t.key ? (dark ? 'bg-slate-950 text-white' : 'bg-white text-slate-900 shadow-sm') : (dark ? 'text-slate-400 hover:text-white' : 'text-slate-500 hover:text-slate-800'))}>
          {t.label}
        </button>
      ))}
    </div>
  );
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <button type="button" role="switch" aria-checked={checked} aria-label={label} onClick={() => onChange(!checked)}
      className={cx('relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition', checked ? 'bg-emerald-500' : 'bg-slate-300')}>
      <span className={cx('inline-block h-4 w-4 rounded-full bg-white shadow transition', checked ? 'translate-x-4.5' : 'translate-x-0.5')} />
    </button>
  );
}

// ---- toasts ----
type ToastMsg = { id: number; text: string; tone: 'green' | 'red' };
let pushToast: (t: Omit<ToastMsg, 'id'>) => void = () => {};
export const toast = {
  ok: (text: string) => pushToast({ text, tone: 'green' }),
  err: (e: unknown) => pushToast({ text: e instanceof Error ? e.message : String(e), tone: 'red' }),
};

export function Toaster() {
  const [list, setList] = useState<ToastMsg[]>([]);
  const n = useRef(0);
  useEffect(() => {
    pushToast = (t) => {
      const id = ++n.current;
      setList((l) => [...l, { ...t, id }]);
      setTimeout(() => setList((l) => l.filter((x) => x.id !== id)), 4500);
    };
  }, []);
  return (
    <div aria-live="polite" className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-80 flex-col gap-2">
      {list.map((t) => (
        <div key={t.id} className={cx('pointer-events-auto rounded-lg px-4 py-3 text-sm font-medium shadow-lg ring-1', t.tone === 'green' ? 'bg-emerald-600 text-white ring-emerald-700' : 'bg-rose-600 text-white ring-rose-700')}>{t.text}</div>
      ))}
    </div>
  );
}

// ---- data hook ----
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [state, setState] = useState<{ data?: T; error?: Error; loading: boolean }>({ loading: true });
  const run = useCallback(() => {
    setState((s) => ({ ...s, loading: true }));
    return fn().then((data) => setState({ data, loading: false }), (error) => setState({ error, loading: false }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(() => { run(); }, [run]);
  return { ...state, reload: run };
}

export function SandboxBar({ variant = 'portal' }: { variant?: 'portal' | 'console' | 'docs' }) {
  const c = variant === 'console' ? 'bg-amber-500 text-slate-950' : variant === 'docs' ? 'bg-teal-800 text-teal-50' : 'bg-rose-700 text-white';
  return (
    <div className={cx('px-4 py-1.5 text-center text-xs font-medium', c)}>
      <strong className="tracking-wider">SANDBOX</strong> · Mock DigiLocker for testing only · env <code className="rounded bg-black/15 px-1">{runtime.env}</code> · never enter real Aadhaar, PAN or personal data
    </div>
  );
}

export function Logo({ dark, sub }: { dark?: boolean; sub?: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="grid h-9 w-9 place-items-center rounded-lg bg-brand-600 text-sm font-bold text-white">ML</div>
      <div className="leading-tight">
        <div className={cx('font-bold', dark ? 'text-white' : 'text-slate-900')}>Mock DigiLocker</div>
        <div className={cx('text-[11px]', dark ? 'text-slate-400' : 'text-slate-500')}>{sub || 'DigiLocker-compatible sandbox'}</div>
      </div>
    </div>
  );
}

export function PageHeader({ title, subtitle, actions, dark }: { title: ReactNode; subtitle?: ReactNode; actions?: ReactNode; dark?: boolean }) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className={cx('text-2xl font-bold tracking-tight', dark ? 'text-white' : 'text-slate-900')}>{title}</h1>
        {subtitle && <p className={cx('mt-1 text-sm', dark ? 'text-slate-400' : 'text-slate-500')}>{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </div>
  );
}
