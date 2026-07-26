export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body suppressHydrationWarning style={{ margin: 0, background: '#0f172a' }}>{children}</body>
    </html>
  );
}
