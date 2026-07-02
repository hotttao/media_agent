// Package hello 存放 hello 领域的稳定业务核心：实体、值对象、仓储接口。
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM SDK。
package hello

// Greeting 是一次问候的领域实体。
type Greeting struct {
	Name string
	Body string
}

// NewGreeting 根据名称构造问候。名称为空时回退到 "world"。
func NewGreeting(name string) Greeting {
	if name == "" {
		name = "world"
	}
	return Greeting{
		Name: name,
		Body: "Hello, " + name,
	}
}
