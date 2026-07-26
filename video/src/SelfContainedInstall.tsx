import React from 'react';
import {AbsoluteFill, Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';

type LogLine = {at: number; text: string; tone?: 'command' | 'success' | 'info' | 'muted' | 'warning'};

const logs: LogLine[] = [
  {at: 20, text: '$ ./bin/api.exe', tone: 'command'},
  {at: 48, text: '  VPS Tools API  |  starting self-contained tier', tone: 'info'},
  {at: 74, text: '  ✓ configuration validated', tone: 'success'},
  {at: 98, text: '  ✓ SQLite opened  |  WAL mode  |  foreign keys on', tone: 'success'},
  {at: 122, text: '  ✓ database schema migrated  |  18 tables', tone: 'success'},
  {at: 146, text: '  ✓ local artefacts  |  encrypted  |  ./data/artifacts', tone: 'success'},
  {at: 170, text: '  + generated ./data/artifacts/.key  (service account only)', tone: 'muted'},
  {at: 198, text: '  ✓ embedded scheduler  |  database polling enabled', tone: 'success'},
  {at: 226, text: '  ✓ backup manifest ready  |  make backup', tone: 'success'},
  {at: 256, text: '  API listening on http://localhost:8080', tone: 'info'},
  {at: 286, text: '$ ./bin/runner.exe', tone: 'command'},
  {at: 314, text: '  ✓ runner rnr_local connected  |  SIMULATE=true', tone: 'success'},
  {at: 344, text: '$ vps health', tone: 'command'},
  {at: 370, text: '  ✓ ready  |  self-contained  |  no external services', tone: 'success'},
];

const cards = [
  {from: 8, to: 130, number: '01', title: 'One command to start', detail: 'No Docker, PostgreSQL, S3 or NATS required for a small deployment.'},
  {from: 130, to: 245, number: '02', title: 'Data stays local', detail: 'SQLite metadata and encrypted artefacts are created beside the service.'},
  {from: 245, to: 390, number: '03', title: 'Ready to operate', detail: 'The embedded scheduler and local runner are ready for the first task.'},
];

const c = {bg: '#06111d', panel: '#0b1a2a', terminal: '#061019', border: '#1c3a50', text: '#dcecf1', muted: '#7894a4', cyan: '#67e8f9', blue: '#7dd3fc', green: '#86efac', amber: '#fbbf24'};

const fade = (frame: number, from: number, length = 16) => interpolate(frame, [from, from + length], [0, 1], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});

function typed(text: string, frame: number, at: number) {
  const count = Math.floor(interpolate(Math.max(0, frame - at), [0, 13], [0, text.length], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.quad)}));
  return text.slice(0, count);
}

function InstallTerminal({frame}: {frame: number}) {
  const visible = logs.filter((log) => frame >= log.at);
  const shown = visible.slice(Math.max(0, visible.length - 10));
  return <div style={{position: 'absolute', left: 88, top: 208, width: 1100, height: 690, background: c.terminal, border: `1px solid ${c.border}`, borderRadius: 18, boxShadow: '0 30px 80px rgba(0,0,0,.38)', overflow: 'hidden'}}>
    <div style={{height: 56, display: 'flex', alignItems: 'center', padding: '0 22px', borderBottom: `1px solid ${c.border}`, background: '#0b1725'}}>
      <span style={{width: 12, height: 12, borderRadius: 99, background: '#fb7185', marginRight: 8}} />
      <span style={{width: 12, height: 12, borderRadius: 99, background: '#fbbf24', marginRight: 8}} />
      <span style={{width: 12, height: 12, borderRadius: 99, background: '#4ade80', marginRight: 18}} />
      <span style={{fontFamily: 'Inter, sans-serif', fontSize: 16, color: c.muted}}>vps-tools  •  first boot</span>
      <span style={{marginLeft: 'auto', fontFamily: 'Inter, sans-serif', fontSize: 13, color: '#466277'}}>single server / local data</span>
    </div>
    <div style={{padding: '28px 32px', fontFamily: 'JetBrains Mono, Consolas, monospace', fontSize: 20, lineHeight: 1.65}}>
      {shown.map((log) => {
        const latest = log === visible[visible.length - 1];
        const text = latest ? typed(log.text, frame, log.at) : log.text;
        const colour = log.tone === 'command' ? c.blue : log.tone === 'success' ? c.green : log.tone === 'info' ? c.cyan : log.tone === 'warning' ? c.amber : c.muted;
        return <div key={`${log.at}-${log.text}`} style={{height: 33, color: colour, opacity: fade(frame, log.at)}}>{text}{latest && <span style={{display: 'inline-block', width: 11, height: 22, marginLeft: 5, verticalAlign: -3, background: frame % 30 < 15 ? c.cyan : 'transparent'}} />}</div>;
      })}
    </div>
    <div style={{position: 'absolute', right: 30, bottom: 20, fontFamily: 'Inter, sans-serif', fontSize: 12, color: '#36546a'}}>safe defaults, ready to extend</div>
  </div>;
}

