# Claude Proxy - SSH 反向隧道代理

<p align="center">
  <img src="screenshots/app-icon.png" alt="Claude Proxy Logo" width="128"/>
</p>

<p align="center">
  <strong>让局域网内的电脑也能使用 Claude API</strong>
</p>

## 📖 软件介绍

Claude Proxy 是一款桌面应用程序，通过 SSH 反向隧道技术，让无法直接访问互联网的电脑也能使用 Claude API 和 Claude Code。

### 适用场景

- **企业内网开发**：公司电脑只能访问局域网，无法直接连接互联网
- **安全隔离环境**：生产服务器、开发测试机等需要网络隔离的环境
- **离线工作站**：科研、金融等对网络访问有严格限制的工作环境
- **多机协作**：一台电脑有网络，其他电脑通过它共享 Claude API 访问

### 工作原理

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│  Claude API │◄────────│  A 电脑     │────────►│  B电脑      │
│  (互联网)    │  HTTPS  │  (可上网)    │   SSH   │  (局域网)    │
└─────────────┘         └─────────────┘         └─────────────┘
```

- **A 电脑**：可以访问互联网，运行 Claude Proxy 应用
- **B 电脑**：在局域网内，无法访问互联网，需要使用 Claude Code

**数据流向：**
```
B电脑 Claude Code (localhost:8080)
    ↓
SSH 反向隧道 (B:8080 → A:8080)
    ↓
A电脑 代理服务 (8080)
    ↓
Claude API (api.anthropic.com)
```

## 🚀 快速开始

### 第一步：在 A 电脑上运行应用

1. 下载并解压对应系统的安装包
   - Windows: `claude-proxy.exe`
   - macOS: `claude-proxy.app`
   - Linux: `claude-proxy`

2. 双击运行应用

![应用主界面](screenshots/main-interface.png)

### 第二步：配置 SSH 连接

在应用界面中填写 B 电脑的连接信息：

![配置界面](screenshots/config-form.png)

| 配置项 | 说明 | 示例 |
|--------|------|------|
| SSH 主机 | B 电脑的 IP 地址或域名 | `192.168.1.100` |
| SSH 端口 | B 电脑的 SSH 服务端口 | `22` |
| SSH 用户 | B 电脑的登录用户名 | `ubuntu` |
| SSH 密码 | 登录密码（可选，推荐使用密钥） | |
| SSH 密钥路径 | 私钥文件路径（可选） | `~/.ssh/id_rsa` |
| 代理端口 | A 电脑上的代理服务端口 | `8080` |
| 远程端口 | B 电脑上的隧道端口 | `8080` |

**提示：** 推荐使用 SSH 密钥认证，更安全且无需每次输入密码。

### 第三步：启动连接

点击 **「启动连接」** 按钮，等待状态指示灯变为绿色。

![连接成功状态](screenshots/connected-status.png)

连接成功后，界面会显示：
- ✅ 绿色状态指示灯
- 📋 B 电脑上需要执行的配置命令
- 📝 实时运行日志

![命令提示卡片](screenshots/command-cards.png)

### 第四步：在 B 电脑上配置代理

连接成功后，在 B 电脑的终端中执行以下命令：

```bash
# 设置代理环境变量（端口与「远程端口」一致）
export HTTP_PROXY=http://127.0.0.1:8080
export HTTPS_PROXY=http://127.0.0.1:8080

# 测试代理连接
curl -x $HTTPS_PROXY https://httpbin.org/ip

