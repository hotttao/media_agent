import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createSessionRelay} from './index.js';
const root='https://business.test/api/agent/workspaces/me/sessions';
function fixture(overrides={}) {
 const calls=[];const client={listSessions:async()=>({sessions:[{id:'owned',workspace_id:'fixed'}]}),createSession:async(ws,title)=>{calls.push([ws,title]);return {id:'new',workspace_id:ws};},runTurn:async(...args)=>{calls.push(args);return {turn_id:'turn'};},listMessages:async()=>({messages:[]}),...overrides};
 const relay=createSessionRelay({basePath:'/api/agent',authenticate:async r=>r.headers.has('cookie')?{id:'local-user'}:null,resolve:async(ctx,key)=>{calls.push([ctx.id,key]);return {client,workspaceId:'fixed'};},maxBodyBytes:128});
 return {relay,calls};
}
const req=(path='',method='GET',value,headers={})=>new Request(root+path,{method,headers:{cookie:'local-session',origin:'https://business.test','content-type':'application/json',...headers},...(value===undefined?{}:{body:JSON.stringify(value)})});
test('relay binds authenticated user to server-selected workspace and hides credentials',async()=>{
 const {relay,calls}=fixture();const r=await relay.handle(req('','POST',{title:'hello',workspace_id:'attacker',user_id:'attacker'}));assert.equal(r.status,201);assert.deepEqual(calls,[['local-user','me'],['fixed','hello']]);assert.equal((await r.json()).workspace_id,'fixed');
});
test('relay rejects foreign sessions, unknown routes, cross-origin writes and oversized bodies',async()=>{
 const {relay}=fixture();assert.equal((await relay.handle(req('/foreign/turns','POST',{message:'hi',idempotency_key:'k'}))).status,404);
 assert.equal((await relay.handle(req('/owned/arbitrary'))).status,404);
 assert.equal((await relay.handle(req('','POST',{title:'x'},{origin:'https://evil.test'}))).status,403);
 assert.equal((await relay.handle(req('','POST',{title:'x'.repeat(200)}))).status,413);
 const r=new Request(root);assert.equal((await relay.handle(r)).status,401);
});
test('SSE preserves sequence cursor and cancellation reaches upstream',async()=>{
 let observed,cancelled=false;const {relay}=fixture({events:async(id,seq,{signal})=>{observed={id,seq,signal};return new Response(new ReadableStream({start(c){c.enqueue(new TextEncoder().encode('id: 9\ndata: {}\n\n'));},cancel(){cancelled=true;}}),{headers:{'content-type':'text/event-stream','set-cookie':'secret'}});}});
 const r=await relay.handle(req('/owned/events?after_sequence=8'));assert.equal(r.status,200);assert.equal(observed.seq,8);assert.equal(r.headers.get('set-cookie'),null);const reader=r.body.getReader();assert.match(new TextDecoder().decode((await reader.read()).value),/id: 9/);await reader.cancel();assert.equal(cancelled,true);assert.equal(observed.signal.aborted,true);
});
test('cancelling a real fetch stream closes its reader before aborting transport',async()=>{
 const {relay}=fixture({events:async(id,seq,{signal})=>new Response(new ReadableStream({start(c){signal.addEventListener('abort',()=>c.error(signal.reason),{once:true});c.enqueue(new TextEncoder().encode('id: 1\ndata: {}\n\n'));}}),{headers:{'content-type':'text/event-stream'}})});
 const response=await relay.handle(req('/owned/events'));const reader=response.body.getReader();await reader.read();await reader.cancel();
});
