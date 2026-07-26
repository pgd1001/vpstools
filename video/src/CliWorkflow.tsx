import React from 'react';
import {AbsoluteFill, Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';

type TerminalLine = {
  at: number;
  text: string;
  tone?: 'command' | 'success' | 'warning' | 'muted' | 'plain';
};

const lines: TerminalLine[] = [
  {at: 22, text: '$ vps runbook run restart-nginx --target server:web-prod', tone: 'command'},
  {at: 48, text: '  reason: deploy v2.3', tone: 'muted'},
  {at: 72, text: '  preflight  checking target, policy, parameters', tone: 'muted'},
  {at: 98, text: '  ✓ target web-prod  |  production  |  1 server', tone: 'success'},
  {at: 120, text: '  ✓ runbook v3  |  risk high  |  timeout 300s', tone: 'success'},
  {at: 142, text: '  ! approval required  apr_7f31c2', tone: 'warning'},
  {at: 170, text: '$ vps approvals approve apr_7f31c2 --note "Reviewed change window"', tone: 'command'},
  {at: 198, text: '  ✓ approval recorded  |  audit event aud_93c1', tone: 'success'},
  {at: 222, text: '  ✓ queued execution  exe_4b82aa', tone: 'success'},
  {at: 250, text: '  runner rnr_local claimed target  [lease 30s]', tone: 'plain'},
  {at: 278, text: '  [1/1] web-prod  running  systemctl restart nginx', tone: 'plain'},
  {at: 314, text: '  [1/1] web-prod  ✓ succeeded  842ms', tone: 'success'},
  {at: 344, text: '  ✓ verification passed  |  service active (running)', tone: 'success'},
  {at: 370, text: '  audit trail complete  |  output redacted and stored', tone: 'success'},
];

const phaseCards = [
  {from: 10, to: 155, step: '01', title: 'Preflight first', detail: 'Policy, scope and risk are checked before anything runs.'},
  {from: 155, to: 235, step: '02', title: 'Human approval', detail: 'High-risk production work stops for a recorded decision.'},
  {from: 235, to: 390, step: '03', title: 'Execution with evidence', detail: 'Leases, verification and audit events close the loop.'},
];

const colours = {
  bg: '#07111f',
  panel: '#0d1b2d',
  terminal: '#071019',
  border: '#1e3a52',
  text: '#d8e7ef',
  muted: '#7390a1',
  cyan: '#5eead4',
  blue: '#60a5fa',
  amber: '#fbbf24',
  green: '#86efac',
};

function fadeIn(frame: number, from: number, length = 18) {
  return interpolate(frame, [from, from + length], [0, 1], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
}

function typedText(text: string, frame: number, at: number) {
  const local = Math.max(0, frame - at);
  const chars = Math.floor(interpolate(local, [0, 13], [0, text.length], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.quad)}));
  return text.slice(0, chars);
}

function Terminal({frame}: {frame: number}) {
  const visible = lines.filter((line) => frame >= line.at);
  const start = Math.max(0, visible.length - 10);
  const shown = visible.slice(start);
  return (
    <div style={{position: 'absolute', left: 88, top: 208, width: 1100, height: 690, background: colours.terminal, border: `1px solid ${colours.border}`, borderRadius: 18, boxShadow: '0 30px 80px rgba(0,0,0,.38)', overflow: 'hidden'}}>
      <div style={{height: 56, display: 'flex', alignItems: 'center', padding: '0 22px', borderBottom: `1px solid ${colours.border}`, background: '#0b1726'}}>
        <span style={{width: 12, height: 12, borderRadius: 99, background: '#fb7185', marginRight: 8}} />
        <span style={{width: 12, height: 12, borderRadius: 99, background: '#fbbf24', marginRight: 8}} />
        <span style={{width: 12, height: 12, borderRadius: 99, background: '#4ade80', marginRight: 18}} />
        <span style={{fontFamily: 'Inter, sans-serif', fontSize: 16, color: colours.muted}}>vps-tools  •  operator session</span>
        <span style={{marginLeft: 'auto', fontFamily: 'Inter, sans-serif', fontSize: 13, color: '#466277'}}>self-contained / local</span>
      </div>
      <div style={{padding: '28px 32px', fontFamily: 'JetBrains Mono, Consolas, monospace', fontSize: 20, lineHeight: 1.65}}>
        {shown.map((line) => {
          const isLatest = line === visible[visible.length - 1];
          const text = isLatest ? typedText(line.text, frame, line.at) : line.text;
          const color = line.tone === 'command' ? colours.blue : line.tone === 'success' ? colours.green : line.tone === 'warning' ? colours.amber : line.tone === 'muted' ? colours.muted : colours.text;
          return <div key={`${line.at}-${line.text}`} style={{height: 33, color, opacity: fadeIn(frame, line.at)}}>{text}{isLatest && <span style={{display: 'inline-block', width: 11, height: 22, marginLeft: 5, verticalAlign: -3, background: frame % 30 < 15 ? colours.cyan : 'transparent'}} />}</div>;
        })}
      </div>
      <div style={{position: 'absolute', right: 30, bottom: 20, fontFamily: 'Inter, sans-serif', fontSize: 12, color: '#36546a'}}>CTRL+C remains available</div>
    </div>
  );
}

