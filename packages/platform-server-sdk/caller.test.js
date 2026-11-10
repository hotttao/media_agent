import {test} from 'node:test';
import assert from 'node:assert/strict';
import {generateKeyPairSync, sign} from 'node:crypto';
import {verifyCaller} from './index.js';
test('Caller pins key, type, issuer, audience, lifetime and workspace',()=>{
 const {privateKey,publicKey}=generateKeyPairSync('rsa',{modulusLength:2048});
 const jwks={keys:[{...publicKey.export({format:'jwk'}),kid:'key',alg:'RS256',use:'sig'}]};
 const now=Math.floor(Date.now()/1000);
 const claims={iss:'gateway',aud:['business'],sub:'managed-user',ext:'external-user',jti:'request',iat:now,nbf:now,exp:now+60,workspace_id:'workspace',session_id:'session',organization_id:'org',team_id:'team'};
 const make=(c=claims,h={alg:'RS256',typ:'platform-caller+jwt',kid:'key'})=>{
  const raw=[h,c].map(x=>Buffer.from(JSON.stringify(x)).toString('base64url')).join('.');return raw+'.'+sign('RSA-SHA256',Buffer.from(raw),privateKey).toString('base64url');
 };
 const options={jwks,issuer:'gateway',audience:'business',workspaceId:'workspace',sessionId:'session',now:now*1000};
 assert.equal(verifyCaller(make(),options).externalId,'external-user');
 for(const patch of [{aud:['other']},{iss:'other'},{exp:now},{exp:now+61},{workspace_id:'other'},{iat:now+1},{ext:''}]) assert.throws(()=>verifyCaller(make({...claims,...patch}),options));
 assert.throws(()=>verifyCaller(make(claims,{alg:'RS256',typ:'JWT',kid:'key'}),options));
 const token=make();assert.throws(()=>verifyCaller(token.slice(0,-10)+'AAAAAAAAAA',options));
});
