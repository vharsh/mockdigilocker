import { useEffect, useState, type ReactNode, type FormEvent } from 'react';
import { Link, navigate, useLocation, match } from '../lib/router';
import { portal, portalStore, request, runtime, ApiError } from '../lib/api';
import { Alert, Button, Card, Field, Input, Logo, SandboxBar, Select, Tabs, cx, toast, useAsync, Loading, Badge } from '../components/ui';
import { Dashboard } from './Dashboard';
import { Documents, DocumentDetail } from './Documents';
import { FetchDocuments } from './Fetch';
import { Consents, Activity } from './Consents';
import { Settings } from './Settings';
import { Landing } from './Landing';

export function PortalApp() {
  const { path } = useLocation();
  const [session, setSession] = useState(portalStore.get());

  useEffect(() => {
    const onUnauth = (e: Event) => {
      if (String((e as CustomEvent).detail).startsWith('/portal-api') && portalStore.get()) {
        portalStore.set(null);
        setSession(null);
        toast.err('Your sandbox session ended — please sign in again');
        navigate('/login');
      }
    };
    window.addEventListener('mdl:unauthorized', onUnauth);
    return () => window.removeEventListener('mdl:unauthorized', onUnauth);
  }, []);

  const onLogin = (s: any) => { portalStore.set(s); setSession(s); navigate('/dashboard'); };
  const logout = async () => {
    try { await portal('POST', '/auth/logout', { refresh_token: session?.refresh_token }); } catch { /* ignore */ }
    portalStore.set(null); setSession(null); navigate('/');
  };

  if (path === '/') return <Shell session={session} logout={logout}><Landing session={session} /></Shell>;
  if (path === '/login') return <Shell session={session} logout={logout}><Login onLogin={onLogin} /></Shell>;
  if (path === '/register') return <Shell session={session} logout={logout}><Register onLogin={onLogin} /></Shell>;

  if (!session) {
    setTimeout(() => navigate('/login?next=' + encodeURIComponent(path), true));
    return null;
  }
  let page: ReactNode = <NotFound />;
  let m: Record<string, string> | null;
  if (path === '/dashboard') page = <Dashboard />;
  else if (path === '/documents') page = <Documents />;
  else if ((m = match('/documents/:uri', path))) page = <DocumentDetail uri={m.uri} />;
  else if (path === '/fetch') page = <FetchDocuments />;
  else if (path === '/consents') page = <Consents />;
  else if (path === '/activity') page = <Activity />;
  else if (path === '/settings') page = <Settings onLogout={logout} onUpdated={(u) => { const s = { ...session, user: u }; portalStore.set(s); setSession(s); }} />;
  return <Shell session={session} logout={logout} app>{page}</Shell>;
}

function NotFound() {
  return <Card><p className="text-sm text-slate-600">Page not found. <Link to="/dashboard" className="text-brand-600 underline">Go to dashboard</Link></p></Card>;
}

const nav = [
  ['/dashboard', 'Dashboard'], ['/documents', 'Issued documents'], ['/fetch', 'Fetch from issuer'], ['/consents', 'Shared with apps'], ['/activity', 'Activity'], ['/settings', 'Account'],
];

