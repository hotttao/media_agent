// Opt-in live acceptance. Reads application credentials and an authorized auth file path from stdin JSON.
import {PlatformClient,createSessionRelay} from '../index.js';
import {readFile} from 'node:fs/promises';
let raw='';for await(const chunk of process.stdin)raw+=chunk;const input=JSON.parse(raw);
if (!input.authFile) throw new Error('authFile must explicitly identify an authorized Codex auth.json');
const platform=new PlatformClient({baseUrl:input.base,clientId:input.app,clientSecret:input.secret});
const user=platform.as(input.external,{teamId:input.team,scopes:['workspace.read','workspace.ensure','session.chat']});
const connection=await user.createModelConnection({name:'OAuth runtime verification',auth_json:await readFile(input.authFile,'utf8')});
const report={connection_id:connection.id};let version=connection.version;
try {
 const listed=await user.listModelConnections();if(!listed.connections?.some(c=>c.id===connection.id))throw new Error('connection absent');
 const config={display_name:'OAuth verification',agent_type:'codex',model_connection_id:connection.id};
 const ws=await user.ensureAgent('oauth-verification',config);report.workspace_id=ws.id;
 if((await user.ensureAgent('oauth-verification',config)).id!==ws.id)throw new Error('Ensure changed identity');
 const relay=createSessionRelay({basePath:'/api/agent',authenticate:async()=>({id:input.external}),resolve:async()=>({client:user,workspaceId:ws.id})});
 const root='http://business.test/api/agent/workspaces/me/sessions';
 const post=(url,body)=>relay.handle(new Request(url,{method:'POST',headers:{origin:'http://business.test','content-type':'application/json'},body:JSON.stringify(body)}));
 const created=await post(root,{title:'SDK personal Codex OAuth verification'});if(created.status!==201)throw new Error('create session '+created.status);
 const session=await created.json();report.session_id=session.id;const path=root+'/'+session.id;
 const run=await post(path+'/turns',{message:'Reply with exactly SDK_OAUTH_OK. Do not use tools.',idempotency_key:crypto.randomUUID()});if(!run.ok)throw new Error('run turn '+run.status);report.turn=await run.json();
 const consume=async(after,untilFirst=false)=>{
  const response=await relay.handle(new Request(path+'/events',{headers:{'Last-Event-ID':String(after)},signal:AbortSignal.timeout(150000)}));if(!response.ok)throw new Error('events '+response.status);
  const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='';const events=[];
  try{while(true){const {value,done}=await reader.read();if(done)break;buffer+=decoder.decode(value,{stream:true}).replace(/\r\n/g,'\n');let boundary;while((boundary=buffer.indexOf('\n\n'))>=0){const frame=buffer.slice(0,boundary);buffer=buffer.slice(boundary+2);const id=Number(frame.match(/^id: ?(\d+)/m)?.[1]);if(!id)continue;const data=JSON.parse(frame.split('\n').filter(s=>s.startsWith('data:')).map(s=>s.slice(5).trimStart()).join('\n'));events.push({id,type:data.type});if(untilFirst||['turn.completed','turn.failed','turn.interrupted'].includes(data.type))return events;}}return events;}finally{await reader.cancel();}
 };
 const first=await consume(0,true);if(!first.length)throw new Error('no persisted events');
 const rest=await consume(first.at(-1).id);report.events=[...first,...rest];if(rest.some(e=>e.id<=first.at(-1).id))throw new Error('replayed cursor');
 if(report.events.at(-1)?.type!=='turn.completed')throw new Error('terminal '+JSON.stringify(report.events));
 const history=await relay.handle(new Request(path+'/messages'));const messages=await history.json();if(!messages.messages?.some(m=>m.role==='assistant'&&m.content_json.includes('SDK_OAUTH_OK')))throw new Error('expected model reply absent');
 report.model_inference_verified=true;report.reconnect_verified=true;report.history_verified=true;
 const followup=await post(path+'/turns',{message:'Reply exactly SDK_SECOND_OK. Do not use tools.',idempotency_key:crypto.randomUUID()});if(!followup.ok)throw new Error('followup '+followup.status);
 const followupEvents=await consume(report.events.at(-1).id);report.followup_events=followupEvents;if(followupEvents.at(-1)?.type!=='turn.completed')throw new Error('followup terminal '+JSON.stringify(followupEvents));
 const followupHistory=await user.listMessages(session.id);if(!followupHistory.messages?.some(m=>m.role==='assistant'&&m.content_json.includes('SDK_SECOND_OK')))throw new Error('followup reply absent');
 report.followup_verified=true;

 const next=await post(path+'/turns',{message:'Write a detailed essay about the history of mathematics with at least 5000 words. Do not use tools.',idempotency_key:crypto.randomUUID()});if(!next.ok)throw new Error('second turn '+next.status);
 const interrupted=await post(path+'/interrupt',{});if(!interrupted.ok)throw new Error('interrupt '+interrupted.status);report.interrupt=await interrupted.json();
 const terminal=await consume(report.followup_events.at(-1).id);report.interrupt_events=terminal;if(terminal.at(-1)?.type!=='turn.interrupted')throw new Error('interrupt terminal '+JSON.stringify(terminal));
 const renamed=await user.updateModelConnection(connection.id,{name:'Verified OAuth',expected_version:version});version=renamed.version;
} finally { await user.revokeModelConnection(connection.id,version); }
report.connection_revoked=true;console.log(JSON.stringify(report));
