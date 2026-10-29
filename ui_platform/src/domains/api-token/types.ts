export interface ApiToken {
  id: string;
  name: string;
  scopes: string[];
  status: string;
  expires_at: string;
}
export interface TokenDraft {
  name: string;
  scopes: string[];
  expires_at: string;
  idempotency_key: string;
}
