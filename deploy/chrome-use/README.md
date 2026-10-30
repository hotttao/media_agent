# chrome-use

`bash deploy/chrome-use/install.sh`：安装已验收的 v1.5.266 至 `/home/tao/deploy/chrome-use/bin`，校验固定安装器 SHA-256，安装器再检查 Release checksum（上游 checksum 缺失时会提示跳过）。
同版本重装使用相同脚本；升级需同时修改脚本中的 tag 和安装器哈希，并验收 image-use bridge。
脚本跳过浏览器/Skill 安装，不主动重启 Chrome，不改当前登录态。

安装后由桌面用户显式执行：

```bash
/home/tao/deploy/chrome-use/bin/chrome-use extension install
/home/tao/deploy/chrome-use/bin/chrome-use doctor --quick --offline
```

在当前 Chrome 启用扩展并授权，登录 ChatGPT。native-messaging manifest 必须位于 Chrome 规定的用户目录，这是浏览器协议入口；不要搬进仓库。
CLI 升级后在空闲时重载扩展，使 native host 使用新二进制。PATH 必须能找到该版本 `chrome-use`；保留旧命令时先显式处理冲突。
Worker 自身持久化状态放 `/data/media-worker/gpt`，Chrome 现有用户 profile 原地保留。

[官方安装说明](https://github.com/leeguooooo/chrome-use#installation)。
