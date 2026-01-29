// main.js - Wails frontend JavaScript

let isRunning = false;
let logPollInterval = null;

function showMessage(text, type) {
    const msg = document.getElementById('message');
    msg.textContent = text;
    msg.className = 'message ' + type;
    setTimeout(() => { msg.className = 'message'; }, 5000);
}

function getConfig() {
    return {
        ssh_host: document.getElementById('sshHost').value,
        ssh_port: parseInt(document.getElementById('sshPort').value) || 22,
        ssh_user: document.getElementById('sshUser').value,
        ssh_password: document.getElementById('sshPassword').value,
        ssh_key_path: document.getElementById('sshKeyPath')?.value || '',
        proxy_port: parseInt(document.getElementById('proxyPort').value) || 8080,
        remote_port: parseInt(document.getElementById('remotePort').value) || 8080,
        http_proxy: document.getElementById('httpProxy').value,
        https_proxy: document.getElementById('httpsProxy').value
    };
}

function setFormDisabled(disabled) {
    document.querySelectorAll('#configForm input').forEach(input => {
        input.disabled = disabled;
    });
}

function updateButtons(running) {
    isRunning = running;
    document.getElementById('startBtn').style.display = running ? 'none' : 'block';
    document.getElementById('stopBtn').style.display = running ? 'block' : 'none';
    setFormDisabled(running);
}

async function startTunnel() {
    const config = getConfig();

    if (!config.ssh_host || !config.ssh_user) {
        showMessage('请填写 SSH 主机和用户名', 'error');
        return;
    }

    document.getElementById('startBtn').disabled = true;
    document.getElementById('startBtn').textContent = '正在连接...';

    try {
        await window.go.main.App.Start(config);
        updateButtons(true);
        document.getElementById('startBtn').textContent = '启动连接';
        document.getElementById('startBtn').disabled = false;

        // Update command port
        document.getElementById('cmdPort').textContent = config.remote_port;
        document.getElementById('cmdPort2').textContent = config.remote_port;

        // Show command card immediately when starting
        document.getElementById('commandCard').style.display = 'block';

        // Start polling logs
        startLogPolling();
    } catch (err) {
        showMessage('启动失败: ' + err, 'error');
        document.getElementById('startBtn').disabled = false;
        document.getElementById('startBtn').textContent = '启动连接';
    }
}

async function stopTunnel() {
    try {
        await window.go.main.App.Stop();
        updateButtons(false);
        document.getElementById('commandCard').style.display = 'none';
        stopLogPolling();
    } catch (err) {
        showMessage('停止失败: ' + err, 'error');
    }
}

async function updateStatus() {
    try {
        const status = await window.go.main.App.GetStatus();
        const proxyDot = document.getElementById('proxyStatus');
        const tunnelDot = document.getElementById('tunnelStatus');
        const tunnelText = document.getElementById('tunnelStatusText');

        proxyDot.className = 'status-dot ' + (status.proxy_running ? 'green' : 'red');

        if (status.tunnel_connected) {
            tunnelDot.className = 'status-dot green';
            tunnelText.textContent = 'SSH 隧道 (已连接)';
            document.getElementById('commandCard').style.display = 'block';
        } else if (status.tunnel_running) {
            tunnelDot.className = 'status-dot yellow';
            tunnelText.textContent = 'SSH 隧道 (连接中...)';
        } else {
            tunnelDot.className = 'status-dot red';
            tunnelText.textContent = 'SSH 隧道 (未连接)';
        }

        // Update button state based on tunnel status
        if (status.tunnel_running && !isRunning) {
            updateButtons(true);
            startLogPolling();
        } else if (!status.tunnel_running && isRunning) {
            updateButtons(false);
        }
    } catch (err) {
        console.error('Status update error:', err);
    }
}

async function updateLogs() {
    try {
        const logs = await window.go.main.App.GetLogs();
        const container = document.getElementById('logContainer');

        if (!logs || logs.length === 0) {
            container.innerHTML = '<div class="log-entry">等待启动...</div>';
            return;
        }

        container.innerHTML = logs.map(log => {
            let className = 'log-entry';
            if (log.includes('Error') || log.includes('error') || log.includes('failed') || log.includes('Failed') || log.includes('错误')) {
                className += ' error';
            } else if (log.includes('success') || log.includes('established') || log.includes('Connected') || log.includes('成功')) {
                className += ' success';
            }
            return '<div class="' + className + '">' + escapeHtml(log) + '</div>';
        }).join('');

        container.scrollTop = container.scrollHeight;
    } catch (err) {
        console.error('Logs update error:', err);
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function copyCommand(elementId) {
    const el = document.getElementById(elementId);
    const text = el.innerText || el.textContent;
    navigator.clipboard.writeText(text).then(() => {
        const btn = el.parentElement.querySelector('.copy-btn');
        if (btn) {
            btn.textContent = '已复制';
            btn.classList.add('copied');
            setTimeout(() => {
                btn.textContent = '复制';
                btn.classList.remove('copied');
            }, 2000);
        }
    }).catch(err => {
        console.error('Copy failed:', err);
    });
}

function startLogPolling() {
    if (!logPollInterval) {
        logPollInterval = setInterval(updateLogs, 1000);
        updateLogs();
    }
}

function stopLogPolling() {
    if (logPollInterval) {
        clearInterval(logPollInterval);
        logPollInterval = null;
    }
}

async function loadConfig() {
    try {
        const config = await window.go.main.App.GetConfig();
        if (config.ssh_host) document.getElementById('sshHost').value = config.ssh_host;
        if (config.ssh_port) document.getElementById('sshPort').value = config.ssh_port;
        if (config.ssh_user) document.getElementById('sshUser').value = config.ssh_user;
        if (config.proxy_port) document.getElementById('proxyPort').value = config.proxy_port;
        if (config.http_proxy) document.getElementById('httpProxy').value = config.http_proxy;
        if (config.https_proxy) document.getElementById('httpsProxy').value = config.https_proxy;

        // Update command box ports
        const remotePort = config.remote_port || 8080;
        document.getElementById('cmdPort').textContent = remotePort;
        document.getElementById('cmdPort2').textContent = remotePort;
        if (config.remote_port) document.getElementById('remotePort').value = config.remote_port;
    } catch (err) {
        console.error('Load config error:', err);
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    loadConfig();
    updateStatus();
    setInterval(updateStatus, 2000);
});

// Make functions available globally
window.startTunnel = startTunnel;
window.stopTunnel = stopTunnel;
window.copyCommand = copyCommand;