function Shell({ children, session, logout, app }: { children: ReactNode; session: any; logout: () => void; app?: boolean }) {
  const { path } = useLocation();
  const [open, setOpen] = useState(false);
  return (
    <div className="min-h-screen">
      <SandboxBar />
      <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/90 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-4">
          <Link to={session ? '/dashboard' : '/'} className="shrink-0"><Logo /></Link>
          {app && (
            <nav className="hidden items-center gap-0.5 xl:flex" aria-label="Portal">
              {nav.map(([to, label]) => (
                <Link key={to} to={to} className={cx('whitespace-nowrap rounded-md px-2.5 py-2 text-sm font-medium', path === to || (to !== '/dashboard' && path.startsWith(to)) ? 'bg-brand-50 text-brand-700' : 'text-slate-600 hover:text-slate-900')}>{label}</Link>
              ))}
            </nav>
          )}
          <div className="flex shrink-0 items-center gap-2">
            <a href="/docs" className="hidden text-sm font-medium text-slate-600 hover:text-slate-900 sm:block">Docs</a>
            {session ? (
              <>
                <span className="hidden whitespace-nowrap text-sm text-slate-700 2xl:block">{session.user?.name}</span>
                <Button variant="secondary" size="sm" onClick={logout}>Sign out</Button>
                {app && <button className="rounded-md p-2 text-slate-600 xl:hidden" onClick={() => setOpen(!open)} aria-label="Menu">☰</button>}
              </>
            ) : (
              path !== '/login' && <Button size="sm" onClick={() => navigate('/login')}>Sign in</Button>
            )}
          </div>
        </div>
        {app && open && (
          <nav className="border-t border-slate-100 px-4 py-2 xl:hidden" aria-label="Portal mobile">
            {nav.map(([to, label]) => <div key={to} onClick={() => setOpen(false)}><Link to={to} className="block rounded px-2 py-2 text-sm font-medium text-slate-700">{label}</Link></div>)}
          </nav>
        )}
      </header>
      <main className="mx-auto max-w-6xl px-4 py-8">{children}</main>
      <footer className="mx-auto max-w-6xl px-4 pb-10 text-center text-xs text-slate-500">
        Mock DigiLocker {runtime.version} — an open-source testing simulator. Not affiliated with or endorsed by DigiLocker / MeitY. All data is synthetic.
      </footer>
    </div>
  );
}

function PersonaPicker({ onPick }: { onPick: (id: string) => void }) {
  const meta = useAsync(() => request('GET', '/portal-api/meta'));
  const personas = meta.data?.test_personas || [];
  if (!personas.length) return null;
  return (
    <Card title="Predefined test personas" className="lg:col-span-2">
      <p className="mb-3 text-xs text-slate-500">Click to fill the login field. PIN for every persona: <code className="rounded bg-slate-100 px-1">{meta.data.default_pin}</code>{meta.data.otp_mode === 'fixed' && <> · OTP: <code className="rounded bg-slate-100 px-1">123456</code> (unless configured otherwise)</>}</p>
      <div className="grid gap-2 sm:grid-cols-2">
        {personas.map((p: any) => (
          <button key={p.mobile} onClick={() => onPick(p.mobile)} className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-left transition hover:border-brand-500 hover:bg-brand-50">
            <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">{p.name}{p.disabled && <Badge tone="red">disabled</Badge>}</div>
            <div className="mt-0.5 text-xs text-slate-600">{p.description}</div>
            <div className="mt-1 font-mono text-[11px] text-slate-500">{p.mobile} · {p.email}</div>
          </button>
        ))}
      </div>
    </Card>
  );
}

function Login({ onLogin }: { onLogin: (s: any) => void }) {
  const [mode, setMode] = useState<'otp' | 'pin'>('otp');
  const [identifier, setIdentifier] = useState('');
  const [secret, setSecret] = useState('');
  const [otpSent, setOtpSent] = useState<any>(null);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setErr(''); setBusy(true);
    try {
      if (mode === 'pin') onLogin(await request('POST', '/portal-api/auth/pin', { body: { identifier, pin: secret } }));
      else if (!otpSent) { setOtpSent(await request('POST', '/portal-api/auth/otp/send', { body: { identifier } })); setSecret(''); }
      else onLogin(await request('POST', '/portal-api/auth/otp/verify', { body: { identifier, otp: secret } }));
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally { setBusy(false); }
  };

  return (
    <div className="grid gap-6 lg:grid-cols-5">
      <Card className="lg:col-span-3">
        <h1 className="text-2xl font-bold">Sign in to your sandbox locker</h1>
        <p className="mt-1 text-sm text-slate-500">Use a synthetic test account — mobile number, e-mail or username.</p>
        <div className="mt-5"><Tabs tabs={[{ key: 'otp', label: 'OTP' }, { key: 'pin', label: 'Security PIN' }]} value={mode} onChange={(m) => { setMode(m); setOtpSent(null); setErr(''); setSecret(''); }} /></div>
        <form onSubmit={submit} className="mt-5 flex flex-col gap-4">
          {err && <Alert>{err}</Alert>}
          <Field label="Mobile / e-mail / username">
            <Input value={identifier} onChange={(e) => { setIdentifier(e.target.value); setOtpSent(null); }} placeholder="9999900001" autoComplete="username" required autoFocus />
          </Field>
          {mode === 'pin' && <Field label="6-digit security PIN"><Input type="password" inputMode="numeric" maxLength={6} value={secret} onChange={(e) => setSecret(e.target.value)} required autoComplete="current-password" /></Field>}
          {mode === 'otp' && otpSent && (
            <>
              <Alert tone="blue">OTP sent to {otpSent.masked_mobile} (delivered to the sandbox OTP outbox — no real SMS).{otpSent.sandbox_otp && <> Sandbox OTP: <code className="font-bold tracking-widest">{otpSent.sandbox_otp}</code></>}</Alert>
              <Field label="One-time password"><Input inputMode="numeric" maxLength={8} value={secret} onChange={(e) => setSecret(e.target.value)} required autoFocus autoComplete="one-time-code" /></Field>
            </>
          )}
          <Button type="submit" loading={busy}>{mode === 'pin' ? 'Sign in' : otpSent ? 'Verify OTP' : 'Send OTP'}</Button>
          <p className="text-sm text-slate-600">No account? <Link to="/register" className="font-semibold text-brand-600">Create a synthetic test account</Link></p>
        </form>
      </Card>
      <div className="lg:col-span-2"><PersonaPicker onPick={(id) => { setIdentifier(id); setOtpSent(null); }} /></div>
    </div>
  );
}

