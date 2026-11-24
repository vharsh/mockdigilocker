import { useEffect, useState, type FormEvent } from 'react';
import { portal } from '../lib/api';
import { Alert, Button, Card, Field, Input, KV, Loading, PageHeader, Select, toast, useAsync } from '../components/ui';

export function Settings({ onLogout, onUpdated }: { onLogout: () => void; onUpdated: (u: any) => void }) {
  const me = useAsync(() => portal('GET', '/me'));
  const [f, setF] = useState<any>(null);
  const [pin, setPin] = useState({ current_pin: '', new_pin: '' });
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  useEffect(() => { if (me.data) setF({ ...me.data, address: { ...me.data.address } }); }, [me.data]);
  if (!f) return <Loading />;
  const set = (k: string) => (e: any) => setF({ ...f, [k]: e.target.value });
  const setA = (k: string) => (e: any) => setF({ ...f, address: { ...f.address, [k]: e.target.value } });

  const save = async (e: FormEvent) => {
    e.preventDefault(); setBusy(true); setErr('');
    try {
      const u = await portal('PUT', '/me', { name: f.name, dob: f.dob, gender: f.gender, mobile: f.mobile, email: f.email, father_name: f.father_name, address: f.address });
      onUpdated(u); toast.ok('Profile updated — issued documents now reflect the new details');
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  const changePin = async (e: FormEvent) => {
    e.preventDefault();
    try { await portal('POST', '/me/pin', pin); toast.ok('PIN changed'); setPin({ current_pin: '', new_pin: '' }); } catch (e) { toast.err(e); }
  };
  const addr: [string, string][] = [['house', 'House / building'], ['street', 'Street'], ['landmark', 'Landmark'], ['locality', 'Locality'], ['vtc', 'Village / town / city'], ['district', 'District'], ['state', 'State'], ['pincode', 'PIN code']];

  return (
    <div>
      <PageHeader title="Account" subtitle="Update this synthetic persona. Changes flow into issued documents and API responses." actions={<Button variant="secondary" onClick={onLogout}>Sign out</Button>} />
      <div className="grid gap-6 lg:grid-cols-3">
        <form onSubmit={save} className="lg:col-span-2">
          <Card title="Personal details" actions={<Button type="submit" size="sm" loading={busy}>Save changes</Button>}>
            {err && <Alert className="mb-4">{err}</Alert>}
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Full name" className="sm:col-span-2"><Input value={f.name} onChange={set('name')} required /></Field>
              <Field label="Date of birth"><Input type="date" value={f.dob} onChange={set('dob')} required /></Field>
              <Field label="Gender"><Select value={f.gender} onChange={set('gender')}><option value="F">Female</option><option value="M">Male</option><option value="T">Transgender</option></Select></Field>
              <Field label="Mobile"><Input value={f.mobile} onChange={set('mobile')} pattern="\d{10}" required /></Field>
              <Field label="E-mail"><Input type="email" value={f.email} onChange={set('email')} /></Field>
              <Field label="Father's / guardian's name" className="sm:col-span-2"><Input value={f.father_name || ''} onChange={set('father_name')} /></Field>
              <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-slate-500 sm:col-span-2">Address</div>
              {addr.map(([k, l]) => <Field key={k} label={l}><Input value={f.address?.[k] || ''} onChange={setA(k)} /></Field>)}
            </div>
          </Card>
        </form>
        <div className="flex flex-col gap-6">
          <Card title="Synthetic identifiers">
            <KV className="sm:grid-cols-1" items={[['DigiLocker ID', <code className="text-xs">{f.id}</code>], ['Username', f.username], ['Synthetic Aadhaar (UID)', f.synthetic_uid || '—'],
              ['PAN', f.pan], ['Driving licence', f.driving_licence], ['Reference key', <code className="break-all text-[11px]">{f.reference_key}</code>]]} />
          </Card>
          <Card title="Security PIN">
            <form onSubmit={changePin} className="flex flex-col gap-3">
              <Field label="Current PIN"><Input type="password" inputMode="numeric" maxLength={6} value={pin.current_pin} onChange={(e) => setPin({ ...pin, current_pin: e.target.value })} /></Field>
              <Field label="New 6-digit PIN"><Input type="password" inputMode="numeric" pattern="\d{6}" maxLength={6} value={pin.new_pin} onChange={(e) => setPin({ ...pin, new_pin: e.target.value })} required /></Field>
              <Button type="submit" variant="secondary">Change PIN</Button>
            </form>
          </Card>
        </div>
      </div>
    </div>
  );
}
