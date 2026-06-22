// loadbalance.go：客户端负载均衡。
//
// loadbalance 与 discovery 紧耦合：sd.Discovery(r, sd.WithLoadBalanceOptions(...))。
// 字符串 → 具体 LoadBalancer 的映射在此处理。
package clienthertz

// pickLoadBalancer 把配置字符串映射为 hertz loadbalance 类型。
// 当前未引入 hertz-contrib/loadbalance 时返回原始字符串供 discovery 层使用。
func pickLoadBalancer(name string) string {
	switch name {
	case "weighted_random", "round_robin", "consistent_hash":
		return name
	default:
		return "weighted_random"
	}
}