function Register({ onLogin }: { onLogin: (s: any) => void }) {
  const [f, setF] = useState({ name: '', dob: '', gender: '', mobile: '', email: '', pin: '' });
  const [stage, setStage] = useState<any>(null);
  const [otp, setOtp] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof f) => (e: any) => setF({ ...f, [k]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault(); setErr(''); setBusy(true);
    try {
      if (!stage) setStage(await request('POST', '/portal-api/auth/register', { body: f }));
      else onLogin(await request('POST', '/portal-api/auth/otp/verify', { body: { identifier: stage.identifier, otp } }));
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };

  return (
    <Card className="mx-auto max-w-2xl">
      <h1 className="text-2xl font-bold">Create a synthetic sandbox account</h1>
      <Alert tone="amber" className="mt-3">Use <strong>fictional</strong> details only. Never enter your real name, Aadhaar, PAN or phone number. Suggested mobile range: 99999xxxxx · e-mail domain: <code>sandbox.test</code></Alert>
      <form onSubmit={submit} className="mt-5 grid gap-4 sm:grid-cols-2">
        {err && <Alert className="sm:col-span-2">{err}</Alert>}
        {!stage ? (
          <>
            <Field label="Full name (fictional)" className="sm:col-span-2"><Input value={f.name} onChange={set('name')} required placeholder="Test User" /></Field>
            <Field label="Date of birth"><Input type="date" value={f.dob} onChange={set('dob')} required /></Field>
            <Field label="Gender"><Select value={f.gender} onChange={set('gender')} required><option value="">Select</option><option value="F">Female</option><option value="M">Male</option><option value="T">Transgender</option></Select></Field>
            <Field label="Mobile (10 digits)"><Input value={f.mobile} onChange={set('mobile')} inputMode="numeric" pattern="\d{10}" required placeholder="99999xxxxx" /></Field>
            <Field label="E-mail (optional)"><Input type="email" value={f.email} onChange={set('email')} placeholder="name@sandbox.test" /></Field>
            <Field label="6-digit security PIN" className="sm:col-span-2"><Input type="password" inputMode="numeric" pattern="\d{6}" maxLength={6} value={f.pin} onChange={set('pin')} required /></Field>
            <Button type="submit" loading={busy} className="sm:col-span-2">Create account &amp; send OTP</Button>
          </>
        ) : (
          <>
            <Alert tone="blue" className="sm:col-span-2">Account created. Verify the OTP sent to {stage.identifier}.{stage.sandbox_otp && <> Sandbox OTP: <code className="font-bold tracking-widest">{stage.sandbox_otp}</code></>}</Alert>
            <Field label="One-time password" className="sm:col-span-2"><Input value={otp} onChange={(e) => setOtp(e.target.value)} required autoFocus inputMode="numeric" /></Field>
            <Button type="submit" loading={busy} className="sm:col-span-2">Verify &amp; open my locker</Button>
          </>
        )}
      </form>
    </Card>
  );
}

export { Loading };
