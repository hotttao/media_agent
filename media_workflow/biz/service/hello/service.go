// Package hello 是 hello 用例服务层。
// 接受 command，编排 domain 完成用例，禁止 import Hertz / hertz_gen / biz/dal / biz/infra。
package hello

import (
	"context"

	"media_agent/media_workflow/biz/domain/hello"
)

// Service 是 hello 用例服务。通过 NewService 构造，由 main.go 注入 handler。
type Service struct{}

// NewService 构造 hello 用例服务。
// 生产可替换为 wire 注入 dal/infra 依赖。
func NewService() *Service {
	return &Service{}
}

// SayHelloCommand 是 SayHello 用例的入参。
type SayHelloCommand struct {
	Name string
}

// SayHello 执行问候用例：调用 domain 构造 Greeting，返回其 Body。
func (s *Service) SayHello(ctx context.Context, cmd SayHelloCommand) (string, error) {
	g := hello.NewGreeting(cmd.Name)
	return g.Body, nil
}
