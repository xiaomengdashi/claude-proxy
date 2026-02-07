# Claude Proxy 技术原理深度解析

本文档详细解释了 Claude Proxy 的工作原理、与 VPN 的区别、HTTPS 穿透机制以及在企业内网中的应用场景。

## 1. SSH 反向代理原理 (Reverse Tunneling)

**核心概念**：利用 A 电脑（外网可达）作为跳板，让内网受限的 B 电脑能够访问互联网。

```mermaid
graph LR
    subgraph PC_B ["B电脑 (受限环境)"]
        App["应用程序\n(curl/git/pip)"] -- "1. 请求" --> Listener["监听端口\n(127.0.0.1:8890)"]
        Listener -- "2. 进入隧道" --> SSH_Channel["SSH 通道"]
    end

    subgraph Internet ["互联网"]
        Server["目标服务器\n(api.anthropic.com)"]
    end

    subgraph PC_A ["A电脑 (可访问外网)"]
        SSH_Client["Claude Proxy\n(SSH 客户端)"] -- "3. 接收数据" --> LocalProxy["本地代理服务\n(127.0.0.1:8080)"]
        LocalProxy -- "4. 发起请求" --> Server
    end

    SSH_Channel <==> SSH_Client
    Server -- "5. 响应数据" --> LocalProxy
    LocalProxy -- "6. 返回数据" --> SSH_Client
    SSH_Client -- "7. 返回隧道" --> Listener
    Listener -- "8. 返回响应" --> App
```

### 关键步骤：
1.  **建立隧道**：Claude Proxy (A) 主动连接 B 电脑的 SSH 服务，并请求**反向端口转发** (`-R`)。
2.  **只有 TCP**：目前仅支持 TCP 协议，不支持 UDP（如 DNS 查询、QUIC）。
3.  **身份伪装**：对于目标服务器来说，访问者是 A 电脑，B 电脑是完全隐形的。

---

## 2. HTTPS 穿透流程 (The "CONNECT" Method)

当 B 电脑请求 HTTPS 网站（如 `https://baidu.com`）时，发生的是**盲传**。

```mermaid
sequenceDiagram
    autonumber
    participant AppB as B电脑 (curl)
    participant LocalProxy as A电脑 (代理)
    participant Baidu as 百度服务器

    Note over AppB, LocalProxy: 阶段一：建立 TCP 通路 (明文)
    AppB->>LocalProxy: CONNECT baidu.com:443 HTTP/1.1
    LocalProxy->>Baidu: [TCP SYN] 建立连接
    Baidu-->>LocalProxy: [TCP SYN-ACK]
    LocalProxy-->>AppB: HTTP/1.1 200 Connection Established

    Note over AppB, Baidu: 阶段二：TLS 加密传输 (密文)
    AppB->>Baidu: [Client Hello] 发送 TLS 握手
    Note right of LocalProxy: A 电脑只转发，看不懂内容
    Baidu-->>AppB: [Server Hello] 返回证书
    AppB->>Baidu: [加密数据] GET /
    Baidu-->>AppB: [加密数据] (HTML内容)
```

*   **A 电脑角色**：拨线员 + 搬运工。
*   **隐私性**：A 电脑只能看到你访问了 *哪个域名* (baidu.com)，但无法通过抓包看到你 *请求的具体路径* 或 *页面内容*。

---

## 3. Proxy vs VPN：核心区别

| 特性 | Proxy (Claude Proxy) | VPN (虚拟专用网) |
| :--- | :--- | :--- |
| **工作层级** | <span style="color:blue">应用层/会话层 (L7/L5)</span> | <span style="color:orange">网络层 (L3)</span> |
| **比喻** | **代购** (你把清单给他) | **修路** (把家门口的路改了) |
| **适用范围** | 仅限支持代理的软件 (浏览器, curl) | 全局所有软件 (系统更新, 游戏) |
| **协议支持** | 通常仅 TCP (SOCKS5 可支持 UDP) | 全协议 (TCP/UDP/ICMP/Ping) |
| **配置方式** | 每个软件单独配 (`export HTTP_PROXY`) | 连接后自动接管整个系统 |
| **UDP支持** | **不支持** | **支持** (适合游戏、DNS) |

