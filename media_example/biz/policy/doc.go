// Package policy 存放易变业务规则：权限决策、定价/折扣、取消/发布规则等。
// policy 可依赖 domain，但不得依赖 Hertz/handler/DTO/DAL/基础设施/其他 policy feature。
// policy 应为纯决策逻辑，不自取数据；由 service/workflow 加载事实后传入。
package policy
