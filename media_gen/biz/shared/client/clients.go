package client

import jobsvc "media_agent/hertz_gen/media_workflow/job_service"

// 此处声明全局下游 client 变量。
//
// 命名规则：<AppPascalCase><ServicePascalCase>Client
//
// media_workflow.job_service：供 generation.Repository 实现调 ClaimStepRun / CompleteStepRun / FailStepRun。
// 实例由 Init() 装配。
var MediaWorkflowJobClient jobsvc.Client
