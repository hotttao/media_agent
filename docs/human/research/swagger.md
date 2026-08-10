在 media_example 下增加一个  swagger 目录，makefile 增加一个 swagger-gen 目录，用于生成 swagger 文档。

参考:
swagger-gen:
	protoc --proto_path=idl \
		--openapi_out=swagger \
		--openapi_opt=naming=proto \

在 server suite 添加一个默认的 swagger handler，用于生成 swagger 相关的api 接口文档，默认给所有 server 加上。

```
	h.GET("/swagger/", func(c context.Context, ctx *app.RequestContext) {
		ctx.Redirect(consts.StatusMovedPermanently, []byte("/swagger/index.html"))
	})

	h.GET("/swagger/*any", swagger.WrapHandler(
		swaggerFiles.Handler,
		swagger.URL("/openapi.yaml"),
	))

	h.GET("/openapi.yaml", func(c context.Context, ctx *app.RequestContext) {
		ctx.Header("Content-Type", "application/x-yaml")
		ctx.Write(openapiYAML)
	})
```

再给 media-example 添加一个 skill-gen 目录，用于生成 skill 文档。skills 生活目录: skills 以服务名为目录名。

参考命令:

```
skill-gen:
	cd .claude/skills && npx openapi-to-skills@latest ../../swagger/openapi.yaml -o . --groupBy=path --name media-materials-api --force 2>&1
```