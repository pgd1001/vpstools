const API = process.env.NEXT_PUBLIC_API_URL || '/api/proxy';

export type Server = {
  id: string; name: string; hostname: string; environment: string;
  public_ip?: string; private_ip?: string; ssh_port?: number; ssh_username?: string; provider?: string;
  status: string; os_name: string; os_version: string; tags: {key:string;value:string}[];
  last_seen_at: string; created_at: string;
};

export type Runner = { id:string; name:string; runner_type:string; status:string; version:string; hostname:string; platform:string; ip_address:string; last_seen_at:string; registered_at:string; revoked_at:string; created_at:string };

export type Runbook = {
  id: string; name: string; title: string; status: string;
  description?: string; risk_level: string; command_preview: string; created_at: string;
  version?: number; command?: string; definition_json?: string; allowed_roles?: string; permitted?: boolean;
};
export type RunbookParameter = { name:string; type?:string; default?:string; allowedValues?:string[]; required?:boolean; description?:string };

export type Approval = {
  id: string; requester_name: string; action_type: string;
  status: string; risk_level: string; reason: string;
  target_type: string; target_id: string; created_at: string;
  expires_at?: string; target_snapshot?: string; request_payload?: string | Record<string, unknown>; decision_note?: string;
};

export type AuditEvent = {
  id: string; action: string; target_type: string; target_id: string;
  result: string; created_at: string; actor_id: string;
};

export type Execution = {
  id: string; status: string; command_preview: string;
  target_count: number; succeeded_count: number; failed_count: number;
  requested_at: string; finished_at: string; actor_user_id?: string;
};

export type ExecutionDetail = Execution & { reason?: string; targets: { id:string; server_id:string; server_name:string; status:string; stdout:string; stderr:string; exit_code:number; started_at:string; finished_at:string }[]; events?: {id:string;target_id:string;from_status:string;to_status:string;event_type:string;metadata:string;occurred_at:string}[] };
export type RunbookRunResponse = { status:string; approval_required?:boolean; target_count?:number; execution_id?:string; approval_id?:string; message?:string };
export type Schedule = { id:string; name:string; runbook_name:string; target:string; reason:string; params:string; interval_seconds:number; next_run_at:string; enabled:boolean; last_run_at:string; last_error:string };
export type AutomationStatus = { paused:boolean; paused_at?:string; paused_by?:string; reason?:string };

let user = '';

function getUser(): string {
  if (user) return user;
  if (typeof window !== 'undefined') {
    user = localStorage.getItem('vps_user') || (process.env.NEXT_PUBLIC_DEV_AUTH === 'true' ? 'user_senior' : '');
  } else {
    user = process.env.NEXT_PUBLIC_DEV_AUTH === 'true' ? 'user_senior' : '';
  }
  return user;
}

function headers(): Record<string, string> {
  const result: Record<string, string> = { 'Content-Type': 'application/json' };
  if (process.env.NEXT_PUBLIC_DEV_AUTH === 'true') result['X-VPS-User'] = getUser();
  return result;
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(API + path, { headers: headers(), credentials: 'include' });
  if (!res.ok) throw new Error(await errorMessage(res));
  return res.json();
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(API + path, {
    method: 'POST', headers: headers(),
    body: body ? JSON.stringify(body) : undefined, credentials: 'include',
  });
  if (!res.ok) throw new Error(await errorMessage(res));
  return res.json();
}

async function mutate<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(API + path, { method, headers: headers(), body: body === undefined ? undefined : JSON.stringify(body), credentials: 'include' });
  if (!res.ok) throw new Error(await errorMessage(res));
  return res.json();
}

async function errorMessage(res: Response): Promise<string> {
  const raw = await res.text();
  try {
    const body = JSON.parse(raw) as { error?: string; message?: string; next?: string };
    return [body.error || body.message || `Request failed (${res.status})`, body.next].filter(Boolean).join(' ');
  } catch {
    return raw || `Request failed (${res.status})`;
  }
}

