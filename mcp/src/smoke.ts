import {Client} from '@modelcontextprotocol/sdk/client/index.js';
import {StdioClientTransport} from '@modelcontextprotocol/sdk/client/stdio.js';

const command = process.platform === 'win32' ? 'node_modules/.bin/tsx.cmd' : 'node_modules/.bin/tsx';
const transport = new StdioClientTransport({command, args: ['src/server.ts']});
const client = new Client({name: 'svrtools-mcp-smoke', version: '0.1.0'});

await client.connect(transport);
const tools = await client.listTools();
const names = tools.tools.map((tool) => tool.name);
const required = ['vps_health', 'vps_preflight_runbook', 'vps_execute_runbook', 'vps_get_execution', 'vps_search_audit'];
for (const name of required) {
  if (!names.includes(name)) throw new Error(`missing MCP tool: ${name}`);
}
console.log(`MCP smoke test passed with ${names.length} tools`);
await transport.close();
