import { createContext, useContext, useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { Link, navigate, useLocation, match } from '../lib/router';
import { admin, adminStore, request, type AdminSession } from '../lib/api';
import { Alert, Button, Card, Field, Input, Logo, SandboxBar, cx, toast } from '../components/ui';
import { Overview } from './Overview';
import { Personas, PersonaDetail } from './Personas';
import { Clients } from './Clients';
import { Scenarios } from './Scenarios';
import { Inspector } from './Inspector';
import { Webhooks } from './Webhooks';
import { Sessions } from './Sessions';
import { Environment } from './Environment';
import { Admins } from './Admins';

const AdminCtx = createContext<AdminSession | null>(null);
export const useAdmin = () => useContext(AdminCtx)!;
export const useCanWrite = () => useAdmin()?.role !== 'viewer';

const nav: [string, string, string][] = [
  ['/admin', 'Overview', '◎'],
  ['/admin/personas', 'Test personas', '☺'],
  ['/admin/clients', 'Integration clients', '⚙'],
  ['/admin/scenarios', 'Scenario simulator', '⚡'],
  ['/admin/inspector', 'API inspector', '⌕'],
  ['/admin/webhooks', 'Webhooks', '↯'],
  ['/admin/sessions', 'Consents & sessions', '⛁'],
  ['/admin/environment', 'Environment & reset', '↺'],
  ['/admin/admins', 'Console users', '♟'],
];

export function AdminApp() {
  const { path } = useLocation();
  const [session, setSession] = useState(adminStore.get());

  useEffect(() => {
    const onUnauth = (e: Event) => {
      if (String((e as CustomEvent).detail).startsWith('/admin-api') && adminStore.get()) {
        adminStore.set(null); setSession(null); navigate('/admin/login');
      }
    };
    window.addEventListener('mdl:unauthorized', onUnauth);
    return () => window.removeEventListener('mdl:unauthorized', onUnauth);
  }, []);

  if (!session || path === '/admin/login') {
    return <AdminLogin onLogin={(s) => { adminStore.set(s); setSession(s); navigate('/admin'); }} />;
  }
  const logout = async () => { try { await admin('POST', '/logout'); } catch { /* */ } adminStore.set(null); setSession(null); navigate('/admin/login'); };

  let page: ReactNode = <Card><p>Not found</p></Card>;
  let m: Record<string, string> | null;
  if (path === '/admin' || path === '/admin/') page = <Overview />;
  else if (path === '/admin/personas') page = <Personas />;
  else if ((m = match('/admin/personas/:id', path))) page = <PersonaDetail id={m.id} />;
  else if (path === '/admin/clients') page = <Clients />;
  else if (path === '/admin/scenarios') page = <Scenarios />;
  else if (path === '/admin/inspector') page = <Inspector />;
  else if (path === '/admin/webhooks') page = <Webhooks />;
  else if (path === '/admin/sessions') page = <Sessions />;
  else if (path === '/admin/environment') page = <Environment />;
  else if (path === '/admin/admins') page = <Admins />;

  return (
    <AdminCtx.Provider value={session}>
      <div className="min-h-screen bg-slate-100">
        <SandboxBar variant="console" />
        <div className="flex">
          <aside className="sticky top-0 hidden h-screen w-64 shrink-0 flex-col bg-console-950 text-slate-300 lg:flex">
            <div className="border-b border-slate-800 px-5 py-4"><Logo dark sub="Developer & QA console" /></div>
            <nav className="flex-1 overflow-y-auto px-3 py-4" aria-label="Console">
              {nav.filter(([to]) => to !== '/admin/admins' || session.role === 'admin').map(([to, label, icon]) => {
                const active = to === '/admin' ? path === '/admin' || path === '/admin/' : path.startsWith(to);
                return (
                  <Link key={to} to={to} className={cx('mb-0.5 flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition',
                    active ? 'bg-amber-500/10 text-amber-300' : 'text-slate-400 hover:bg-slate-800 hover:text-white')}>
                    <span className="w-4 text-center">{icon}</span>{label}
                  </Link>
                );
              })}
            </nav>
            <div className="border-t border-slate-800 p-4 text-xs">
              <div className="text-slate-400">Signed in as <span className="font-semibold text-white">{session.username}</span></div>
              <div className="mt-1 flex items-center justify-between"><span className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-amber-300">{session.role}</span>
                <button onClick={logout} className="text-slate-400 hover:text-white">Sign out</button></div>
              <div className="mt-3 flex gap-3"><a href="/" className="text-slate-500 hover:text-white">Portal</a><a href="/docs" className="text-slate-500 hover:text-white">Docs</a><a href="/demo-partner" className="text-slate-500 hover:text-white">Demo partner</a></div>
            </div>
          </aside>
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between border-b border-slate-800 bg-console-950 px-4 py-3 lg:hidden">
              <Logo dark sub="Developer console" />
              <select className="rounded bg-slate-800 px-2 py-1 text-sm text-white" value={path} onChange={(e) => navigate(e.target.value)} aria-label="Navigate">
                {nav.map(([to, label]) => <option key={to} value={to}>{label}</option>)}
              </select>
            </div>
            {session.default_password && <div className="bg-amber-100 px-6 py-2 text-sm text-amber-900">You are using the default admin password. Set <code>ADMIN_PASSWORD</code> before sharing this environment.</div>}
            <main className="mx-auto max-w-7xl p-4 sm:p-8">{page}</main>
          </div>
        </div>
      </div>
    </AdminCtx.Provider>
  );
}

function AdminLogin({ onLogin }: { onLogin: (s: AdminSession) => void }) {
  const [u, setU] = useState('admin');
  const [p, setP] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault(); setBusy(true); setErr('');
    try {
      const s = await request('POST', '/admin-api/login', { body: { username: u, password: p } });
      onLogin(s);
      toast.ok('Welcome, ' + s.username);
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  return (
    <div className="min-h-screen bg-console-950">
      <SandboxBar variant="console" />
      <div className="mx-auto flex max-w-sm flex-col gap-6 px-4 pt-[12vh]">
        <Logo dark sub="Developer & QA console" />
        <form onSubmit={submit} className="flex flex-col gap-4 rounded-xl bg-white p-6 shadow-xl">
          <h1 className="text-lg font-bold">Console sign in</h1>
          {err && <Alert>{err}</Alert>}
          <Field label="Username"><Input value={u} onChange={(e) => setU(e.target.value)} autoComplete="username" required /></Field>
          <Field label="Password" hint="Local default: admin / admin123 (override with ADMIN_PASSWORD)"><Input type="password" value={p} onChange={(e) => setP(e.target.value)} autoComplete="current-password" required autoFocus /></Field>
          <Button variant="console" type="submit" loading={busy}>Sign in</Button>
        </form>
        <a href="/" className="text-center text-sm text-slate-400 hover:text-white">← Back to the user portal</a>
      </div>
    </div>
  );
}
