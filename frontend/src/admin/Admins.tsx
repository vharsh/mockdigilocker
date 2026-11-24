import { useState, type FormEvent } from 'react';
import { admin } from '../lib/api';
import { Alert, Badge, Button, Card, Field, Input, Loading, PageHeader, Select, toast, useAsync } from '../components/ui';
import { fmtDate } from '../lib/format';
import { useAdmin } from './AdminApp';

export function Admins() {
  const me = useAdmin();
  const { data, error, loading, reload } = useAsync(() => admin('GET', '/admins'));
  const [f, setF] = useState({ username: '', password: '', role: 'qa' });
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try { await admin('POST', '/admins', f); toast.ok('Console user saved'); setF({ username: '', password: '', role: 'qa' }); reload(); } catch (e) { toast.err(e); }
  };
  const del = async (u: string) => { if (!confirm('Delete ' + u + '?')) return; try { await admin('DELETE', '/admins/' + u); reload(); } catch (e) { toast.err(e); } };
  if (error) return <Alert>{error.message}</Alert>;
  return (
    <div>
      <PageHeader title="Console users" subtitle="Role-based access: admin (everything), qa (personas, clients, scenarios, resets), viewer (read-only)." />
      <div className="grid gap-6 lg:grid-cols-3">
        <Card title="Users" className="lg:col-span-2">{loading && !data ? <Loading /> : (
          <table className="w-full text-sm"><thead className="text-left text-xs uppercase text-slate-500"><tr><th className="py-2">Username</th><th>Role</th><th>Created</th><th /></tr></thead>
            <tbody className="divide-y divide-slate-100">{data.items.map((a: any) => (
              <tr key={a.username}><td className="py-2 font-medium">{a.username}{a.disabled && <Badge tone="red" className="ml-2">disabled</Badge>}</td><td><Badge tone={a.role === 'admin' ? 'red' : a.role === 'qa' ? 'amber' : 'slate'}>{a.role}</Badge></td>
                <td className="text-xs">{fmtDate(a.created_at)}</td><td className="text-right">{a.username !== me.username && <Button size="sm" variant="ghost" onClick={() => del(a.username)}>Delete</Button>}</td></tr>
            ))}</tbody></table>
        )}</Card>
        <Card title="Add or update user">
          <form onSubmit={submit} className="flex flex-col gap-3">
            <Field label="Username"><Input value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} required /></Field>
            <Field label="Password" hint="min 8 chars; blank keeps existing"><Input type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} /></Field>
            <Field label="Role"><Select value={f.role} onChange={(e) => setF({ ...f, role: e.target.value })}><option value="admin">admin</option><option value="qa">qa</option><option value="viewer">viewer</option></Select></Field>
            <Button variant="console" type="submit">Save</Button>
          </form>
        </Card>
      </div>
    </div>
  );
}
