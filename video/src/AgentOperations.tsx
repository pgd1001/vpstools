import React from 'react';
import {AbsoluteFill, Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';

type LogLine = {at: number; text: string; tone?: 'command' | 'success' | 'info' | 'muted' | 'warning'};

type AgentConfig = {
  label: string;
  eyebrow: string;
  title: string;
  subtitle: string;
  session: string;
  toolLabel: string;
  footer: string;
  logs: LogLine[];
  cards: {from: number; to: number; number: string; title: string; detail: string}[];
};

const claude: AgentConfig = {
  label: 'CLAUDE CLI',
  eyebrow: 'VPS TOOLS  /  MCP OPERATOR FLOW',
  title: 'Ask Claude. Run safely.',
  subtitle: 'An MCP-connected operations workflow',
  session: 'claude  •  mcp session',
  toolLabel: 'MCP SERVER  /  VPS TOOLS',
  footer: 'MCP  •  preflight  •  approval  •  audit',
  logs: [
    {at: 20, text: '$ claude', tone: 'command'},
    {at: 48, text: '  > Use VPS Tools to check web-prod before the restart.', tone: 'info'},
    {at: 78, text: '  tool  vps.list_servers  →  web-prod  [production]', tone: 'muted'},
    {at: 108, text: '  tool  vps.runbook.preview  →  restart-nginx v3', tone: 'muted'},
    {at: 140, text: '  ✓ policy preflight passed  |  risk high', tone: 'success'},
    {at: 168, text: '  ! approval required  apr_7f31c2', tone: 'warning'},
    {at: 196, text: '  > Approval recorded. Execute the runbook.', tone: 'info'},
    {at: 228, text: '  tool  vps.runbook.execute  →  exe_4b82aa', tone: 'muted'},
    {at: 260, text: '  runner  claimed  |  web-prod  |  lease 30s', tone: 'muted'},
    {at: 292, text: '  ✓ verification passed  |  nginx active', tone: 'success'},
    {at: 324, text: '  ✓ audit event stored  |  output redacted', tone: 'success'},
    {at: 356, text: '  completed safely in 842ms', tone: 'success'},
  ],
  cards: [
    {from: 8, to: 130, number: '01', title: 'Natural language', detail: 'Describe the operational goal in the agent session.'},
    {from: 130, to: 245, number: '02', title: 'MCP tools', detail: 'Claude discovers servers, previews the runbook and checks policy.'},
    {from: 245, to: 390, number: '03', title: 'Evidence by default', detail: 'Approval, execution, verification and audit stay connected.'},
  ],
};

const codex: AgentConfig = {
  label: 'CHATGPT CODEX',
  eyebrow: 'VPS TOOLS  /  SKILLS + MCP WORKFLOW',
  title: 'Plan first. Operate with context.',
  subtitle: 'Codex uses skills and MCP to work through the change',
  session: 'codex  •  project session',
  toolLabel: 'SKILL  /  VPS OPERATOR',
  footer: 'skills  •  MCP  •  scoped plan  •  audit',
  logs: [
    {at: 20, text: '$ codex', tone: 'command'},
    {at: 48, text: '  > Restart nginx on web-prod after checking the policy.', tone: 'info'},
    {at: 78, text: '  skill  vps-operator  →  loaded for this task', tone: 'muted'},
    {at: 108, text: '  mcp    vps-tools     →  connected to control plane', tone: 'muted'},
    {at: 140, text: '  plan   inspect → preview → approve → execute', tone: 'info'},
    {at: 168, text: '  ✓ scope confirmed  |  1 production server', tone: 'success'},
    {at: 196, text: '  ! approval required  apr_7f31c2', tone: 'warning'},
    {at: 224, text: '  > Plan paused until approval is recorded.', tone: 'info'},
    {at: 256, text: '  mcp    execute runbook  →  exe_4b82aa', tone: 'muted'},
    {at: 288, text: '  runner  claimed target  |  lease 30s', tone: 'muted'},
    {at: 320, text: '  ✓ verification passed  |  service active', tone: 'success'},
    {at: 352, text: '  ✓ audit trail complete  |  task finished', tone: 'success'},
  ],
  cards: [
    {from: 8, to: 130, number: '01', title: 'Load the skill', detail: 'Codex starts with the project rules and operating context.'},
    {from: 130, to: 245, number: '02', title: 'Make a plan', detail: 'The task is scoped before MCP tools are allowed to act.'},
    {from: 245, to: 390, number: '03', title: 'Close the loop', detail: 'The runner executes, verifies and records the result.'},
  ],
};

const colours = {bg: '#07111f', terminal: '#071019', border: '#1e3a52', text: '#d8e7ef', muted: '#7390a1', cyan: '#5eead4', blue: '#60a5fa', amber: '#fbbf24', green: '#86efac'};

function fade(frame: number, from: number, length = 16) {
  return interpolate(frame, [from, from + length], [0, 1], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
}

function typedText(text: string, frame: number, at: number) {
  const chars = Math.floor(interpolate(Math.max(0, frame - at), [0, 13], [0, text.length], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.quad)}));
  return text.slice(0, chars);
}

