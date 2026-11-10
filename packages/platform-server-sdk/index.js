// Server-only client. Application secrets must never be sent to a browser.
const segment = value => {
  if (typeof value !== 'string' || !value || value === '.' || value === '..') throw new TypeError('A stable identifier is required');
  return encodeURIComponent(value);
};
function waitForToken(promise, signal) {
  if (!signal) return promise;
  signal.throwIfAborted();
  return new Promise((resolve,reject) => {
    const cancel=()=>reject(signal.reason);
    signal.addEventListener('abort',cancel,{once:true});
    promise.then(resolve,reject).finally(()=>signal.removeEventListener('abort',cancel));
  });
}
const validScopes = new Set(['workspace.read', 'workspace.ensure', 'session.chat']);
export class PlatformError extends Error {
  constructor(status) { super(`Platform request failed (${status})`); this.name = 'PlatformError'; this.status = status; }
}
export class PlatformClient {
  #base; #basic; #fetch; #tokens = new Map(); #pending = new Map();
  constructor({ baseUrl, clientId, clientSecret, fetch: fetchImpl = globalThis.fetch }) {
    if (typeof window !== 'undefined') throw new Error('PlatformClient is server-only');
    const url = new URL(baseUrl);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) throw new TypeError('Invalid platform URL');
    if (!clientId || clientId.includes(':') || !clientSecret) throw new TypeError('Application credentials are required');
    this.#base = url.href.replace(/\/$/, '');
    this.#basic = 'Basic ' + Buffer.from(`${clientId}:${clientSecret}`, 'utf8').toString('base64');
    this.#fetch = fetchImpl;
  }
  async #send(path, {method = 'GET', body, authorization, stream = false, signal} = {}) {
    const response = await this.#fetch(this.#base + path, {
      method, redirect: 'error', signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(stream ? 3600000 : 30000)]) : AbortSignal.timeout(stream ? 3600000 : 30000),
      headers: {Authorization: authorization, ...(body === undefined ? {} : {'Content-Type': 'application/json'})},
      ...(body === undefined ? {} : {body: JSON.stringify(body)})
    });
    if (!response.ok) { await response.body?.cancel(); throw new PlatformError(response.status); }
    return stream ? response : response.json();
  }
  upsertUser(externalId, profile = {}) {
    return this.#send(`/apps/v1/users/${segment(externalId)}`, {method: 'PUT', body: profile, authorization: this.#basic});
  }
  getUser(externalId) { return this.#send(`/apps/v1/users/${segment(externalId)}`, {authorization: this.#basic}); }
  async #token(externalId, teamId, scopes, key) {
    const cached = this.#tokens.get(key);
    if (cached && cached.expires > Date.now()) return cached.token;
    this.#tokens.delete(key);
    if (this.#pending.has(key)) return this.#pending.get(key);
    const request = (async () => {
      const started = Date.now();
      const value = await this.#send(`/apps/v1/users/${segment(externalId)}/token`, {
        method: 'POST', body: teamId ? {team_id: teamId, scopes} : {}, authorization: this.#basic
      });
      if (typeof value.access_token !== 'string' || !/^mac_[a-f0-9]{64}$/.test(value.access_token) || !Number.isFinite(value.expires_in) || value.expires_in <= 30) throw new Error('Invalid platform token response');
      // Bound cache size and renew before expiry. No refresh token or automatic write retry.
      if (this.#tokens.size >= 1000) this.#tokens.delete(this.#tokens.keys().next().value);
      this.#tokens.set(key, {token: value.access_token, expires: started + (Math.min(value.expires_in, 3600) - 30) * 1000});
      return value.access_token;
    })();
    this.#pending.set(key, request);
    try { return await request; } finally { this.#pending.delete(key); }
  }
  as(externalId, {teamId = '', scopes = []} = {}) {
    segment(externalId);
    const sorted = [...new Set(scopes)].sort();
    if (!!teamId !== !!sorted.length || sorted.some(s => !validScopes.has(s))) throw new TypeError('Team and valid scopes must be supplied together');
    const key = JSON.stringify([externalId, teamId, sorted]);
    const call = async (path, options = {}) => {
      options.signal?.throwIfAborted();
      const token = await waitForToken(this.#token(externalId, teamId, sorted, key), options.signal);
      options.signal?.throwIfAborted();
      try { return await this.#send(path, {...options, authorization: 'Bearer ' + token}); }
      catch (error) { if (error instanceof PlatformError && [401,403].includes(error.status)) this.#tokens.delete(key); throw error; }
    };
    const sessionPath = id => `/apps/v1/chat/sessions/${segment(id)}`;
    return Object.freeze({
      me: () => call('/apps/v1/me'),
      createModelConnection: (configuration, options = {}) => call('/apps/v1/model-connections', {method:'POST', body:configuration, signal:options.signal}),
      listModelConnections: (options = {}) => call('/apps/v1/model-connections', options),
      updateModelConnection: (id, configuration, options = {}) => call(`/apps/v1/model-connections/${segment(id)}`, {method:'PUT',body:configuration,signal:options.signal}),
      revokeModelConnection: (id, expectedVersion, options = {}) => call(`/apps/v1/model-connections/${segment(id)}/revoke`, {method:'POST',body:{expected_version:expectedVersion},signal:options.signal}),
      ensureAgent: (name, configuration) => call(`/apps/v1/agents/${segment(name)}`, {method:'PUT', body:configuration}),
      getAgent: name => call(`/apps/v1/agents/${segment(name)}`),
      createSession: (workspaceId, title = '', options = {}) => call('/apps/v1/chat/sessions', {method:'POST', body:{workspace_id:workspaceId,title}, signal:options.signal}),
      listSessions: (workspaceId, options = {}) => call(`/apps/v1/chat/sessions?workspace_id=${segment(workspaceId)}`, options),
      listMessages: (id, options = {}) => call(sessionPath(id) + '/messages', options),
      runTurn: (id, message, idempotencyKey, options = {}) => {
        if (!idempotencyKey) throw new TypeError('An idempotency key is required');
        return call(sessionPath(id) + '/turns', {method:'POST',body:{message,idempotency_key:idempotencyKey},signal:options.signal});
      },
      interrupt: (id, options = {}) => call(sessionPath(id) + '/interrupt', {method:'POST',body:{},signal:options.signal}),
      events: (id, afterSequence = 0, options = {}) => {
        if (!Number.isSafeInteger(afterSequence) || afterSequence < 0) throw new TypeError('Invalid event sequence');
        return call(sessionPath(id) + `/events?after_sequence=${afterSequence}`, {stream:true,signal:options.signal});
      },
      listRequests: (id, options = {}) => call(sessionPath(id) + '/requests', options),
      resolveRequest: (id, turn, request, resolution, options = {}) => call(sessionPath(id) + `/turns/${segment(turn)}/requests/${segment(request)}/resolve`, {method:'POST',body:resolution,signal:options.signal})
    });
  }
}
export {verifyCaller} from './caller.js';

export {createSessionRelay} from './session-relay.js';
