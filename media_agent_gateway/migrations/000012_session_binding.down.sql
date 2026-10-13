DROP INDEX media_agent_gateway.agent_session_token;
DROP INDEX media_agent_gateway.session_binding_owner;
DROP INDEX media_agent_gateway.session_transfer_pending;
ALTER TABLE media_agent_gateway.agent_sessions
 DROP environment_id,DROP replica_id,DROP incarnation_id,DROP binding_version,DROP binding_phase,
 DROP transfer_id,DROP target_replica_id,DROP target_incarnation_id,DROP execution_epoch,
 DROP expected_execution_epoch,DROP token_hash,DROP token_key_id;
