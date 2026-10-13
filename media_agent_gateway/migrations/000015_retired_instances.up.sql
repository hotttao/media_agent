ALTER TABLE agent_instances DROP CONSTRAINT agent_instances_admission_check;
ALTER TABLE agent_instances ADD CONSTRAINT agent_instances_admission_check CHECK(admission IN ('active','draining','retired'));