```mermaid
graph TD
    subgraph B_Computer ["B电脑"]
        App1["Chrome"]
        App2["Ping"]
        App3["游戏 (UDP)"]
    
        subgraph Layer4_Proxy ["代理模式 (Claude Proxy)"]
            Params["环境变量设置"]
            LocalSOCKS["本地监听端口"]
        end
        
        subgraph Layer3_VPN ["VPN模式"]
            VirtualNIC["虚拟网卡 (TUN/TAP)"]
            Routing["路由表规则"]
        end
        
        RealNIC["物理网卡"]
    end

    %% 代理流程
    App1 -- "HTTP请求" --> Params
    Params -- "转发" --> LocalSOCKS
    LocalSOCKS -- "TCP封装" --> RealNIC
    App2 -- "ICMP包 (不走代理)" --> RealNIC
    App3 -- "UDP包 (不走代理)" --> RealNIC

    %% VPN流程
    App1 -.-> VirtualNIC
    App2 -.-> VirtualNIC
    App3 -.-> VirtualNIC
    VirtualNIC -.-> RealNIC

    style Layer4_Proxy fill:#e1f5fe,stroke:#01579b
    style Layer3_VPN fill:#fff3e0,stroke:#ff6f00
```

### VPN 工作流程详解 (https://google.com)

VPN 通过虚拟网卡劫持整个操作系统的网络流量。

```mermaid
sequenceDiagram
    autonumber
    participant App as 浏览器 (B电脑)
    participant OS as 操作系统内核 (B电脑)
    participant NIC_Virtual as 虚拟网卡 (tun0)
    participant VPN_Client as VPN客户端软件 (B电脑)
    participant NIC_Real as 物理网卡 (eth0)
    participant Internet as 互联网
    participant VPN_Server as VPN服务器
    participant Google as Google服务器

    Note over App, Google: 阶段一：发起请求 (不知情的浏览器)
    
    App->>OS: 1. 我要访问 google.com (解析得到 IP 1.2.3.4)
    OS->>OS: 2. 查路由表: 去往 1.2.3.4 的路怎么走？
    Note right of OS: 路由表被 VPN 修改过：<br>默认网关是虚拟网卡 tun0
    OS->>NIC_Virtual: 3. 把 IP 数据包扔给 tun0

    Note over NIC_Virtual, VPN_Server: 阶段二：偷梁换柱 (封装与加密)

    NIC_Virtual->>VPN_Client: 4. 虚拟网卡把数据包传给 VPN 软件
    Note right of NIC_Virtual: 依然是原始 IP 包<br>源:10.8.0.2(内网IP) -> 目:1.2.3.4
    
    VPN_Client->>VPN_Client: 5. 加密整个 IP 包 -> [密文]
    VPN_Client->>VPN_Client: 6. 封装新包头
    Note right of VPN_Client: 变成普通 UDP/TCP 包<br>源:B真实IP -> 目:VPN服务器IP<br>载荷:[密文]
    
    VPN_Client->>NIC_Real: 7. 通过物理网卡发出新包
    NIC_Real->>Internet: 8. 发送数据
    Internet->>VPN_Server: 9. 数据到达服务器

    Note over VPN_Server, Google: 阶段三：还原与转发 (代访问)

    VPN_Server->>VPN_Server: 10. 解密 -> 还原出原始 IP 包
    Note right of VPN_Server: 看到原始包:<br>源:10.8.0.2 -> 目:1.2.3.4 (Google)
    
    VPN_Server->>Google: 11. 做 NAT 转发，发给 Google
    Google-->>VPN_Server: 12. Google 回复 (以为是 VPN服务器 访问的)

    Note over VPN_Server, App: 阶段四：原路返回
    
    VPN_Server->>VPN_Server: 13. 加密回复包 -> 封装
    VPN_Server-->>NIC_Real: 14. 发回给 B 电脑
    NIC_Real->>VPN_Client: 15. VPN 软件收到，解密
    VPN_Client->>NIC_Virtual: 16. 把还原的回复包写入虚拟网卡
    NIC_Virtual->>App: 17. 浏览器收到 Google 的回复
```


