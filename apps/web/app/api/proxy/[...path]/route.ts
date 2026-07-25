import { NextRequest, NextResponse } from 'next/server';
import { readSession } from '../../../lib/oidc';

async function proxyHandler(request: NextRequest, context: {params: Promise<{path:string[]}>}) {
  const session = await readSession(); if (!session) return NextResponse.json({error:'authentication required'},{status:401});
  const {path}=await context.params; const base=(process.env.API_INTERNAL_URL||process.env.NEXT_PUBLIC_API_URL||'http://localhost:8080').replace(/\/$/,'');
  const target=`${base}/${path.join('/')}${request.nextUrl.search}`; const headers=new Headers(request.headers);
  headers.set('X-VPS-Internal-Secret',process.env.VPS_WEB_SHARED_SECRET||''); headers.set('X-VPS-OIDC-Subject',session.sub); headers.set('X-VPS-OIDC-Email',session.email); headers.delete('host');
  const body=['GET','HEAD'].includes(request.method)?undefined:await request.arrayBuffer();
  const response=await fetch(target,{method:request.method,headers,body,redirect:'manual'}); return new NextResponse(response.body,{status:response.status,headers:response.headers});
}
export const GET=proxyHandler; export const POST=proxyHandler; export const PUT=proxyHandler; export const PATCH=proxyHandler; export const DELETE=proxyHandler; export const OPTIONS=proxyHandler;
