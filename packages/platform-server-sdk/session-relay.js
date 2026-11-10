import {PlatformError} from './index.js';
class RelayError extends Error { constructor(status) { super('Relay request rejected'); this.status = status; } }
const json = (value, status = 200) => Response.json(value, {status, headers:{'Cache-Control':'no-store'}});
async function body(request, limit) {
  if (request.headers.get('content-type')?.split(';')[0].trim().toLowerCase() !== 'application/json') throw new RelayError(415);
  const reader = request.body?.getReader(); if (!reader) throw new RelayError(400);
  let size = 0; const chunks = [];
  try {
    for (;;) { const {done,value} = await reader.read(); if (done) break; size += value.byteLength; if (size > limit) { await reader.cancel(); throw new RelayError(413); } chunks.push(value); }
  } finally { reader.releaseLock(); }
  const bytes = new Uint8Array(size); let offset = 0; for (const chunk of chunks) { bytes.set(chunk,offset);offset+=chunk.length; }
  try { const value=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes)); if (!value || typeof value!=='object' || Array.isArray(value)) throw 0; return value; } catch { throw new RelayError(400); }
}
export function createSessionRelay({basePath, authenticate, resolve, maxBodyBytes = 1024*1024}) {
  if (!/^\/[\w/-]+$/.test(basePath) || basePath.endsWith('/') || typeof authenticate!=='function' || typeof resolve!=='function' || !Number.isSafeInteger(maxBodyBytes) || maxBodyBytes<1) throw new TypeError('Invalid relay configuration');
  return Object.freeze({async handle(request) {
    const abort = new AbortController(); const signal = AbortSignal.any([request.signal,abort.signal]);
    try {
      const url=new URL(request.url); if (!url.pathname.startsWith(basePath+'/')) throw new RelayError(404);
      const path=url.pathname.slice(basePath.length).split('/').slice(1);
      if (path.some(p=>!p || /%2f|%5c/i.test(p))) throw new RelayError(404);
      try { for(let i=0;i<path.length;i++) path[i]=decodeURIComponent(path[i]); } catch { throw new RelayError(404); }
      const [root,key,collection,sid,action,turn,requests,rid,operation]=path;
      let route;
      if(root==='workspaces' && collection==='sessions') {
        if(path.length===3 && ['GET','POST'].includes(request.method)) route=request.method==='GET'?'list':'create';
        if(path.length===5 && request.method==='GET' && ['messages','events','requests'].includes(action)) route=action;
        if(path.length===5 && request.method==='POST' && ['turns','interrupt'].includes(action)) route=action;
        if(path.length===9 && action==='turns' && requests==='requests' && operation==='resolve' && request.method==='POST') route='resolve';
      }
      if(!route) throw new RelayError(404);
      if(request.method==='POST' && request.headers.get('origin')!==url.origin) throw new RelayError(403);
      const ctx=await authenticate(request);if(ctx==null) throw new RelayError(401);
      const target=await resolve(ctx,key);if(!target?.client || !target.workspaceId) throw new RelayError(404);
      const {client,workspaceId}=target;const options={signal};
      let data;
      if(request.method==='POST') data=await body(request,maxBodyBytes);
      if(sid) {const current=await client.listSessions(workspaceId,options);if(!current.sessions?.some(s=>s.id===sid && s.workspace_id===workspaceId)) throw new RelayError(404);}
      switch(route) {
        case 'list': return json(await client.listSessions(workspaceId,options));
        case 'create': if(typeof data.title!=='string') throw new RelayError(400); return json(await client.createSession(workspaceId,data.title,options),201);
        case 'messages': return json(await client.listMessages(sid,options));
        case 'requests': return json(await client.listRequests(sid,options));
        case 'turns': if(typeof data.message!=='string'||!data.message.trim()||typeof data.idempotency_key!=='string'||!data.idempotency_key) throw new RelayError(400);return json(await client.runTurn(sid,data.message,data.idempotency_key,options));
        case 'interrupt': return json(await client.interrupt(sid,options));
        case 'resolve': if(typeof data.response_json!=='string') throw new RelayError(400);return json(await client.resolveRequest(sid,turn,rid,{response_json:data.response_json},options));
        case 'events': {
          const cursor=url.searchParams.get('after_sequence') ?? request.headers.get('last-event-id') ?? '0';
          if(!/^\d+$/.test(cursor)||!Number.isSafeInteger(Number(cursor))) throw new RelayError(400);
          const upstream=await client.events(sid,Number(cursor),options);
          if(!upstream.body || !upstream.headers.get('content-type')?.startsWith('text/event-stream')) {await upstream.body?.cancel();throw new RelayError(502);}
          const reader=upstream.body.getReader();
          let cancelled=false;
          const stream=new ReadableStream({
            async pull(controller){
              try {const {done,value}=await reader.read();if(cancelled)return;if(done){controller.close();reader.releaseLock();}else controller.enqueue(value);}
              catch(error){if(!cancelled)controller.error(error);abort.abort();}
            },
            async cancel(reason){cancelled=true;try{await reader.cancel(reason);}finally{abort.abort(reason);}}
          });
          return new Response(stream,{headers:{'Content-Type':'text/event-stream','Cache-Control':'no-store','X-Accel-Buffering':'no'}});
        }
      }
    } catch(error) {
      abort.abort();
      if(request.signal.aborted) throw request.signal.reason;
      const status=error instanceof RelayError||error instanceof PlatformError ? error.status : 502;
      return json({error:status===404?'not_found':status===401?'authentication_required':'relay_request_failed'},status);
    }
  }});
}
