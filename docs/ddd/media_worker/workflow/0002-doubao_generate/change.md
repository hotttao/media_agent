# 豆包生成流程变更

## 2026-10-26 · 明确 Provider 边界

用户要求将 generate 正名为 doubao_generate；保留专用 Studio 生成流程，不引入 GPT Recipe 或共用生成状态机。检查点仓储命名为 doubao_checkpoints，物理文件格式和旧目录不变。

2026-10-29：增加真实 multi_image_prompt 执行路径，复用原检查点与协议，保留首帧单图限制；逐图隔离下载并保持顺序，Studio 逐图验证素材归属。真实网页验收单独记录。
