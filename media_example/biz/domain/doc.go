// Package domain 是稳定业务核心：实体、值对象、领域错误、领域事件、仓储接口、稳定业务不变量。
//
// 禁止 import：Hertz、hertz_gen/model、biz/dal、biz/infra、logger/config/Redis/Consul/GORM/OpenTelemetry SDK。
// 同层 feature 隔离：domain/<featureA> 不得 import domain/<featureB>，跨 feature 装配放 service/workflow。
package domain
