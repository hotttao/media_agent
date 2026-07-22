// Package publish 实现视频发布（mock）。
//
// 本期不调 media_materials / media_crawler，直接返回假 publish_url + SUCCEEDED。
// 真实实现需调 AssetRead 校验 + 调 crawler 平台 API + 异步轮询状态。
package publish

import (
	"fmt"
	"time"

	publishdomain "media_agent/media_gen/biz/domain/publish"
)

// Service 实现视频发布（mock）。
type Service struct{}

// NewService 装配 publish service（无依赖）。
func NewService() *Service { return &Service{} }

// Publish 执行单次发布（mock）。
//
// 流程：
//  1. 校验 asset_id 非空、target 在支持列表内
//  2. mock: 构造 publish_url + platform_publish_id，state=SUCCEEDED
//
// 真实实现需：
//   - 调 media_materials.AssetRead 校验 asset_id 存在 + media_type=VIDEO + 拿 url
//   - 调 media_crawler 暴露的平台发布 API
//   - 异步轮询平台状态（PENDING -> PUBLISHING -> SUCCEEDED/FAILED）
//   - 持久化到 publishes 表（按 asset_id+target 幂等）
func (s *Service) Publish(cmd publishdomain.Command) *publishdomain.Result {
	if cmd.AssetID == "" {
		return &publishdomain.Result{
			AssetID:       "",
			Target:        cmd.Target,
			State:         publishdomain.StateFailed,
			FailureReason: "asset_id is required",
		}
	}
	if !isSupported(cmd.Target) {
		return &publishdomain.Result{
			AssetID:       cmd.AssetID,
			Target:        cmd.Target,
			State:         publishdomain.StateFailed,
			FailureReason: fmt.Sprintf("unsupported target: %s", cmd.Target),
		}
	}
	// mock 直接成功。
	return &publishdomain.Result{
		AssetID:           cmd.AssetID,
		Target:            cmd.Target,
		State:             publishdomain.StateSucceeded,
		PublishURL:        fmt.Sprintf("mock://publish/%s/%s", cmd.Target, cmd.AssetID),
		PlatformPublishID: fmt.Sprintf("mock-%s-%d", cmd.AssetID, time.Now().UnixMilli()),
		PublishedAt:       time.Now().UnixMilli(),
	}
}

// GetSupportedTargets 返回当前支持的发布目标列表。
func (s *Service) GetSupportedTargets() []publishdomain.Target {
	return append([]publishdomain.Target(nil), publishdomain.SupportedTargets...)
}

func isSupported(t publishdomain.Target) bool {
	for _, s := range publishdomain.SupportedTargets {
		if s == t {
			return true
		}
	}
	return false
}
