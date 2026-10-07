# 单样本Workflow定义工具

只创建/发布Template与Workflow，不Submit Job、不启动Worker、不迁移历史执行。

新Provider契约须显式选择：

```bash
go -C media_workflow run ./tools/media_ai_workflow_migrate \
  --target-url http://192.168.2.41:18080 \
  --image-capability jimeng.i2i.multi_image_prompt@1 \
  --video-capability doubao.i2v.first_frame_prompt@1 \
  --workflow-name 'Provider Capability · 即梦图片与豆包视频单样本'
```

User API Token通过`MEDIA_WORKFLOW_BEARER_TOKEN`环境传入，不放命令行、不打印。提示词默认读取`/home/tao/code/github/gpt_image2/prompts`，可通过`--prompt-dir`覆盖。`--dry-run`只输出定义，不写API。图像模板各有两张图片映射，因此不能选择只接受一张的首帧生图模式；视频接受i2v首帧或多图模式，本样本只传一张首帧。

前三个模板共用指定生图Capability，第四个使用指定生视频Capability；模板名追加契约引用，保留旧样本。输入参数为prompt/images，业务输出分类保持model_image/style_image/first_frame/ai_video。两项能力都省略时是旧fixture兼容模式，不表示自动选择真实Provider。

发布返回异步回执，工具只观察已创建Workflow收敛，不因观察超时另建或重发publish。归档旧定义只有显式`--archive-workflow-id`才执行，本轮未使用。执行门禁和独立客户端准备见[测试规范](../../../harness/contributing/e2e.md)与[Worker文档](../../../media_worker/README.md)。
