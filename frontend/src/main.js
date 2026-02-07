// main.js - Wails frontend JavaScript

let isRunning = false;
let logPollInterval = null;
let records = [];
let activeRecordId = null;

function showMessage(text, type) {
    const msg = document.getElementById('message');
    msg.textContent = text;
    msg.className = 'message ' + type;
    setTimeout(() => { msg.className = 'message'; }, 5000);
}

function getConfig() {
    return {
        record_name: document.getElementById('recordName')?.value || '',
        active_id: activeRecordId,
        ssh_host: document.getElementById('sshHost').value,
        ssh_port: parseInt(document.getElementById('sshPort').value) || 22,
        ssh_user: document.getElementById('sshUser').value,
        ssh_password: document.getElementById('sshPassword').value,
        ssh_key_path: document.getElementById('sshKeyPath')?.value || '',
        proxy_port: parseInt(document.getElementById('proxyPort').value) || 8080,
        remote_port: parseInt(document.getElementById('remotePort').value) || 8080,
        http_proxy: document.getElementById('httpProxy').value,
        https_proxy: document.getElementById('httpsProxy').value,
        log_level: document.getElementById('logLevel').value,
    };
}

function setFormDisabled(disabled) {
    document.querySelectorAll('#configForm input').forEach(input => {
        input.disabled = disabled;
    });
    const recordSelect = document.getElementById('recordSelect');
    if (recordSelect) {
        recordSelect.disabled = disabled;
    }
    ['newRecordBtn', 'saveRecordBtn', 'deleteRecordBtn'].forEach(id => {
        const button = document.getElementById(id);
        if (button) {
            button.disabled = disabled;
        }
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

        updateCommandPorts(config.remote_port);

        // Show command card immediately when starting
        document.getElementById('commandCard').style.display = 'block';

        // Start polling logs
        startLogPolling();
        const updated = await window.go.main.App.GetRecords();
        if (updated) {
            updateRecordsFromResponse(updated);
        }
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
        records = config.records || [];
        activeRecordId = config.active_id || (records[0] ? records[0].id : null);
        renderRecordSelect();
        const activeRecord = records.find(record => record.id === activeRecordId);
        if (activeRecord) {
            applyRecordToForm(activeRecord);
        } else {
            applyConfigToForm(config);
        }
    } catch (err) {
        console.error('Load config error:', err);
    }
}

function buildRecordLabel(record) {
    const name = (record.name || '').trim();
    if (name) {
        return name;
    }
    const user = record.ssh_user || '';
    const host = record.ssh_host || '';
    const port = record.ssh_port || 22;
    let label = user && host ? `${user}@${host}` : host || user || '未命名记录';
    if (port) {
        label += `:${port}`;
    }
    return label;
}

function renderRecordSelect() {
    const select = document.getElementById('recordSelect');
    if (!select) {
        return;
    }
    const options = records.map(record => {
        const label = buildRecordLabel(record);
        return `<option value="${record.id}">${label}</option>`;
    });
    if (!activeRecordId) {
        options.unshift('<option value="">新建记录</option>');
    }
    select.innerHTML = options.join('');
    select.value = activeRecordId || '';
    updateRecordButtons();
}

function updateRecordButtons() {
    const deleteBtn = document.getElementById('deleteRecordBtn');
    if (deleteBtn) {
        deleteBtn.disabled = !activeRecordId;
    }
}

function applyRecordToForm(record) {
    document.getElementById('recordName').value = record.name || '';
    document.getElementById('sshHost').value = record.ssh_host || '';
    document.getElementById('sshPort').value = record.ssh_port || 22;
    document.getElementById('sshUser').value = record.ssh_user || '';
    document.getElementById('proxyPort').value = record.proxy_port || 8080;
    document.getElementById('remotePort').value = record.remote_port || 8080;
    document.getElementById('httpProxy').value = record.http_proxy || '';
    document.getElementById('httpsProxy').value = record.https_proxy || '';
    document.getElementById('logLevel').value = record.log_level || 'INFO';
    updateCommandPorts(record.remote_port || 8080);
}

function applyConfigToForm(config) {
    document.getElementById('recordName').value = config.record_name || '';
    if (config.ssh_host) document.getElementById('sshHost').value = config.ssh_host;
    if (config.ssh_port) document.getElementById('sshPort').value = config.ssh_port;
    if (config.ssh_user) document.getElementById('sshUser').value = config.ssh_user;
    if (config.proxy_port) document.getElementById('proxyPort').value = config.proxy_port;
    if (config.http_proxy) document.getElementById('httpProxy').value = config.http_proxy;
    if (config.https_proxy) document.getElementById('httpsProxy').value = config.https_proxy;
    const remotePort = config.remote_port || 8080;
    if (config.remote_port) document.getElementById('remotePort').value = config.remote_port;
    updateCommandPorts(remotePort);
}

function updateCommandPorts(port) {
    document.getElementById('cmdPort').textContent = port;
    document.getElementById('cmdPort2').textContent = port;
}

function buildRecordFromForm() {
    const config = getConfig();
    return {
        id: config.active_id,
        name: config.record_name,
        ssh_host: config.ssh_host,
        ssh_port: config.ssh_port,
        ssh_user: config.ssh_user,
        ssh_key_path: config.ssh_key_path,
        proxy_port: config.proxy_port,
        remote_port: config.remote_port,
        http_proxy: config.http_proxy,
        https_proxy: config.https_proxy
    };
}

function resetNewRecordForm() {
    activeRecordId = null;
    document.getElementById('recordName').value = '';
    document.getElementById('sshHost').value = '';
    document.getElementById('sshPort').value = 22;
    document.getElementById('sshUser').value = '';
    document.getElementById('sshPassword').value = '';
    document.getElementById('proxyPort').value = 8080;
    document.getElementById('remotePort').value = 8080;
    document.getElementById('httpProxy').value = '';
    document.getElementById('httpsProxy').value = '';
    updateCommandPorts(8080);
    renderRecordSelect();
}

function updateRecordsFromResponse(response) {
    records = response.records || [];
    activeRecordId = response.active_id || (records[0] ? records[0].id : null);
    renderRecordSelect();
    const activeRecord = records.find(record => record.id === activeRecordId);
    if (activeRecord) {
        applyRecordToForm(activeRecord);
    }
}

async function saveRecord() {
    try {
        const record = buildRecordFromForm();
        const response = await window.go.main.App.SaveRecord(record);
        if (response) {
            updateRecordsFromResponse(response);
        }
        showMessage('记录已保存', 'success');
    } catch (err) {
        showMessage('保存失败: ' + err, 'error');
    }
}

async function deleteRecord() {
    if (!activeRecordId) {
        showMessage('请选择要删除的记录', 'error');
        return;
    }
    try {
        const response = await window.go.main.App.DeleteRecord(activeRecordId);
        if (response) {
            updateRecordsFromResponse(response);
        }
        showMessage('记录已删除', 'success');
    } catch (err) {
        showMessage('删除失败: ' + err, 'error');
    }
}

function newRecord() {
    resetNewRecordForm();
}

async function clearLogs() {
    try {
        await window.go.main.App.ClearLogs();
        document.getElementById('logContainer').innerHTML = '';
        updateLogs();
    } catch (err) {
        console.error('Clear logs error:', err);
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    loadConfig();
    updateStatus();
    setInterval(updateStatus, 2000);
    const recordSelect = document.getElementById('recordSelect');
    if (recordSelect) {
        recordSelect.addEventListener('change', async (event) => {
            const selectedId = event.target.value;
            if (!selectedId) {
                resetNewRecordForm();
                return;
            }
            activeRecordId = selectedId;
            renderRecordSelect();
            const record = records.find(item => item.id === selectedId);
            if (record) {
                applyRecordToForm(record);
            }
            try {
                await window.go.main.App.SetActiveRecord(selectedId);
            } catch (err) {
                console.error('Set active record error:', err);
            }
        });
    }
});

// Make functions available globally
window.startTunnel = startTunnel;
window.stopTunnel = stopTunnel;
window.copyCommand = copyCommand;
window.newRecord = newRecord;
window.saveRecord = saveRecord;
window.deleteRecord = deleteRecord;
window.clearLogs = clearLogs;
