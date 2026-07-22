// Package worker 实现 media_gen 进程内 worker 生命周期管理。
//
// worker 与 HTTP server 同进程（in-process goroutine），不独立 listener。
// loop 本期 stub：goroutine 启动后 select { <-ctx.Done() }，不实际跑 Claim 循环。
// 真实实现需替换 loop 为 Claim -> ExecuteOnce -> Complete/Fail 循环 + backoff。
package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	workerdomain "media_agent/media_gen/biz/domain/worker"
)

// DefaultPollIntervalMs 空闲轮询间隔（无 StepRun 时）。
const DefaultPollIntervalMs = 5000

// Service 管理进程内 worker 注册表 + goroutine 生命周期。
//
// 不持久化：进程重启后全部 worker STOPPED，需调用方重新 StartWorker。
type Service struct {
	mu       sync.Mutex
	workers  map[string]*workerEntry
	executor workerdomain.StepExecutor // 由 generation.Service 实现
}

type workerEntry struct {
	info   workerdomain.Info
	cancel context.CancelFunc
	done   chan struct{} // goroutine 退出信号
}

// NewService 装配 worker service。
// executor: 单次 StepRun 执行端口（generation.Service 实现之）。
func NewService(executor workerdomain.StepExecutor) *Service {
	return &Service{
		workers:  make(map[string]*workerEntry),
		executor: executor,
	}
}

// Start 启动一个 worker goroutine。
//
// 幂等：
//   - worker_id 已注册且 state=RUNNING -> ErrWorkerAlreadyRunning
//   - worker_id 已注册且 state=STOPPED -> 复用注册记录重启
func (s *Service) Start(req StartRequest) (*workerdomain.Info, error) {
	if req.WorkerID == "" {
		return nil, errors.New("worker_id is required")
	}
	if req.Platform == "" || req.GenWay == "" {
		return nil, errors.New("platform and gen_way are required")
	}
	pollInterval := int32(req.PollIntervalMs)
	if pollInterval == 0 {
		pollInterval = DefaultPollIntervalMs
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.workers[req.WorkerID]; ok && entry.info.State == workerdomain.StateRunning {
		return nil, fmt.Errorf("worker %s already running", req.WorkerID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	info := workerdomain.Info{
		WorkerID:       req.WorkerID,
		Platform:       req.Platform,
		GenWay:         req.GenWay,
		GenType:        req.GenType,
		State:          workerdomain.StateRunning,
		PollIntervalMs: pollInterval,
		StartedAt:      workerdomain.NowFn().UnixMilli(),
	}

	entry := &workerEntry{info: info, cancel: cancel, done: done}
	s.workers[req.WorkerID] = entry

	go s.loop(ctx, entry)
	return &entry.info, nil
}

// Stop 停止 worker goroutine。
//
// 幂等：state=STOPPED 直接返回当前状态；不存在返回 ErrWorkerNotFound。
func (s *Service) Stop(workerID string) (*workerdomain.Info, error) {
	s.mu.Lock()
	entry, ok := s.workers[workerID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("worker %s not found", workerID)
	}

	entry.cancel()
	<-entry.done // 等 goroutine 退出

	s.mu.Lock()
	defer s.mu.Unlock()
	entry.info.State = workerdomain.StateStopped
	entry.info.StoppedAt = workerdomain.NowFn().UnixMilli()
	return &entry.info, nil
}

// Get 查询单个 worker 状态。
func (s *Service) Get(workerID string) (*workerdomain.Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.workers[workerID]
	if !ok {
		return nil, fmt.Errorf("worker %s not found", workerID)
	}
	info := entry.info
	return &info, nil
}

// List 列出进程内全部 worker（含 STOPPED）。
// stateFilter 非空时按 state 过滤。
func (s *Service) List(stateFilter string) []*workerdomain.Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*workerdomain.Info, 0, len(s.workers))
	for _, entry := range s.workers {
		if stateFilter != "" && string(entry.info.State) != stateFilter {
			continue
		}
		info := entry.info
		out = append(out, &info)
	}
	return out
}

// loop 是 worker goroutine 的主循环。
//
// 本期 stub：启动后立即 select { <-ctx.Done() }，不实际跑 Claim 循环。
// 真实实现需替换为：
//
//	for {
//	    select {
//	    case <-ctx.Done():
//	        close(done)
//	        return
//	    default:
//	    }
//	    err := executor.ExecuteOnce(ctx, info.WorkerID)
//	    // 更新 entry.info.LastClaimAt / LastCompleteAt / LastError
//	    if err != nil {
//	        // backoff: 2x 指数退避，上限 30s
//	    }
//	    select {
//	    case <-ctx.Done():
//	    case <-time.After(pollInterval):
//	    }
//	}
func (s *Service) loop(ctx context.Context, entry *workerEntry) {
	defer close(entry.done)
	log.Printf("worker %s started (stub loop, not actually claiming)", entry.info.WorkerID)
	<-ctx.Done()
	log.Printf("worker %s stopped", entry.info.WorkerID)
}

// StartRequest 是 Start 的入参（对齐 proto StartWorkerReq）。
type StartRequest struct {
	WorkerID       string
	Platform       string
	GenWay         string
	GenType        string
	PollIntervalMs int32
}
