// Package worker 定义 media_gen 进程内 worker 的领域模型。
//
// worker 是媒体生成执行单元：按 (platform, gen_way, gen_type) 拉取 StepRun 执行。
// worker 状态仅进程内可见，不持久化；进程重启后全部 STOPPED。
package worker

import (
	"context"
	"time"
)

// State -- worker 进程内状态机。
type State string

const (
	StateStopped State = "STOPPED"
	StateRunning State = "RUNNING"
)

// Info -- worker 进程内状态视图，对齐 hertz_gen/model/media_gen/worker.WorkerInfo。
type Info struct {
	WorkerID       string
	Platform       string
	GenWay         string
	GenType        string
	State          State
	PollIntervalMs int32
	StartedAt      int64
	StoppedAt      int64
	LastClaimAt    int64
	LastCompleteAt int64
	LastError      string
}

// StepExecutor 是单次 StepRun 执行的端口，由 generation.Service 实现。
// worker loop goroutine 通过此接口调用 generation 域执行一次 Claim -> Generate -> Complete。
//
// 接口而非直引 generation.Service：避免 service/worker -> service/generation 同层 import
// （架构规则：biz/service/<a> ─X-> biz/service/<b>）。
type StepExecutor interface {
	// ExecuteOnce 执行单次 Claim -> 生成(mock) -> Complete/Fail 循环。
	// 返回 nil 表示成功完成一次执行（含 Claim 返回空的无操作场景）。
	ExecuteOnce(ctx context.Context, workerID string) error
}

// NowFn 可注入的时钟，便于测试。生产用 time.Now。
var NowFn = func() time.Time { return time.Now() }
