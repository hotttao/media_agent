# image-use

```bash
python3 deploy/image-use/install.py
```

安装至 `/home/tao/deploy/image-use/7f97846e70e1b82b53baac3499f3272e9f5c1aa9/image-use`，使用仓库 bridge 定义的 commit 和 SHA-256，下载后校验，原子替换。
可以 `--source /path/to/downloaded/image-use` 离线安装，同样必须通过哈希校验。
旧 `media_worker/tools/install_image_use.py` 保留兼容入口，实际安装逻辑已迁至这里。

依赖 Python 3、已连接的 chrome-use 与 Chrome。只支持 Worker 强制的 `web` backend，不回退 Codex。
更新必须同时审查 `media_worker/tools/image_use_bridge.py` 的版本/哈希与网页兼容性并回归测试，不能独立升级。
GPT 台账、输入和下载由 Worker 保存在 `/data/media-worker/gpt`。浏览器登录态仍使用当前用户的 Chrome，不新建无登录 headless 会话。

[上游](https://github.com/leeguooooo/image-use)。
