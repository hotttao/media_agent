# Agent 产品功能检查

提供仅用于测试的 PostgreSQL DSN（需有创建测试数据库权限），从仓库根运行：

```bash
MEDIA_AGENT_PRODUCT_TEST_DSN='postgres://user:password@127.0.0.1:5432/postgres?sslmode=disable' \
  GOMAXPROCS=2 go test -p 2 -count=1 ./harness/tools/agent_product_integration
```

测试创建并迁移独立数据库，完成后删除该数据库。Prompt 检查使用真实 Hertz 路由、HTTP、Control 服务、仓库、ACL、版本历史和配置 Release/Outbox；Auth 企业/部门事实是明确 HTTP fixture，不使用真实模型凭据。未提供 DSN 时跳过 live 检查。

Skill 检查覆盖 ZIP/tar.gz、文件编辑、发布/历史、配置选择与权限，并使用本机 Git CLI 和临时 smart HTTP 仓库验证私有凭据、分支、子目录及手动同步；不请求互联网仓库、不执行真实模型。运行该分支需要已安装 Git，临时仓库随测试清理。
