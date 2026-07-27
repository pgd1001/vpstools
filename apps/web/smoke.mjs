import assert from 'node:assert/strict';
import { chromium } from 'playwright';

const baseURL = process.env.WEB_BASE_URL || 'http://127.0.0.1:3000';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
const consoleErrors = [];
const pageErrors = [];
let approvalStatus = 'pending';
let executionStatus = 'queued';
let devIdentitySwitched = false;

page.on('console', message => {
  if (message.type() === 'error') consoleErrors.push(message.text());
});
page.on('pageerror', error => pageErrors.push(error.message));

const runbook = {
  id: 'rb_smoke',
  name: 'check-nginx',
  title: 'Check Nginx',
  status: 'published',
  risk_level: 'low',
  command_preview: 'systemctl status nginx',
  created_at: '2026-01-01T00:00:00Z',
  permitted: true,
};

const approval = {
  id: 'apr_smoke',
  requester_name: 'Junior Engineer',
  action_type: 'runbook',
  status: 'pending',
  risk_level: 'high',
  reason: 'planned maintenance',
  target_type: 'server',
  target_id: 'srv_demo',
  created_at: '2026-01-01T00:00:00Z',
  expires_at: '2026-01-01T01:00:00Z',
  target_snapshot: '[{"id":"srv_demo","environment":"production"}]',
  request_payload: { rollback: 'systemctl start nginx', verification: 'systemctl is-active nginx' },
};

const execution = {
  id: 'exec_smoke',
  actor_user_id: 'user_junior',
  status: 'queued',
  command_preview: 'systemctl status nginx',
  target_count: 1,
  succeeded_count: 0,
  failed_count: 0,
  requested_at: '2026-01-01T00:00:00Z',
};

await page.route('**/api/proxy/**', async route => {
  const url = new URL(route.request().url());
  const path = url.pathname.replace('/api/proxy', '');
  const user = route.request().headers()['x-vps-user'] || 'user_senior';
  const role = user === 'user_junior' ? 'junior' : user === 'user_auditor' ? 'auditor' : 'senior_engineer';
  let body;

  if (path === '/api/v1/health') body = { status: 'ok', database: 'ok', deployment_tier: 'self-contained' };
  else if (path === '/api/v1/ready') body = { status: 'ready', database: 'ok', artifacts: 'ok' };
  else if (path === '/api/v1/whoami') body = { user_id: user, email: `${user}@example.test`, role };
  else if (path === '/api/v1/runbooks/check-nginx') body = { runbook: { ...runbook, definition_json: JSON.stringify({ spec: { parameters: [] } }) } };
  else if (path === '/api/v1/runbooks/check-nginx/run' && route.request().method() === 'POST') body = { status: 'preflight', approval_required: false, target_count: 1 };
  else if (path === '/api/v1/runbooks') body = { runbooks: [runbook] };
  else if (path === '/api/v1/servers') body = { servers: [] };
  else if (path === '/api/v1/runners') body = { runners: [] };
  else if (path === '/api/v1/schedules') body = { schedules: [] };
  else if (path === '/api/v1/automation/status') body = { paused: false };
  else if (path === '/api/v1/approvals/apr_smoke' && route.request().method() === 'GET') body = { approval: { ...approval, status: approvalStatus } };
  else if (path === '/api/v1/approvals/apr_smoke/deny' && route.request().method() === 'POST') { approvalStatus = 'denied'; body = { status: approvalStatus }; }
  else if (path === '/api/v1/approvals') body = { approvals: approvalStatus === 'pending' ? [{ ...approval, status: approvalStatus }] : [] };
  else if (path === '/api/v1/executions/exec_smoke' && route.request().method() === 'GET') body = { execution: { ...execution, status: executionStatus, events: [{ id: 'evt_smoke', target_id: 'tgt_smoke', from_status: 'created', to_status: executionStatus, event_type: 'status_changed', metadata: '{}', occurred_at: '2026-01-01T00:00:00Z' }], targets: [{ id: 'tgt_smoke', server_id: 'srv_demo', server_name: 'Demo', status: executionStatus, stdout: '', stderr: '', exit_code: 0 }] } };
  else if (path === '/api/v1/executions/exec_smoke/cancel' && route.request().method() === 'POST') { executionStatus = 'canceled'; body = { status: executionStatus }; }
  else if (path === '/api/v1/executions') body = { executions: [{ ...execution, status: executionStatus }] };
  else if (path === '/api/v1/audit') body = { events: [] };
  else body = {};

  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
});