function PhaseCard({frame}: {frame: number}) {
  const phase = phaseCards.find((card) => frame >= card.from && frame < card.to) ?? phaseCards[2];
  const opacity = fadeIn(frame, phase.from, 16);
  const rise = interpolate(frame, [phase.from, phase.from + 16], [12, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
  return <div style={{position: 'absolute', right: 88, top: 286, width: 390, opacity, transform: `translateY(${rise}px)`}}>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 16, letterSpacing: 2, color: colours.cyan, fontWeight: 700}}>STEP {phase.step}</div>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 35, color: colours.text, fontWeight: 700, marginTop: 15, lineHeight: 1.1}}>{phase.title}</div>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 19, color: colours.muted, lineHeight: 1.5, marginTop: 20}}>{phase.detail}</div>
    <div style={{marginTop: 34, height: 4, width: 390, background: '#163049', borderRadius: 99, overflow: 'hidden'}}><div style={{height: '100%', width: `${((frame - phase.from) / (phase.to - phase.from)) * 100}%`, background: colours.cyan, borderRadius: 99}} /></div>
  </div>;
}

export const CliWorkflow: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const intro = spring({frame, fps, config: {damping: 200, stiffness: 100}});
  const titleOpacity = interpolate(intro, [0, 1], [0, 1]);
  const titleY = interpolate(intro, [0, 1], [18, 0]);
  return <AbsoluteFill style={{background: colours.bg, color: colours.text}}>
    <div style={{position: 'absolute', inset: 0, background: 'radial-gradient(circle at 76% 12%, rgba(20,184,166,.14), transparent 30%), linear-gradient(135deg, #07111f 0%, #081a2b 100%)'}} />
    <div style={{position: 'absolute', left: 88, top: 72, right: 88, display: 'flex', alignItems: 'end', opacity: titleOpacity, transform: `translateY(${titleY}px)`}}>
      <div>
        <div style={{fontFamily: 'Inter, sans-serif', fontSize: 17, fontWeight: 700, letterSpacing: 3, color: colours.cyan}}>VPS TOOLS  /  CLI MOTION SAMPLE</div>
        <div style={{fontFamily: 'Inter, sans-serif', fontSize: 44, lineHeight: 1.1, fontWeight: 700, marginTop: 12}}>Safe operations, made routine.</div>
      </div>
      <div style={{marginLeft: 'auto', fontFamily: 'Inter, sans-serif', fontSize: 15, color: colours.muted}}>A guided production change</div>
    </div>
    <Terminal frame={frame} />
    <PhaseCard frame={frame} />
    <div style={{position: 'absolute', left: 88, bottom: 64, right: 88, display: 'flex', alignItems: 'center', fontFamily: 'Inter, sans-serif', fontSize: 14, color: colours.muted}}>
      <span style={{color: colours.green, marginRight: 10}}>●</span> policy enforced at the control plane
      <span style={{marginLeft: 'auto'}}>junior workflow  •  approval  •  runner  •  audit</span>
    </div>
  </AbsoluteFill>;
};
