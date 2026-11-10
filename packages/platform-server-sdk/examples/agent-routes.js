import {createSessionRelay} from '../index.js';

// authenticate must validate the application's own login session.
// teamForUser must select a permitted team from server-side business facts.
export function createAgentRoutes({platform, authenticate, teamForUser}) {
  return createSessionRelay({
    basePath:'/api/agent',
    authenticate,
    async resolve(user, key) {
      if (key !== 'me') return null;
      await platform.upsertUser(user.id,{display_name:user.name});
      const client=platform.as(user.id,{teamId:await teamForUser(user),scopes:['workspace.read','workspace.ensure','session.chat']});
      const workspace=await client.ensureAgent('assistant',{display_name:'Assistant',agent_type:'goose'});
      return {client,workspaceId:workspace.id};
    },
  });
}
// Framework adapter: return routes.handle(theExternalFetchRequest).
// Preserve the external request origin and cancellation signal when adapting HTTP.