# 使用 Claude Code
claude
```

### 第五步：永久配置（可选）

为了避免每次都要设置环境变量，可以将代理配置添加到 shell 配置文件：

**Bash 用户：**
```bash
echo 'export HTTP_PROXY=http://127.0.0.1:8080' >> ~/.bashrc
echo 'export HTTPS_PROXY=http://127.0.0.1:8080' >> ~/.bashrc
source ~/.bashrc
```

**Zsh 用户：**
```bash
echo 'export HTTP_PROXY=http://127.0.0.1:8080' >> ~/.zshrc
echo 'export HTTPS_PROXY=http://127.0.0.1:8080' >> ~/.zshrc
source ~/.zshrc
```

## 🎯 应用界面功能

![功能说明](screenshots/features-overview.png)

| 功能区域 | 说明 |
|---------|------|
| **状态指示器** | 🔴 红色：未连接 / 🟡 黄色：连接中 / 🟢 绿色：已连接 |
| **配置表单** | 填写 SSH 连接信息和端口设置 |
| **启动/停止按钮** | 一键控制隧道连接 |
| **运行日志** | 实时查看连接过程、错误信息和调试信息 |
| **命令提示卡片** | 连接成功后显示 B 电脑需要执行的命令 |

## 🔐 SSH 认证方式

程序支持多种 SSH 认证方式，按以下优先级自动尝试：

1. **SSH Agent** - 如果系统有 ssh-agent 运行（最推荐）
2. **指定密钥** - 使用界面中填写的「SSH 密钥路径」
3. **默认密钥** - 自动查找 `~/.ssh/id_rsa`、`~/.ssh/id_ed25519`、`~/.ssh/id_ecdsa`、`~/.ssh/id_dsa`
4. **密码认证** - 使用界面中填写的密码

### 配置 SSH 密钥认证（推荐）

```bash
# 在 A 电脑上生成密钥（如果还没有）
ssh-keygen -t ed25519 -C "your_email@example.com"

# 将公钥复制到 B 电脑
ssh-copy-id username@b-computer-ip

# 测试免密登录
ssh username@b-computer-ip
```

配置成功后，在应用中只需填写 SSH 主机、端口和用户名即可，无需填写密码。

## ❓ 常见问题

### 连接失败怎么办？

1. **检查网络连通性**
   ```bash
   # 在 A 电脑上测试能否 SSH 到 B 电脑
   ssh username@b-computer-ip
   ```

2. **检查 SSH 服务状态**
   ```bash
   # 在 B 电脑上检查 SSH 服务
   sudo systemctl status sshd   # Linux
   sudo systemsetup -getremotelogin  # macOS
   ```

3. **查看应用日志**

   应用界面底部的运行日志会显示详细的错误信息，帮助定位问题。

![错误日志示例](screenshots/error-logs.png)

### 代理不工作怎么办？

1. **确认隧道状态**

   检查应用界面状态指示灯是否为绿色（已连接）。

2. **检查端口监听**
   ```bash
   # 在 B 电脑上检查端口是否监听
   netstat -tlnp | grep 8080
   # 或
   ss -tlnp | grep 8080
   ```

3. **测试代理连接**
   ```bash
   # 在 B 电脑上测试代理
   curl -x http://127.0.0.1:8080 https://httpbin.org/ip
   ```

### Claude Code 无法使用？

1. **确认环境变量**
   ```bash
   echo $HTTP_PROXY
   echo $HTTPS_PROXY
   ```

2. **确认端口一致**

   环境变量中的端口号必须与应用配置中的「远程端口」一致。

3. **重新设置环境变量**
   ```bash
   export HTTP_PROXY=http://127.0.0.1:8080
   export HTTPS_PROXY=http://127.0.0.1:8080
   claude
   ```

### 如何修改端口？

如果默认的 8080 端口被占用，可以在应用配置中修改：
- **代理端口**：A 电脑上的端口，可以任意修改
- **远程端口**：B 电脑上的端口，修改后需要同步更新 B 电脑的环境变量

## 📁 配置文件

应用配置自动保存在用户目录下的 `~/.claude-proxy.json`：

```json
{
  "ssh_host": "192.168.1.100",
  "ssh_port": 22,
  "ssh_user": "ubuntu",
  "ssh_key_path": "",
  "proxy_port": 8080,
  "remote_port": 8080
}
```

**安全说明：** 出于安全考虑，SSH 密码和密钥密码不会保存到配置文件，每次启动时需要重新输入。

## 🔒 安全特性

- **本地监听**：代理服务仅在 127.0.0.1 上监听，不会暴露给外部网络
- **密码不保存**：SSH 密码和密钥密码不会写入配置文件
- **自动重连**：连接断开后自动尝试重新连接（5秒间隔）
- **连接超时**：设置合理的连接和空闲超时，防止资源占用

## 📊 技术特性

- **跨平台支持**：Windows、macOS、Linux
- **自动重连**：网络波动时自动恢复连接
- **实时日志**：详细的运行日志帮助排查问题
- **配置持久化**：自动保存配置，下次启动无需重新填写
- **多种认证**：支持密码、密钥、SSH Agent 等多种认证方式

## 📝 许可证

MIT License

---

<p align="center">
  如有问题或建议，欢迎提交 Issue
</p>
