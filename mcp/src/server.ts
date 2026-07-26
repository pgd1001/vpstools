import {McpServer} from '@modelcontextprotocol/sdk/server/mcp.js';
import {StdioServerTransport} from '@modelcontextprotocol/sdk/server/stdio.js';
import {z} from 'zod';

const apiURL = (process.env.VPS_API_URL || 'http://localhost:8080').replace(/\/$/, '');
const user = process.env.VPS_USER || '';
const writesEnabled = process.env.VPS_MCP_ALLOW_WRITES === 'true';

type JSONValue = string | number | boolean | null | JSONValue[] | {[key: string]: JSONValue};

class VPSClient {
  private headers(): Record<string, string> {
    const headers: Record<string, string> = {'content-type': 'application/json', 'x-vps-user': user};
    const sharedSecret = process.env.VPS_WEB_SHARED_SECRET;
    const subject = process.env.VPS_OIDC_SUBJECT;
    const email = process.env.VPS_OIDC_EMAIL;
    if (sharedSecret && subject && email) {
      headers['x-vps-internal-secret'] = sharedSecret;
      headers['x-vps-oidc-subject'] = subject;
      headers['x-vps-oidc-email'] = email;
    }
    return headers;
  }

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(`${apiURL}${path}`, { ...init, headers: {...this.headers(), ...(init.headers || {})} });
    const text = await response.text();
    let body: JSONValue = {};
    try { body = text ? JSON.parse(text) as JSONValue : {}; } catch { body = {error: text}; }
    if (!response.ok) {
      const error = typeof body === 'object' && body !== null && 'error' in body ? String(body.error) : `API request failed (${response.status})`;
      throw new Error(error);
    }
    return body as T;
  }

  get<T>(path: string) { return this.request<T>(path); }
  post<T>(path: string, body: unknown) { return this.request<T>(path, {method: 'POST', body: JSON.stringify(body)}); }
  delete<T>(path: string) { return this.request<T>(path, {method: 'DELETE'}); }
}

const client = new VPSClient();
const server = new McpServer({name: 'svrtools', version: '0.1.0'});

const textResult = (value: unknown) => ({content: [{type: 'text' as const, text: JSON.stringify(value, null, 2)}]});
const errorResult = (error: unknown) => ({isError: true, content: [{type: 'text' as const, text: error instanceof Error ? error.message : String(error)}]});

async function safe<T>(operation: () => Promise<T>) {
  try { return textResult(await operation()); } catch (error) { return errorResult(error); }
}

function requireWriteConfirmation(confirm: boolean) {
  if (!writesEnabled) throw new Error('Write tools are disabled. Set VPS_MCP_ALLOW_WRITES=true after reviewing the agent policy.');
  if (!confirm) throw new Error('This operation changes VPS Tools state. Re-run with confirm=true after explicit user confirmation.');
}

server.registerTool('vps_health', {description: 'Read the VPS Tools API health and active deployment tier.', inputSchema: {}}, async () => safe(() => client.get('/api/v1/health')));

server.registerTool('vps_whoami', {description: 'Read the current VPS Tools identity, organisation, and role.', inputSchema: {}}, async () => safe(() => client.get('/api/v1/whoami')));

server.registerTool('vps_list_servers', {description: 'List servers visible to the current actor. Filter by environment or tag when useful.', inputSchema: {environment: z.string().optional(), tag_key: z.string().optional(), tag_value: z.string().optional()}}, async ({environment, tag_key, tag_value}) => {
  const query = new URLSearchParams();
  if (environment) query.set('environment', environment);
  if (tag_key) query.set('tag_key', tag_key);
  if (tag_value) query.set('tag_value', tag_value);
  return safe(() => client.get(`/api/v1/servers${query.size ? `?${query}` : ''}`));
});

server.registerTool('vps_list_runbooks', {description: 'List published and available runbooks. Use this before proposing an operational task.', inputSchema: {search: z.string().optional()}}, async ({search}) => safe(() => client.get(`/api/v1/runbooks${search ? `?search=${encodeURIComponent(search)}` : ''}`)));

server.registerTool('vps_get_runbook', {description: 'Read one runbook definition, risk level, version, and permitted roles.', inputSchema: {name: z.string().min(1)}}, async ({name}) => safe(() => client.get(`/api/v1/runbooks/${encodeURIComponent(name)}`)));

const runbookRequest = {name: z.string().min(1), target: z.string().min(1), reason: z.string().optional(), params: z.record(z.string()).optional()};
server.registerTool('vps_preflight_runbook', {description: 'Run a read-only preflight. This never queues work. Use it before any execution request.', inputSchema: runbookRequest}, async (input) => safe(() => client.post(`/api/v1/runbooks/${encodeURIComponent(input.name)}/run`, {target: input.target, reason: input.reason || '', params: input.params || {}, dry_run: true})));

