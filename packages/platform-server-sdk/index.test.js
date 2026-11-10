import {test} from 'node:test';
import assert from 'node:assert/strict';
import {PlatformClient, PlatformError} from './index.js';
test('cache separates users, teams and scopes and coalesces token requests', async () => {
 let tokens = 0;
 const fetch = async (url, init) => {
   if (url.endsWith('/token')) { tokens++; await new Promise(r => setTimeout(r, 5)); assert.match(init.headers.Authorization, /^Basic /); return Response.json({access_token:'mac_' + 'a'.repeat(64),expires_in:3600}); }
   assert.match(init.headers.Authorization,/^Bearer mac_/); return Response.json({});
 };
 const p = new PlatformClient({baseUrl:'https://platform.test',clientId:'app',clientSecret:'secret',fetch});
 const a = p.as('u',{teamId:'t',scopes:['workspace.read']});
 await Promise.all([a.getAgent('agent'),a.getAgent('agent')]); assert.equal(tokens,1);
 await p.as('other',{teamId:'t',scopes:['workspace.read']}).getAgent('agent');
 await p.as('u',{teamId:'other',scopes:['workspace.read']}).getAgent('agent');
 await p.as('u',{teamId:'t',scopes:['session.chat']}).listMessages('session');assert.equal(tokens,4);
});
test('revocation invalidates cache without replaying a write', async () => {
 let tokens=0,writes=0;
 const fetch = async url => url.endsWith('/token') ? (tokens++,Response.json({access_token:'mac_'+'b'.repeat(64),expires_in:3600})) : (writes++,new Response('',{status:403}));
 const p = new PlatformClient({baseUrl:'https://platform.test',clientId:'app',clientSecret:'secret',fetch});
 const a = p.as('u',{teamId:'t',scopes:['session.chat']});
 await assert.rejects(a.runTurn('s','hello','key'),PlatformError);assert.equal(writes,1);
 await assert.rejects(a.runTurn('s','hello','key'),PlatformError);assert.equal(tokens,2);assert.equal(writes,2);
 assert.throws(()=>a.runTurn('s','hello',''),/idempotency/);
});
test('invalid identities and scope combinations fail before transport',()=>{
 const p=new PlatformClient({baseUrl:'https://platform.test',clientId:'app',clientSecret:'secret'});
 assert.throws(()=>p.as('..'));assert.throws(()=>p.as('u',{teamId:'t'}));assert.throws(()=>p.as('u',{teamId:'t',scopes:['admin']}));
});
test('cancelling one token waiter does not cancel another user request sharing the token', async()=>{
 let release;const gate=new Promise(r=>release=r);let calls=0;
 const fetch=async(url,options)=>{calls++;if(url.endsWith('/token')){await gate;return Response.json({access_token:'mac_'+'c'.repeat(64),expires_in:3600});}options.signal.throwIfAborted();return Response.json({sessions:[]});};
 const p=new PlatformClient({baseUrl:'https://platform.test',clientId:'app',clientSecret:'secret',fetch});const user=p.as('u',{teamId:'t',scopes:['session.chat']});const abort=new AbortController();
 const cancelled=user.listSessions('ws',{signal:abort.signal});const surviving=user.listSessions('ws');abort.abort();await assert.rejects(cancelled,{name:'AbortError'});release();await surviving;assert.equal(calls,2);
});
test('personal connection operations use scoped credentials, CAS and explicit agent binding',async()=>{
 const calls=[];const fetch=async(url,init)=>{if(url.endsWith('/token'))return Response.json({access_token:'mac_'+'d'.repeat(64),expires_in:3600});calls.push({url,method:init.method,body:init.body&&JSON.parse(init.body)});assert.match(init.headers.Authorization,/^Bearer mac_/);return Response.json({id:'connection'});};
 const p=new PlatformClient({baseUrl:'https://platform.test',clientId:'app',clientSecret:'secret',fetch});const u=p.as('u',{teamId:'team',scopes:['workspace.ensure','workspace.read']});
 await u.createModelConnection({name:'Codex',auth_json:'private'});await u.listModelConnections();await u.updateModelConnection('connection',{name:'Codex',expected_version:1,auth_json:'replacement'});await u.ensureAgent('assistant',{display_name:'Assistant',agent_type:'codex',model_connection_id:'connection'});await u.revokeModelConnection('connection',2);
 assert.equal(calls[0].url,'https://platform.test/apps/v1/model-connections');assert.equal(calls[1].method,'GET');assert.equal(calls[2].body.expected_version,1);assert.equal(calls[3].body.model_connection_id,'connection');assert.deepEqual(calls[4].body,{expected_version:2});
});
