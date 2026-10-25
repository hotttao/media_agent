#!/usr/bin/env python3
"""Prepare this repository's existing local Docker installation for Agent product use.

Requires the running Auth, Control, Gateway and PostgreSQL Compose projects.
Keeps existing shared service credentials; creates private runtime keys and databases.
No user/model credentials are read, printed or injected by this command.
"""
from pathlib import Path
import os,subprocess,json,secrets,base64,hashlib,uuid
root=Path(__file__).resolve().parent.parent
private=Path(os.environ.get('AGENT_PRODUCT_DATA_DIR','/data/agent-product')).expanduser().resolve()
private.mkdir(parents=True,exist_ok=True,mode=0o700)
env_path=private/'product.env'
values={}
if env_path.exists():
 values=dict(line.split('=',1) for line in env_path.read_text().splitlines() if '=' in line and not line.startswith('#'))
def env_of(name):
 data=json.loads(subprocess.check_output(['docker','inspect',name]))[0]
 return dict(value.split('=',1) for value in data['Config']['Env'] if '=' in value)
auth=env_of('media_auth-media_auth-1');control=env_of('media_control-media_control-1');gateway=env_of('media_agent_gateway-media_agent_gateway-1')
for key,source in [('MEDIA_AUTH_INTERNAL_TOKEN',auth),('CONTROL_INTERNAL_TOKEN',control),('MEDIA_AUTH_AGENT_GATEWAY_TOKEN',gateway)]:
 values.setdefault(key,source.get(key) or secrets.token_urlsafe(32))
for key in ['AGENT_METADATA_GATEWAY_CREDENTIAL','MCP_CONTROL_CREDENTIAL','MCP_PLUGIN_GATEWAY_CREDENTIAL','MEDIA_AUTH_MCP_PLUGIN_TOKEN','MCP_CREDENTIAL_ENCRYPTION_KEY','AGENT_RUNTIME_DOCKER_INTERNAL_CREDENTIAL','RABBITMQ_PASSWORD']:
 values.setdefault(key,secrets.token_urlsafe(32))
keys=root/'media_agent_gateway/.secrets';keys.mkdir(exist_ok=True,mode=0o700)
signing=keys/'runtime-signing.pem'
if not signing.exists():
 subprocess.run(['openssl','genpkey','-algorithm','ED25519','-out',str(signing)],check=True,capture_output=True);signing.chmod(0o600)
pub=subprocess.check_output(['openssl','pkey','-in',str(signing),'-pubout','-outform','DER'])[-32:]
pub_path=private/'runtime-public-keys.json';pub_path.write_text(json.dumps({'keys':{'rt-'+hashlib.sha256(pub).hexdigest()[:16]:base64.urlsafe_b64encode(pub).decode().rstrip('=')}}));pub_path.chmod(0o644)
ring=keys/'session-keys.json'
if not ring.exists():
 ring.write_text(json.dumps({'active':'local-v1','keys':{'local-v1':base64.urlsafe_b64encode(secrets.token_bytes(32)).decode().rstrip('=')}}));ring.chmod(0o600)
runtime_root=private/'runtime';runtime_root.mkdir(exist_ok=True,mode=0o700)
values.update({'AGENT_METADATA_ENABLED':'true','AGENT_RUNTIME_ENABLED':'true','AGENT_WORKLOAD_ENABLED':'true','AGENT_MCP_ENABLED':'true','AGENT_WORKLOAD_DRIVER':'docker','AGENT_GATEWAY_URL':'http://media_agent_gateway:8006','MEDIA_PLUGIN_URL':'http://host.docker.internal:8011','AGENT_RUNTIME_DOCKER_IDENTITY_URL':'http://runtime-controller:8010','AGENT_RUNTIME_CREDENTIAL_KEYS_FILE':'/run/secrets/agent-gateway/session-keys.json','AGENT_RUNTIME_SIGNING_KEY_FILE':'/run/secrets/agent-gateway/runtime-signing.pem','AGENT_SIGNING_KEY_FILE':'/run/secrets/agent-gateway/agent-signing.pem','RUNTIME_PRODUCT_ROOT':str(runtime_root),'RUNTIME_PUBLIC_KEYS_HOST_FILE':str(pub_path),'RUNTIME_UID':str(os.getuid()),'RUNTIME_GID':str(os.getgid()),'DOCKER_GID':str(os.stat('/var/run/docker.sock').st_gid),'RABBITMQ_USER':'agent_product','AGENT_CODEX_IMAGE':'media-agent-codex:local','AGENT_CLAUDE_IMAGE':'media-agent-claude-code:local','AGENT_GOOSE_IMAGE':'media-agent-goose:local'})
values['MEDIA_AGENTS_RABBITMQ_URL']=f'amqp://{values["RABBITMQ_USER"]}:{values["RABBITMQ_PASSWORD"]}@product-broker:5672/'
# This host administration command creates one local environment without changing existing placements.
values.setdefault('AGENT_ENVIRONMENT_ID',str(uuid.uuid4()))
env_path.write_text(''.join(f'{key}={value}\n' for key,value in sorted(values.items())));env_path.chmod(0o600)
policy={'EnvironmentID':values['AGENT_ENVIRONMENT_ID'],'Network':'media-auth','Root':str(runtime_root),'PublicKeysFile':str(pub_path),'SharedSecret':'media-agent-runtime','DatabaseURL':'postgresql+asyncpg://media_auth:media_auth@172.27.0.1:5432/media-agents','RabbitURL':values['MEDIA_AGENTS_RABBITMQ_URL'],'Images':{'codex':values['AGENT_CODEX_IMAGE'],'claude-code':values['AGENT_CLAUDE_IMAGE'],'goose':values['AGENT_GOOSE_IMAGE']},'AgentTokens':{}}
for field, variable in [('HTTPProxy','AGENT_HTTP_PROXY'),('HTTPSProxy','AGENT_HTTPS_PROXY'),('NoProxy','AGENT_NO_PROXY')]:
 if values.get(variable): policy[field]=values[variable]
(private/'runtime-policy.json').write_text(json.dumps(policy,indent=2));(private/'runtime-policy.json').chmod(0o600)
for database in ['media-plugin','media-agents']:
 result=subprocess.check_output(['docker','exec','docker-compose-postgres-1','psql','-U','media_auth','-d','postgres','-Atc',"SELECT datname FROM pg_database WHERE datname='"+database+"'"],text=True).strip()
 if not result:
  subprocess.run(['docker','exec','docker-compose-postgres-1','psql','-U','media_auth','-d','postgres','-c','CREATE DATABASE "'+database+'" OWNER media_auth'],check=True,stdout=subprocess.DEVNULL)
sql="INSERT INTO runtime_environments(id,name,endpoint,capacity,status,version,created_at,updated_at) VALUES ('"+str(uuid.UUID(values['AGENT_ENVIRONMENT_ID']))+"','Agent local Docker','http://runtime-controller:8010','{}','active',1,now(),now()) ON CONFLICT(id) DO NOTHING"
subprocess.run(['docker','exec','docker-compose-postgres-1','psql','-U','media_auth','-d','media-control','-c',sql],check=True,stdout=subprocess.DEVNULL)
print('Private deployment material prepared at',private,'; no secrets printed.')