export const api = {
  whoami: () => get<{user_id:string;email:string;role:string}>('/api/v1/whoami'),
  authMe: () => get<{authenticated:boolean;email?:string;name?:string}>('/api/auth/me'),
  servers: () => get<{servers:Server[]}>('/api/v1/servers'),
  createServer: (body:unknown) => post('/api/v1/servers', body),
  updateServer: (id:string, body:unknown) => mutate('PATCH', `/api/v1/servers/${encodeURIComponent(id)}`, body),
  archiveServer: (id:string) => mutate('DELETE', `/api/v1/servers/${encodeURIComponent(id)}`),
  checkServer: (id:string) => post(`/api/v1/servers/${encodeURIComponent(id)}/check`),
  runners: () => get<{runners:Runner[]}>('/api/v1/runners'),
  createRunner: (body:unknown) => post('/api/v1/runners/manage', body),
  updateRunner: (id:string, body:unknown) => mutate('PATCH', `/api/v1/runners/${encodeURIComponent(id)}`, body),
  revokeRunner: (id:string) => mutate('DELETE', `/api/v1/runners/${encodeURIComponent(id)}`),
  registrationToken: () => post<{token:string;expires_at:string}>('/api/v1/runners/registration-token'),
  rotateRunner: (id:string) => post<{registration_token:string;expires_in:string;runner_id:string}>(`/api/v1/runners/${encodeURIComponent(id)}/rotate-token`),
  runbooks: (search?: string) => get<{runbooks:Runbook[]}>(`/api/v1/runbooks${search ? `?search=${encodeURIComponent(search)}` : ''}`),
  getRunbook: (name:string) => get<{runbook:Runbook}>(`/api/v1/runbooks/${encodeURIComponent(name)}`),
  createRunbook: (body:unknown) => post('/api/v1/runbooks', body),
  updateRunbook: (name:string, body:unknown) => mutate('PUT', `/api/v1/runbooks/${encodeURIComponent(name)}`, body),
  archiveRunbook: (name:string) => mutate('DELETE', `/api/v1/runbooks/${encodeURIComponent(name)}`),
  publishRunbook: (name:string) => post(`/api/v1/runbooks/${encodeURIComponent(name)}/publish`),
  runRunbook: (name:string, body:unknown) => post<RunbookRunResponse>(`/api/v1/runbooks/${encodeURIComponent(name)}/run`, body),
  approvals: () => get<{approvals:Approval[]}>('/api/v1/approvals'),
  getApproval: (id:string) => get<{approval:Approval}>(`/api/v1/approvals/${encodeURIComponent(id)}`),
  executions: () => get<{executions:Execution[]}>('/api/v1/executions'),
  createExecution: (body:unknown) => post('/api/v1/executions', body),
  getExecution: (id:string) => get<{execution:ExecutionDetail}>(`/api/v1/executions/${encodeURIComponent(id)}`),
  schedules: () => get<{schedules:Schedule[]}>('/api/v1/schedules'),
  automationStatus: () => get<AutomationStatus>('/api/v1/automation/status'),
  pauseAutomation: (reason:string) => post<AutomationStatus>('/api/v1/automation/pause', { reason }),
  resumeAutomation: () => post<AutomationStatus>('/api/v1/automation/resume'),
  createSchedule: (body:unknown) => post('/api/v1/schedules', body),
  disableSchedule: (id:string) => mutate('DELETE', `/api/v1/schedules/${encodeURIComponent(id)}`),
  cancelExecution: (id:string) => post(`/api/v1/executions/${encodeURIComponent(id)}/cancel`),
  audit: (actor?:string) => get<{events:AuditEvent[]}>(`/api/v1/audit?limit=50${actor?`&actor=${encodeURIComponent(actor)}`:''}`),
  approve: (id:string, note?:string) => post(`/api/v1/approvals/${encodeURIComponent(id)}/approve`, note ? {note} : undefined),
  deny: (id:string, note?:string) => post(`/api/v1/approvals/${encodeURIComponent(id)}/deny`, note ? {note} : undefined),
  setUser: (u:string) => { user = u; if (typeof window !== 'undefined') localStorage.setItem('vps_user', u); },
  getUser: () => getUser(),
};
