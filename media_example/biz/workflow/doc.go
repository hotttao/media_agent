// Package workflow 存放跨 feature 业务流程编排。
// workflow 可调用多个 service/policy，但不得 import handler/router/生成 DTO/原始 DAL query/外部 SDK 细节。
// 跨系统流程用 saga/outbox/event 模式，不得在调用外部系统时持有数据库事务。
package workflow