function InstallCard({frame}: {frame: number}) {
  const card = cards.find((item) => frame >= item.from && frame < item.to) ?? cards[2];
  const opacity = fade(frame, card.from, 16);
  const y = interpolate(frame, [card.from, card.from + 16], [12, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
  const progress = Math.min(100, Math.max(0, ((frame - card.from) / (card.to - card.from)) * 100));
  return <div style={{position: 'absolute', right: 88, top: 286, width: 390, opacity, transform: `translateY(${y}px)`}}>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 16, letterSpacing: 2, color: c.cyan, fontWeight: 700}}>STEP {card.number}</div>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 35, color: c.text, fontWeight: 700, marginTop: 15, lineHeight: 1.1}}>{card.title}</div>
    <div style={{fontFamily: 'Inter, sans-serif', fontSize: 19, color: c.muted, lineHeight: 1.5, marginTop: 20}}>{card.detail}</div>
    <div style={{marginTop: 34, height: 4, width: 390, background: '#163049', borderRadius: 99, overflow: 'hidden'}}><div style={{height: '100%', width: `${progress}%`, background: c.cyan, borderRadius: 99}} /></div>
  </div>;
}

export const SelfContainedInstall: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const intro = spring({frame, fps, config: {damping: 200, stiffness: 100}});
  const opacity = interpolate(intro, [0, 1], [0, 1]);
  const y = interpolate(intro, [0, 1], [18, 0]);
  return <AbsoluteFill style={{background: c.bg, color: c.text}}>
    <div style={{position: 'absolute', inset: 0, background: 'radial-gradient(circle at 76% 12%, rgba(34,211,238,.13), transparent 30%), linear-gradient(135deg, #06111d 0%, #082034 100%)'}} />
    <div style={{position: 'absolute', left: 88, top: 72, right: 88, display: 'flex', alignItems: 'end', opacity, transform: `translateY(${y}px)`}}>
      <div>
        <div style={{fontFamily: 'Inter, sans-serif', fontSize: 17, fontWeight: 700, letterSpacing: 3, color: c.cyan}}>VPS TOOLS  /  INSTALL MOTION SAMPLE</div>
        <div style={{fontFamily: 'Inter, sans-serif', fontSize: 44, lineHeight: 1.1, fontWeight: 700, marginTop: 12}}>Start small. Stay in control.</div>
      </div>
      <div style={{marginLeft: 'auto', fontFamily: 'Inter, sans-serif', fontSize: 15, color: c.muted}}>The self-contained deployment</div>
    </div>
    <InstallTerminal frame={frame} />
    <InstallCard frame={frame} />
    <div style={{position: 'absolute', left: 88, bottom: 64, right: 88, display: 'flex', alignItems: 'center', fontFamily: 'Inter, sans-serif', fontSize: 14, color: c.muted}}>
      <span style={{color: c.green, marginRight: 10}}>●</span> SQLite  •  encrypted local artefacts  •  embedded scheduler
      <span style={{marginLeft: 'auto'}}>no external services required</span>
    </div>
  </AbsoluteFill>;
};
