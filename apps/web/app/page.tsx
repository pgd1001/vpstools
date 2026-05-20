'use client';
import { useState, useEffect } from 'react';
import { api, Server, Runbook, Approval, AuditEvent, Execution } from './api';

function Tab({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button onClick={onClick} style={{
      padding: '8px 16px', border: 'none', cursor: 'pointer',
      background: active ? '#0ea5e9' : '#1e293b', color: active ? '#fff' : '#94a3b8',
      borderRadius: '6px 6px 0 0', fontWeight: active ? 600 : 400,
    }}>{label}</button>
  );
}

function UserSwitcher() {
  const [user, setUser] = useState(api.getUser());
  return (
    <select value={user} onChange={e => { api.setUser(e.target.value); setUser(e.target.value); }}
      style={{ padding: '4px 8px', background: '#1e293b', color: '#e2e8f0', border: '1px solid #334155', borderRadius: 6 }}>
      <option value="user_senior">Senior Engineer</option>
      <option value="user_junior">Junior Engineer</option>
      <option value="user_auditor">Auditor</option>
    </select>
  );
}

export default function Home() {
  const [tab, setTab] = useState<'servers'|'runbooks'|'approvals'|'executions'|'audit'>('servers');
  const [servers, setServers] = useState<Server[]>([]);
  const [runbooks, setRunbooks] = useState<Runbook[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [status, setStatus] = useState('');

  const load = async (t: string) => {
    setStatus('loading...');
    try {
      if (t === 'servers') setServers((await api.servers()).servers);
      if (t === 'runbooks') setRunbooks((await api.runbooks()).runbooks);
      if (t === 'approvals') setApprovals((await api.approvals()).approvals);
      if (t === 'executions') setExecutions((await api.executions()).executions);
      if (t === 'audit') setAudit((await api.audit()).events);
      setStatus('');
    } catch (e: any) { setStatus(e.message); }
  };

  useEffect(() => { load(tab); }, [tab]);

  const handleApprove = async (id: string) => {
    await api.approve(id); load('approvals');
  };
  const handleDeny = async (id: string) => {
    await api.deny(id); load('approvals');
  };

  return (
    <div style={{ minHeight: '100vh', background: '#0f172a', color: '#e2e8f0', fontFamily: 'monospace' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 20px', background: '#1e293b', borderBottom: '1px solid #334155' }}>
        <h1 style={{ margin: 0, fontSize: 18, color: '#0ea5e9' }}>VPS Tools Console</h1>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <span style={{ color: '#94a3b8', fontSize: 13 }}>User:</span>
          <UserSwitcher />
        </div>
      </header>
      <nav style={{ display: 'flex', gap: 2, padding: '0 20px', background: '#1e293b' }}>
        {(['servers','runbooks','approvals','executions','audit'] as const).map(t =>
          <Tab key={t} label={t.charAt(0).toUpperCase()+t.slice(1)} active={tab===t} onClick={() => setTab(t)} />
        )}
      </nav>
      <main style={{ padding: 20 }}>
        {status && <p style={{ color: '#f87171' }}>{status}</p>}

        {tab === 'servers' && (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #334155' }}>
                <th style={th}>Name</th><th style={th}>Hostname</th><th style={th}>Environment</th><th style={th}>Status</th><th style={th}>OS</th>
              </tr>
            </thead>
            <tbody>
              {servers.map(s => (
                <tr key={s.id} style={{ borderBottom: '1px solid #1e293b' }}>
                  <td style={td}><strong>{s.name}</strong></td>
                  <td style={td}>{s.hostname||'-'}</td>
                  <td style={td}>{s.environment}</td>
                  <td style={td}>{s.status}</td>
                  <td style={td}>{s.os_name||'-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === 'runbooks' && (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #334155' }}>
                <th style={th}>Name</th><th style={th}>Title</th><th style={th}>Risk</th><th style={th}>Status</th><th style={th}>Command</th>
              </tr>
            </thead>
            <tbody>
              {runbooks.map(r => (
                <tr key={r.id} style={{ borderBottom: '1px solid #1e293b' }}>
                  <td style={td}><strong>{r.name}</strong></td>
                  <td style={td}>{r.title}</td>
                  <td style={td}>{r.risk_level}</td>
                  <td style={td}>{r.status}</td>
                  <td style={td}><code>{r.command_preview}</code></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === 'approvals' && (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #334155' }}>
                <th style={th}>ID</th><th style={th}>Requester</th><th style={th}>Reason</th><th style={th}>Status</th><th style={th}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {approvals.filter(a => a.status === 'pending').map(a => (
                <tr key={a.id} style={{ borderBottom: '1px solid #1e293b' }}>
                  <td style={td}><code>{a.id}</code></td>
                  <td style={td}>{a.requester_name}</td>
                  <td style={td}>{a.reason}</td>
                  <td style={td}>{a.status}</td>
                  <td style={td}>
                    <button onClick={() => handleApprove(a.id)} style={btnGood}>Approve</button>
                    <button onClick={() => handleDeny(a.id)} style={btnBad}>Deny</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === 'executions' && (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #334155' }}>
                <th style={th}>ID</th><th style={th}>Status</th><th style={th}>Results</th><th style={th}>Command</th><th style={th}>Started</th>
              </tr>
            </thead>
            <tbody>
              {executions.map(e => (
                <tr key={e.id} style={{ borderBottom: '1px solid #1e293b' }}>
                  <td style={td}><code>{e.id}</code></td>
                  <td style={td}>{e.status}</td>
                  <td style={td}>{e.succeeded_count}/{e.failed_count}/{e.target_count}</td>
                  <td style={td}><code>{e.command_preview}</code></td>
                  <td style={td}>{e.requested_at?.slice(0,19)||'-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === 'audit' && (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #334155' }}>
                <th style={th}>Time</th><th style={th}>Actor</th><th style={th}>Action</th><th style={th}>Target</th><th style={th}>Result</th>
              </tr>
            </thead>
            <tbody>
              {audit.map(e => (
                <tr key={e.id} style={{ borderBottom: '1px solid #1e293b' }}>
                  <td style={td}>{e.created_at?.slice(0,19)}</td>
                  <td style={td}>{e.actor_id}</td>
                  <td style={td}>{e.action}</td>
                  <td style={td}>{e.target_type}{e.target_id?':'+e.target_id:''}</td>
                  <td style={td}>{e.result}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </main>
    </div>
  );
}

const th: React.CSSProperties = { textAlign: 'left', padding: '8px 12px', color: '#94a3b8', fontSize: 13, fontWeight: 600 };
const td: React.CSSProperties = { padding: '8px 12px', fontSize: 14 };
const btnGood: React.CSSProperties = { padding: '4px 12px', margin: '0 4px', background: '#166534', color: '#86efac', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12 };
const btnBad: React.CSSProperties = { padding: '4px 12px', margin: '0 4px', background: '#7f1d1d', color: '#fca5a5', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12 };