try {
  await page.goto(baseURL, { waitUntil: 'domcontentloaded' });
  await page.getByRole('heading', { name: 'VPS Tools Console' }).waitFor();
  await page.getByRole('status', { name: 'Control plane status' }).filter({ hasText: 'Control plane ready' }).waitFor();
  await page.waitForTimeout(1000);
  assert.equal(await page.getByRole('button', { name: 'Tasks and runbooks' }).getAttribute('aria-current'), 'page');

  const navigation = ['Servers', 'Runners', 'Tasks and runbooks', 'Schedules', 'Approvals', 'Executions', 'Audit'];
  for (const name of navigation) {
    const button = page.getByRole('button', { name, exact: true });
    await button.click();
    await page.waitForTimeout(250);
    const navState = await page.locator('nav button').evaluateAll(buttons => buttons.map(candidate => ({ text: candidate.textContent?.trim(), current: candidate.getAttribute('aria-current') })));
    assert.equal(await button.getAttribute('aria-current'), 'page', `Primary navigation did not select ${name}: ${JSON.stringify(navState)} errors=${JSON.stringify(pageErrors)} console=${JSON.stringify(consoleErrors)}`);
  }

  await page.getByRole('button', { name: 'Tasks and runbooks', exact: true }).click();
  const search = page.getByRole('textbox', { name: 'Search runbooks' });
  await search.fill('nginx');
  const searchRequest = page.waitForRequest(request => request.url().includes('/api/v1/runbooks?search=nginx'));
  await page.getByRole('button', { name: 'Search', exact: true }).click();
  await searchRequest;
  assert.equal(await search.inputValue(), 'nginx');

  const runTaskButton = page.getByRole('button', { name: 'Run task', exact: true });
  await runTaskButton.click();
  await page.getByRole('heading', { name: 'Run task: check-nginx' }).waitFor();
  await page.getByLabel('Target').fill('server:srv_demo');
  await page.getByLabel('Reason').fill('browser smoke preflight');
  const preflightRequest = page.waitForRequest(request => request.url().includes('/api/v1/runbooks/check-nginx/run') && request.method() === 'POST');
  await page.getByRole('button', { name: 'Preview', exact: true }).click();
  await preflightRequest;
  await page.getByText('Preflight passed. Ready to run. Target count: 1.').waitFor();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();

  await page.getByRole('button', { name: 'Approvals', exact: true }).click();
  await page.getByRole('button', { name: 'apr_smoke', exact: true }).click();
  await page.getByRole('heading', { name: 'Approval brief, apr_smoke' }).waitFor();
  await page.getByText(/production/).waitFor();
  await page.getByLabel('Decision note').fill('Denied by browser smoke');
  const denialRequest = page.waitForRequest(request => request.url().includes('/api/v1/approvals/apr_smoke/deny') && request.method() === 'POST');
  await page.getByRole('button', { name: 'Deny with reason', exact: true }).click();
  await denialRequest;
  await page.getByText('Saved.').waitFor();

  if (process.env.EXPECT_DEV_AUTH === 'true') {
    const userSelector = page.getByRole('combobox', { name: 'Development user' });
    const userRequest = page.waitForRequest(request => request.url().includes('/api/v1/whoami') && request.headers()['x-vps-user'] === 'user_junior');
    await userSelector.selectOption('user_junior');
    await userRequest;
    devIdentitySwitched = true;
  }

  await page.getByRole('button', { name: 'Executions', exact: true }).click();
  await page.getByRole('button', { name: 'exec_smoke', exact: true }).click();
  await page.getByRole('button', { name: 'Cancel execution', exact: true }).waitFor();
  page.once('dialog', dialog => dialog.accept());
  const cancellationRequest = page.waitForRequest(request => request.url().includes('/api/v1/executions/exec_smoke/cancel') && request.method() === 'POST');
  await page.getByRole('button', { name: 'Cancel execution', exact: true }).click();
  await cancellationRequest;
  await page.getByText('Saved.').waitFor();

  if (process.env.EXPECT_DEV_AUTH === 'true') {
    const userSelector = page.getByRole('combobox', { name: 'Development user' });
    await userSelector.waitFor();
    if (!devIdentitySwitched) throw new Error('development identity switch was not exercised before requester cancellation');
    assert.equal(await userSelector.inputValue(), 'user_junior');
  } else {
    await page.getByRole('link', { name: 'Sign in' }).waitFor();
  }

  assert.deepEqual(consoleErrors, [], `Browser console errors: ${consoleErrors.join('; ')}`);
  assert.deepEqual(pageErrors, [], `Page errors: ${pageErrors.join('; ')}`);
  console.log(`Web UI smoke passed at ${baseURL}`);
} finally {
  await browser.close();
}
