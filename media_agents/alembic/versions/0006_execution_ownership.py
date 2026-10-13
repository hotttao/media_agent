"""Scope execution ownership and reject unfenced writes from legacy processes."""

from alembic import op

revision = "0006_execution_ownership"
down_revision = "0005_agent_requests"
branch_labels = None
depends_on = None


def _execute(sql: str) -> None:
    start = sql.find("CREATE FUNCTION")
    if start >= 0:
        end = sql.index("END $$;", start) + len("END $$;")
        op.execute(sql[start:end])
        sql = sql[:start] + sql[end:]
    for statement in sql.split(";"):
        if statement.strip():
            op.execute(statement)


def upgrade() -> None:
    _execute("""
      ALTER TABLE media_agents.runtime_sessions
        ADD workspace_id varchar(128) NOT NULL DEFAULT '',
        ADD agent_config_id varchar(128) NOT NULL DEFAULT '',
        ADD owner_instance_id varchar(128) NOT NULL DEFAULT '',
        ADD owner_incarnation_id varchar(128) NOT NULL DEFAULT '',
        ADD execution_epoch bigint NOT NULL DEFAULT 1 CHECK(execution_epoch>0),
        ADD last_transfer_id varchar(128) NOT NULL DEFAULT '',
        ADD handoff_id varchar(128) NOT NULL DEFAULT '';
      ALTER TABLE media_agents.turns ADD execution_epoch bigint NOT NULL DEFAULT 1;
      CREATE INDEX runtime_sessions_owner ON
        media_agents.runtime_sessions(workspace_id,agent_config_id,owner_instance_id,owner_incarnation_id);
      CREATE TABLE media_agents.runtime_session_resumes (
        session_id varchar(64) REFERENCES media_agents.runtime_sessions(session_id),
        operation_id varchar(128) NOT NULL, execution_epoch bigint NOT NULL,
        PRIMARY KEY(session_id,operation_id)
      );
      CREATE TABLE media_agents.runtime_instance_admission (
        workspace_id varchar(128) NOT NULL, agent_config_id varchar(128) NOT NULL,
        replica_id varchar(128) NOT NULL, incarnation_id varchar(128) NOT NULL,
        state varchar(16) NOT NULL CHECK(state IN ('active','draining')),
        operation_id varchar(128) NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
        PRIMARY KEY(workspace_id,agent_config_id,replica_id)
      );
    """)
    _execute("""
      CREATE FUNCTION media_agents.guard_execution_owner() RETURNS trigger LANGUAGE plpgsql AS $$
      DECLARE s media_agents.runtime_sessions%ROWTYPE; a
        media_agents.runtime_instance_admission%ROWTYPE;
              ctl boolean; e bigint;
      BEGIN
        IF TG_TABLE_NAME='runtime_sessions' THEN
          s:=NEW;
          IF TG_OP='UPDATE' AND OLD.workspace_id<>'' THEN s:=OLD; END IF;
        ELSE
          SELECT * INTO s FROM media_agents.runtime_sessions WHERE session_id=NEW.session_id;
        END IF;
        IF s.workspace_id='' THEN RETURN NEW; END IF;
        e:=NULLIF(current_setting('media_agent.epoch',true),'')::bigint;
        ctl:=COALESCE(current_setting('media_agent.transfer',true),'false')='true';
        SELECT * INTO a FROM media_agents.runtime_instance_admission
          WHERE workspace_id=s.workspace_id AND agent_config_id=s.agent_config_id
            AND replica_id=current_setting('media_agent.replica',true);
        IF s.workspace_id IS DISTINCT FROM current_setting('media_agent.workspace',true)
          OR s.agent_config_id IS DISTINCT FROM current_setting('media_agent.config',true)
          OR e IS DISTINCT FROM s.execution_epoch
          OR a.incarnation_id IS DISTINCT FROM current_setting('media_agent.incarnation',true)
      OR (NOT ctl AND (s.owner_instance_id IS DISTINCT FROM a.replica_id OR
        s.owner_incarnation_id IS DISTINCT FROM a.incarnation_id)) THEN
          RAISE EXCEPTION 'stale_execution_owner' USING ERRCODE='40001';
        END IF;
        IF TG_TABLE_NAME='runtime_sessions' THEN
          IF NEW.workspace_id<>s.workspace_id OR NEW.agent_config_id<>s.agent_config_id
            OR NEW.context_version<>s.context_version OR NEW.context_ref<>s.context_ref THEN
            RAISE EXCEPTION 'immutable_session_context' USING ERRCODE='40001';
          END IF;
          IF NEW.execution_epoch<>s.execution_epoch OR NEW.owner_instance_id<>s.owner_instance_id
            OR NEW.owner_incarnation_id<>s.owner_incarnation_id THEN
      IF NOT ctl OR NEW.execution_epoch<>s.execution_epoch+1 OR
        NEW.owner_instance_id<>a.replica_id OR NEW.owner_incarnation_id<>a.incarnation_id
        THEN
            RAISE EXCEPTION 'invalid_epoch_transfer' USING ERRCODE='40001';
          END IF;
          END IF;
        END IF;
        RETURN NEW;
      END $$;
      CREATE TRIGGER execution_owner BEFORE INSERT OR UPDATE ON
        media_agents.runtime_sessions FOR EACH ROW EXECUTE FUNCTION
        media_agents.guard_execution_owner();
      CREATE TRIGGER execution_owner BEFORE INSERT OR UPDATE ON media_agents.turns FOR
        EACH ROW EXECUTE FUNCTION media_agents.guard_execution_owner();
      CREATE TRIGGER execution_owner BEFORE INSERT OR UPDATE ON
        media_agents.universal_events FOR EACH ROW EXECUTE FUNCTION
        media_agents.guard_execution_owner();
      CREATE TRIGGER execution_owner BEFORE INSERT OR UPDATE ON
        media_agents.agent_requests FOR EACH ROW EXECUTE FUNCTION
        media_agents.guard_execution_owner();
      CREATE TRIGGER execution_owner BEFORE INSERT OR UPDATE ON media_agents.messages
        FOR EACH ROW EXECUTE FUNCTION media_agents.guard_execution_owner();
    """)


def downgrade() -> None:
    for table in ("runtime_sessions", "turns", "universal_events", "agent_requests", "messages"):
        op.execute(f"DROP TRIGGER execution_owner ON media_agents.{table}")
    op.execute("DROP FUNCTION media_agents.guard_execution_owner()")
    op.execute("DROP TABLE media_agents.runtime_instance_admission")
    op.execute("DROP TABLE media_agents.runtime_session_resumes")
    op.execute("DROP INDEX media_agents.runtime_sessions_owner")
    op.execute("ALTER TABLE media_agents.turns DROP COLUMN execution_epoch")
    for name in (
        "workspace_id",
        "agent_config_id",
        "owner_instance_id",
        "owner_incarnation_id",
        "execution_epoch",
        "last_transfer_id",
        "handoff_id",
    ):
        op.execute(f"ALTER TABLE media_agents.runtime_sessions DROP COLUMN {name}")
