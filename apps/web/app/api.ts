const API = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export type Server = {
  id: string; name: string; hostname: string; environment: string;
  status: string; os_name: string; os_version: string; tags: {key:string;value:string}[];
  last_seen_at: string; created_at: string;
};

export type Runbook = {
  id: string; name: string; title: string; status: string;
  risk_level: string; command_preview: string; created_at: string;
};

export type Approval = {
  id: string; requester_name: string; action_type: string;
  status: string; risk_level: string; reason: string;
  target_type: string; target_id: string; created_at: string;
};

export type AuditEvent = {
  id: string; action: string; target_type: string; target_id: string;
  result: string; created_at: string; actor_id: string;
};

export type Execution = {
  id: string; status: string; command_preview: string;
  target_count: number; succeeded_count: number; failed_count: number;
  requested_at: string; finished_at: string;
};

let user = 'user_senior';
if (typeof window !== 'undefined') {
  user = localStorage.getItem('vps_user') || 'user_senior';
}

function headers(): Record<string, string> {
  return { 'X-VPS-User': user, 'Content-Type': 'application/json' };
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(API + path, { headers: headers() });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(API + path, {
    method: 'POST', headers: headers(),
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export const api = {
  whoami: () => get<{user_id:string;email:string;role:string}>('/api/v1/whoami'),
  servers: () => get<{servers:Server[]}>('/api/v1/servers'),
  runbooks: () => get<{runbooks:Runbook[]}>('/api/v1/runbooks'),
  approvals: () => get<{approvals:Approval[]}>('/api/v1/approvals'),
  executions: () => get<{executions:Execution[]}>('/api/v1/executions'),
  audit: (actor?:string) => get<{events:AuditEvent[]}>(`/api/v1/audit?limit=50${actor?`&actor=${actor}`:''}`),
  approve: (id:string) => post(`/api/v1/approvals/${id}/approve`),
  deny: (id:string) => post(`/api/v1/approvals/${id}/deny`),
  setUser: (u:string) => { user = u; if (typeof window !== 'undefined') localStorage.setItem('vps_user', u); },
  getUser: () => user,
};