server.registerTool('vps_execute_runbook', {description: 'Request a runbook execution after explicit user confirmation. Always preflight first. High-risk work returns an approval request instead of bypassing approval.', inputSchema: {...runbookRequest, confirm: z.boolean().default(false)}}, async (input) => safe(async () => {
  requireWriteConfirmation(input.confirm);
  const preflight = await client.post<Record<string, JSONValue>>(`/api/v1/runbooks/${encodeURIComponent(input.name)}/run`, {target: input.target, reason: input.reason || '', params: input.params || {}, dry_run: true});
  return {preflight, result: await client.post(`/api/v1/runbooks/${encodeURIComponent(input.name)}/run`, {target: input.target, reason: input.reason || '', params: input.params || {}})};
}));

server.registerTool('vps_list_approvals', {description: 'List approval requests visible to the current actor.', inputSchema: {status: z.string().optional()}}, async ({status}) => safe(() => client.get(`/api/v1/approvals${status ? `?status=${encodeURIComponent(status)}` : ''}`)));

server.registerTool('vps_get_approval', {description: 'Read an approval brief, including the target snapshot and redacted request payload.', inputSchema: {id: z.string().min(1)}}, async ({id}) => safe(() => client.get(`/api/v1/approvals/${encodeURIComponent(id)}`)));

server.registerTool('vps_approve_request', {description: 'Approve a request only after the user explicitly confirms the approval decision. A note is required for traceability.', inputSchema: {id: z.string().min(1), note: z.string().min(1), confirm: z.boolean().default(false)}}, async ({id, note, confirm}) => safe(async () => { requireWriteConfirmation(confirm); return client.post(`/api/v1/approvals/${encodeURIComponent(id)}/approve`, {note}); }));

server.registerTool('vps_deny_request', {description: 'Deny a request only after the user explicitly confirms the decision. A reason is required.', inputSchema: {id: z.string().min(1), note: z.string().min(1), confirm: z.boolean().default(false)}}, async ({id, note, confirm}) => safe(async () => { requireWriteConfirmation(confirm); return client.post(`/api/v1/approvals/${encodeURIComponent(id)}/deny`, {note}); }));

server.registerTool('vps_list_executions', {description: 'List execution state and progress for the current actor.', inputSchema: {status: z.string().optional(), limit: z.number().int().min(1).max(100).default(50)}}, async ({status, limit}) => safe(() => client.get(`/api/v1/executions?limit=${limit}${status ? `&status=${encodeURIComponent(status)}` : ''}`)));

server.registerTool('vps_get_execution', {description: 'Read an execution timeline, target results, output, and audit-linked state.', inputSchema: {id: z.string().min(1)}}, async ({id}) => safe(() => client.get(`/api/v1/executions/${encodeURIComponent(id)}`)));

server.registerTool('vps_search_audit', {description: 'Search recent audit events. Use this to explain who requested, approved, or executed an action.', inputSchema: {limit: z.number().int().min(1).max(100).default(50), actor: z.string().optional()}}, async ({limit, actor}) => safe(() => client.get(`/api/v1/audit?limit=${limit}${actor ? `&actor=${encodeURIComponent(actor)}` : ''}`)));

server.registerTool('vps_list_schedules', {description: 'List interval schedules and their last errors. This is read-only.', inputSchema: {}}, async () => safe(() => client.get('/api/v1/schedules')));

server.registerTool('vps_create_schedule', {description: 'Create an interval schedule after explicit user confirmation. High and critical risk runbooks remain blocked from unattended execution.', inputSchema: {name: z.string().min(1), runbook_name: z.string().min(1), target: z.string().min(1), reason: z.string().min(1), interval_seconds: z.number().int().min(60), params: z.record(z.string()).default({}), next_run_at: z.string().optional(), confirm: z.boolean().default(false)}}, async (input) => safe(async () => { requireWriteConfirmation(input.confirm); return client.post('/api/v1/schedules', input); }));

server.registerTool('vps_disable_schedule', {description: 'Disable an interval schedule after explicit user confirmation. This preserves its audit history.', inputSchema: {id: z.string().min(1), confirm: z.boolean().default(false)}}, async ({id, confirm}) => safe(async () => { requireWriteConfirmation(confirm); return client.delete(`/api/v1/schedules/${encodeURIComponent(id)}`); }));

const transport = new StdioServerTransport();
await server.connect(transport);
