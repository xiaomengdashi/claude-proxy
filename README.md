# Claude Proxy - SSH 反向隧道代理

让无法直接访问互联网的电脑通过 SSH 隧道使用 Claude API。

## 网络拓扑

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│  Claude API │◄────────│  A 电脑     │────────►│  B电脑      │
│  (互联网)    │  HTTPS  │  (可上网)    │   SSH   │  (局域网)    │
└─────────────┘         └─────────────┘         └─────────────┘
```

- **A电脑**：可以访问互联网，运行本代理程序（Wails 桌面应用）
- **B电脑**：在局域网内，无法访问互联网，需要使用 Claude Code

## 工作原理

```
B电脑 Claude Code
    │
    │ localhost:8080 (默认)
    ▼
┌─────────────────────────────┐
│ SSH 反向隧道                │
│ (B:8080 → A:8080)          │
└─────────────────────────────┘
    │
    ▼
A电脑 代理服务 (8080)
    │
    │ HTTPS
    ▼
Claude API (api.anthropic.com)
```

## 编译方法

### macOS

```bash
# 安装 Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 编译
wails build
```

编译产物：`build/bin/claude-proxy.app`

### Windows

```bash
# 安装 Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 编译
wails build
```

编译产物：`build/bin/claude-proxy.exe`

### Linux

```bash
# 安装依赖 (Ubuntu/Debian)
sudo apt install libwebkit2gtk-4.1-dev \
    build-essential \
    pkg-config \
    libgtk-3-dev

# 安装 Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 编译
wails build
```

编译产物：`build/bin/claude-proxy`

## 快速开始

### 1. 运行应用 (在 A 电脑上)

双击运行编译后的应用，或在开发模式下运行：

```bash
wails dev
```

### 2. 配置连接

在应用界面中填写：

| 配置项 | 说明 | 示例 |
|--------|------|------|
| SSH 主机 | B 电脑的 IP 或域名 | `192.168.1.100` |
| SSH 端口 | B 电脑的 SSH 端口 | `22` |
| SSH 用户 | B 电脑的登录用户 | `ubuntu` |
| SSH 密码 | 可选，优先使用密钥 | |
| SSH 密钥路径 | 私钥文件路径 | `~/.ssh/id_rsa` |
| 代理端口 | A 电脑代理服务端口 | `8080` |
| 远程端口 | B 电脑隧道端口 | `8080` |

点击 **「启动连接」** 按钮。

### 3. 使用 Claude Code (在 B 电脑上)

```bash
# 设置代理环境变量（端口与「远程端口」一致）
export HTTP_PROXY=http://127.0.0.1:8080
export HTTPS_PROXY=http://127.0.0.1:8080

# 测试代理连接
curl -x $HTTPS_PROXY https://httpbin.org/ip

# 使用 Claude Code
claude
```

### 4. 永久配置 (可选)

在 B 电脑上添加到 shell 配置文件：

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

## 应用界面功能

| 功能 | 说明 |
|------|------|
| 状态指示 | 绿色/黄色/红色指示灯显示连接状态 |
| 配置表单 | 填写 SSH 连接信息和端口设置 |
| 启动/停止 | 一键控制隧道连接 |
| 运行日志 | 实时查看连接过程和错误信息 |
| B电脑命令 | 连接成功后显示配置命令 |

## SSH 认证

程序按以下顺序尝试认证：

1. **SSH Agent** - 如果系统有 ssh-agent 运行
2. **指定密钥** - 使用「SSH 密钥路径」指定的密钥
3. **默认密钥** - 自动查找 `~/.ssh/id_rsa`、`~/.ssh/id_ed25519`、`~/.ssh/id_ecdsa`、`~/.ssh/id_dsa`
4. **密码认证** - 如果密钥认证失败，使用界面填写的密码

**推荐**：配置 SSH 密钥认证更安全。

### 配置密钥认证

```bash
# 在 A 电脑上生成密钥（如果还没有）
ssh-keygen -t ed25519

# 将公钥复制到 B 电脑
ssh-copy-id user@b-computer-ip

# 测试免密登录
ssh user@b-computer-ip
```

## 故障排除

### 连接失败

1. 确认 A 电脑能 SSH 到 B 电脑：
   ```bash
   ssh user@b-computer-ip
   ```

2. 检查 B 电脑 SSH 服务是否运行：
   ```bash
   sudo systemctl status sshd   # Linux
   sudo systemsetup -getremotelogin  # macOS
   ```

3. 查看应用界面的运行日志，了解具体错误

### 代理不工作

1. 确认隧道状态为绿色（已连接）

2. 在 B 电脑上检查端口是否监听：
   ```bash
   netstat -tlnp | grep 8080
   # 或
   ss -tlnp | grep 8080
   ```

3. 测试代理连接：
   ```bash
   curl -x http://127.0.0.1:8080 https://httpbin.org/ip
   ```

### Claude Code 无法使用

1. 确认环境变量已设置：
   ```bash
   echo $HTTPS_PROXY
   ```

2. 确认代理端口与「远程端口」一致

3. 尝试重新设置环境变量后运行 claude

## 配置文件

配置自动保存在 `~/.claude-proxy.json`：

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

**注意**：出于安全考虑，密码和密钥密码不会保存到配置文件。

## 安全说明

1. **代理仅监听本地** - 代理服务只在 127.0.0.1 上监听，不会暴露给网络
2. **密码不保存** - SSH 密码不会写入配置文件
3. **推荐使用密钥** - 配置 SSH 密钥认证更安全
4. **SSH 主机验证** - 当前版本使用 `InsecureIgnoreHostKey()`，生产环境建议使用 known_hosts

## 项目结构

```
claude-proxy/
├── main.go           # 程序入口
├── app.go            # 应用状态管理
├── config.go         # 配置管理
├── proxy.go          # HTTP 代理服务
├── ssh_tunnel.go     # SSH 隧道管理
├── go.mod            # Go 模块定义
├── wails.json        # Wails 配置
├── frontend/         # 前端资源
│   ├── index.html    # 主页面
│   └── src/
│       ├── main.js   # 前端逻辑
│       └── style.css # 样式
├── build/            # 编译输出
└── setup-b.sh        # B 电脑配置脚本
```

## 许可证

MIT License
