package main

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	router "media_agent/media_materials/biz/router"
)

func register(r *server.Hertz) {
	router.GeneratedRegister(r)
	customizedRegister(r)
}