function AgentTerminal({frame, config}: {frame: number; config: AgentConfig}) {
  const visible = config.logs.filter((line) => frame >= line.at);
  const shown = visible.slice(Math.max(0, visible.length - 10));
  return <div style={{position: 'absolute', left: 88, top: 208, width: 1100, height: 690, background: colours.terminal, border: `1px solid ${colours.border}`, borderRadius: 18, boxShadow: '0 30px 80px rgba(0,0,0,.38)', overflow: 'hidden'}}>
    <div style={{height: 56, display: 'flex', alignItems: 'center', padding: '0 22px', borderBottom: `1px solid ${colours.border}`, background: '#0b1726'}}>
      <span style={{width: 12, height: 12, borderRadius: 99, background: '#fb7185', marginRight: 8}} />
      <span style={{width: 12, height: 12, borderRadius: 99, background: '#fbbf24', marginRight: 8}} />
      <span style={{width: 12, height: 12, borderRadius: 99, background: '#4ade80', marginRight: 18}} />
      <span style={{fontFamily: 'Inter, sans-serif', fontSize: 16, color: colours.muted}}>{config.session}</span>
      <span style={{marginLeft: 'auto', fontFamily: 'Inter, sans-serif', fontSize: 13, color: '#466277'}}>{config.toolLabel}</span>
    </div>
    <div style={{padding: '28px 32px', fontFamily: 'JetBrains Mono, Consolas, monospace', fontSize: 20, lineHeight: 1.65}}>
      {shown.map((line) => {
        const latest = line === visible[visible.length - 1];
        const text = latest ? typedText(line.text, frame, line.at) : line.text;
        const colour = line.tone === 'command' ? colours.blue : line.tone === 'success' ? colours.green : line.tone === 'warning' ? colours.amber : line.tone === 'info' ? colours.cyan : line.tone === 'muted' ? colours.muted : colours.text;
        return <div key={`${line.at}-${line.text}`} style={{height: 33, color: colour, opacity: fade(frame, line.at)}}>{text}{latest && <span style={{display: 'inline-block', width: 11, height: 22, marginLeft: 5, verticalAlign: -3, background: frame % 30 < 15 ? colours.cyan : 'transparent'}} />}</div>;
      })}
    </div>
    <div style={{position: 'absolute', right: 30, bottom: 20, fontFamily: 'Inter, sans-serif', fontSize: 12, color: '#36546a'}}>policy remains at the control plane</div>
  </div>;
}

function AgentCard({frame, config}: {frame: number; config: AgentConfig}) {
  const card = config.cards.find((item) => frame >= item.from && frame < item.to) ?? config.cards[2];
  const opacity = fade(frame, card.from);
  const y = interpolate(frame, [card.from, card.from + 16], [12, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
  const progress = Math.min(100, Math.max(0, ((frame - card.from) / (card.to - card.from)) * 100));
  return <div style={{position: 'absolute', right: 88, top: 286, width: 390, opacity, transform: `translateY(${y}px)`}}>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 16, letterSpacing: 2, color: colours.cyan, fontWeight: 700}}>STEP {card.number}</div>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 35, color: colours.text, fontWeight: 700, marginTop: 15, lineHeight: 1.1}}>{card.title}</div>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 19, color: colours.muted, lineHeight: 1.5, marginTop: 20}}>{card.detail}</div>
    <div style={{marginTop: 34, height: 4, width: 390, background: '#163049', borderRadius: 99, overflow: 'hidden'}}><div style={{height: '100%', width: `${progress}%`, background: colours.cyan, borderRadius: 99}} /></div>
  </div>;
}

export const AgentOperations: React.FC<{config: AgentConfig}> = ({config}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const intro = spring({frame, fps, config: {damping: 200, stiffness: 100}});
  const opacity = interpolate(intro, [0, 1], [0, 1]);
  const y = interpolate(intro, [0, 1], [18, 0]);
  return <AbsoluteFill style={{background: colours.bg, color: colours.text}}>
    <div style={{position: 'absolute', inset: 0, background: 'radial-gradient(circle at 76% 12%, rgba(20,184,166,.14), transparent 30%), linear-gradient(135deg, #07111f 0%, #081a2b 100%)'}} />
    <div style={{position: 'absolute', left: 88, top: 72, right: 88, display: 'flex', alignItems: 'end', opacity, transform: `translateY(${y}px)`}}>
      <div>
        <div style={{fontFamily: 'Inter, sans-serif', fontSize: 17, fontWeight: 700, letterSpacing: 3, color: colours.cyan}}>{config.eyebrow}</div>
        <div style={{fontFamily: 'Inter, sans-serif', fontSize: 44, lineHeight: 1.1, fontWeight: 700, marginTop: 12}}>{config.title}</div>
      </div>
      <div style={{marginLeft: 'auto', fontFamily: 'Inter, sans-serif', fontSize: 15, color: colours.muted}}>{config.subtitle}</div>
    </div>
    <AgentTerminal frame={frame} config={config} />
    <AgentCard frame={frame} config={config} />
    <div style={{position: 'absolute', left: 88, bottom: 64, right: 88, display: 'flex', alignItems: 'center', fontFamily: 'Inter, sans-serif', fontSize: 14, color: colours.muted}}>
      <span style={{color: colours.green, marginRight: 10}}>●</span> {config.footer}
      <span style={{marginLeft: 'auto'}}>controlled agent operations</span>
    </div>
  </AbsoluteFill>;
};

export const ClaudeCliWorkflow: React.FC = () => <AgentOperations config={claude} />;
export const CodexWorkflow: React.FC = () => <AgentOperations config={codex} />;
