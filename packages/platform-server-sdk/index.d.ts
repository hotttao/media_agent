export interface RequestOptions { signal?: AbortSignal; }
export interface Session { id: string; workspace_id: string; agent_id: string; status: string; created_at: number; updated_at: number; title: string; agent_type: string; chat_status: string; context_version: number; runtime_status: string; execution_epoch: number; }
export interface Message { id: string; turn_id: string; role: string; content_json: string; sequence: number; created_at: number; }
export interface TurnResult { turn_id: string; status: string; }
export interface AgentRequest { request_id: string; turn_id: string; request_type: string; payload_json: string; status: string; response_json: string; created_at: number; resolved_at: number; }
export type Scope = 'workspace.read' | 'workspace.ensure' | 'session.chat';
export interface AgentConfiguration { display_name: string; agent_type: 'goose' | 'codex' | 'claude-code'; api_key?: string; model_connection_id?: string; model_protocol?: string; model_name?: string; model_base_url?: string; }
export interface UserProfile { application_id: string; id: string; external_id: string; display_name: string; email: string; }
export interface Workspace { id: string; organization_id: string; team_id: string; created_by: string; name: string; agent_type: string; }
export class PlatformError extends Error { readonly status: number; }
export interface ModelConnection { id: string; name: string; kind: string; status: string; version: number; created_at: number; updated_at: number; }
export interface UserClient {
 createModelConnection(config: {name: string; auth_json: string}, options?: RequestOptions): Promise<ModelConnection>;
 listModelConnections(options?: RequestOptions): Promise<{connections: ModelConnection[]}>;
 updateModelConnection(id: string, config: {expected_version: number; name: string; auth_json?: string}, options?: RequestOptions): Promise<ModelConnection>;
 revokeModelConnection(id: string, expectedVersion: number, options?: RequestOptions): Promise<ModelConnection>;
 me(): Promise<UserProfile>;
 ensureAgent(name: string, config: AgentConfiguration): Promise<Workspace>;
 getAgent(name: string): Promise<Workspace>;
 createSession(workspaceId: string, title?: string, options?: RequestOptions): Promise<Session>;
 listSessions(workspaceId: string, options?: RequestOptions): Promise<{sessions: Session[]}>;
 listMessages(sessionId: string, options?: RequestOptions): Promise<{messages: Message[]}>;
 runTurn(sessionId: string, message: string, idempotencyKey: string, options?: RequestOptions): Promise<TurnResult>;
 interrupt(sessionId: string, options?: RequestOptions): Promise<TurnResult>;
 events(sessionId: string, afterSequence?: number, options?: RequestOptions): Promise<Response>;
 listRequests(sessionId: string, options?: RequestOptions): Promise<{requests: AgentRequest[]}>;
 resolveRequest(sessionId: string, turnId: string, requestId: string, resolution: {response_json: string}, options?: RequestOptions): Promise<AgentRequest>;
}
export class PlatformClient {
 constructor(options: {baseUrl: string; clientId: string; clientSecret: string; fetch?: typeof fetch});
 upsertUser(externalId: string, profile?: {display_name?: string; email?: string}): Promise<UserProfile>;
 getUser(externalId: string): Promise<UserProfile>;
 as(externalId: string, options?: {teamId?: string; scopes?: Scope[]}): UserClient;
}
export interface CallerIdentity {readonly userId: string; readonly externalId: string; readonly applicationId: string; readonly workspaceId: string; readonly sessionId: string; readonly organizationId: string; readonly teamId: string; readonly expiresAt: number; readonly assertionId: string;}
export function verifyCaller(token: string, options: {jwks: {keys: Record<string, unknown>[]}; issuer: string; audience: string; workspaceId?: string; sessionId?: string; now?: number}): CallerIdentity;

export interface SessionRelayOptions<Context> {
 basePath: string;
 authenticate(request: Request): Promise<Context | null>;
 resolve(context: Context, workspaceKey: string): Promise<{client: UserClient; workspaceId: string} | null>;
 maxBodyBytes?: number;
}
export function createSessionRelay<Context>(options: SessionRelayOptions<Context>): {handle(request: Request): Promise<Response>};
