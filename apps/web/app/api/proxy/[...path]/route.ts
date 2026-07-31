import { NextRequest, NextResponse } from 'next/server';
import { readSession } from '../../../lib/oidc';

async function proxyHandler(request: NextRequest, context: {params: Promise<{path:string[]}>}) {
  const developmentAuth = process.env.VPS_DEV_AUTH === 'true' && process.env.NEXT_PUBLIC_DEV_AUTH === 'true';
  const session = await readSession();
  if (!session && !developmentAuth) return NextResponse.json({error:'authentication required'},{status:401});
  if (!['GET', 'HEAD', 'OPTIONS'].includes(request.method)) {
    const requestOrigin = request.headers.get('origin');
    if (!requestOrigin) return NextResponse.json({error:'CSRF origin check failed'},{status:403});
    if (developmentAuth) {
      try {
        if (new URL(requestOrigin).host !== request.headers.get('host')) return NextResponse.json({error:'CSRF origin check failed'},{status:403});
      } catch { return NextResponse.json({error:'CSRF origin check failed'},{status:403}); }
    } else {
      const expectedOrigin = new URL(process.env.APP_URL || request.nextUrl.origin).origin;
      if (requestOrigin !== expectedOrigin) return NextResponse.json({error:'CSRF origin check failed'},{status:403});
    }
  }
  const {path}=await context.params; const base=(process.env.API_INTERNAL_URL||process.env.NEXT_PUBLIC_API_URL||'http://localhost:8080').replace(/\/$/,'');
  const target=`${base}/${path.join('/')}${request.nextUrl.search}`; const headers=new Headers(request.headers);
  // Identity is decided here, never by the caller. Strip any identity headers
  // the browser supplied before setting the ones this proxy vouches for,
  // otherwise a client could name whichever user it liked.
  headers.delete('X-VPS-User');
  headers.delete('X-VPS-OIDC-Subject');
  headers.delete('X-VPS-OIDC-Email');
  headers.delete('X-VPS-Internal-Secret');
  headers.set('X-VPS-Internal-Secret',process.env.VPS_WEB_SHARED_SECRET||'');
  if (session) { headers.set('X-VPS-OIDC-Subject',session.sub); headers.set('X-VPS-OIDC-Email',session.email); }
  // The API never infers an identity, so development mode must name the actor
  // explicitly. This branch only runs when both dev-auth flags are set, and
  // the API additionally refuses header identity outside an explicitly
  // non-production environment.
  else if (developmentAuth) { headers.set('X-VPS-User', process.env.VPS_DEV_USER || 'user_senior'); }
  headers.delete('host');
  const body=['GET','HEAD'].includes(request.method)?undefined:await request.arrayBuffer();
  const response=await fetch(target,{method:request.method,headers,body,redirect:'manual'}); return new NextResponse(response.body,{status:response.status,headers:response.headers});
}
export const GET=proxyHandler; export const POST=proxyHandler; export const PUT=proxyHandler; export const PATCH=proxyHandler; export const DELETE=proxyHandler; export const OPTIONS=proxyHandler;
