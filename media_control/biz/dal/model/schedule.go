package model

import "time"

type AgentSchedule struct {
	ID, WorkspaceID, RequestedBy, AgentConfigID, Rule, Timezone, Status string
	NextRunAt                                                           *time.Time
	Version                                                             int64
	CreatedAt, UpdatedAt                                                time.Time
}

func (AgentSchedule) TableName() string { return "agent_schedules" }

type ScheduleTrigger struct {
	ID, ScheduleID, Status, AgentTaskID, ErrorCode string
	DueAt, CreatedAt, UpdatedAt                    time.Time
}

func (ScheduleTrigger) TableName() string { return "schedule_triggers" }
