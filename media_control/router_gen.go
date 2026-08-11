package main

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	router "media_agent/media_control/biz/router"
)

func register(r *server.Hertz) {
	router.GeneratedRegister(r)
	customizedRegister(r)
}
