import {createPublicKey, verify} from 'node:crypto';
// jwks, issuer and audience must come from deployment configuration, never the request.
export function verifyCaller(token, {jwks, issuer, audience, workspaceId, sessionId, now = Date.now()}) {
  const fail = () => { throw new Error('Invalid platform Caller'); };
  if (typeof token !== 'string' || token.length > 16384 || !issuer || !audience) return fail();
  const parts = token.split('.');
  if (parts.length !== 3 || parts.some(p => !/^[A-Za-z0-9_-]+$/.test(p))) return fail();
  let header, claims, key;
  try {
    header = JSON.parse(Buffer.from(parts[0], 'base64url').toString());
    claims = JSON.parse(Buffer.from(parts[1], 'base64url').toString());
    if (header.alg !== 'RS256' || header.typ !== 'platform-caller+jwt' || typeof header.kid !== 'string' || header.crit) return fail();
    const keys = jwks.keys.filter(k => k.kid === header.kid && k.kty === 'RSA' && k.alg === 'RS256' && k.use === 'sig');
    if (keys.length !== 1 || keys[0].d) return fail();
    key = createPublicKey({key: keys[0], format: 'jwk'});
    if (key.asymmetricKeyType !== 'rsa' || key.asymmetricKeyDetails.modulusLength < 2048) return fail();
  } catch { return fail(); }
  if (!verify('RSA-SHA256', Buffer.from(parts[0] + '.' + parts[1]), key, Buffer.from(parts[2], 'base64url'))) return fail();
  const seconds = Math.floor(now / 1000);
  if (claims.iss !== issuer || !Array.isArray(claims.aud) || claims.aud.length !== 1 || claims.aud[0] !== audience ||
      !Number.isSafeInteger(claims.exp) || !Number.isSafeInteger(claims.iat) || !Number.isSafeInteger(claims.nbf) ||
      claims.exp <= seconds || claims.iat > seconds || claims.nbf > seconds || claims.exp <= claims.iat || claims.exp - claims.iat > 60) return fail();
  for (const name of ['sub','ext','jti','workspace_id','session_id','organization_id','team_id']) {
    if (typeof claims[name] !== 'string' || !claims[name] || claims[name].length > 512) return fail();
  }
  if (workspaceId !== undefined && claims.workspace_id !== workspaceId || sessionId !== undefined && claims.session_id !== sessionId) return fail();
  return Object.freeze({userId:claims.sub,externalId:claims.ext,applicationId:audience,workspaceId:claims.workspace_id,sessionId:claims.session_id,organizationId:claims.organization_id,teamId:claims.team_id,expiresAt:claims.exp,assertionId:claims.jti});
}
