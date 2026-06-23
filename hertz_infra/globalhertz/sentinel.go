// sentinel.go：进程级 Sentinel 全局状态初始化（限流 / 熔断共用底座）。
//
// sentinel-golang 是全局单例状态（内部 datasource / slot chain），server 限流与
// client 熔断共享同一套全局规则注册表，故初始化收口在 globalhertz，进程级一次。
//
// 对齐官方示例（hertz-examples/sentinel）：
//
//	err := sentinel.InitDefault()
//
// 之后由 serverhertz/ratelimit.go 加载 flow 规则、clienthertz/circuitbreaker.go
// 加载 circuitbreaker 规则。
package globalhertz

import (
	"log"
	"sync"

	sentinel "github.com/alibaba/sentinel-golang/api"
)

var sentinelOnce sync.Once

func initSentinel() error {
	var err error
	sentinelOnce.Do(func() {
		if e := sentinel.InitDefault(); e != nil {
			err = e
			log.Printf("globalhertz: sentinel init err: %v", e)
		}
	})
	return err
}
