// static/js/main.js

document.addEventListener('DOMContentLoaded', function() {
    let currentPath = '';
    let refreshTimer = null;

    const loginSection = document.getElementById('loginSection');
    const appSection = document.getElementById('appSection');
    const loginForm = document.getElementById('loginForm');
    const usernameInput = document.getElementById('usernameInput');
    const passwordInput = document.getElementById('passwordInput');
    const currentUser = document.getElementById('currentUser');
    const logoutBtn = document.getElementById('logoutBtn');
    const dropArea = document.getElementById('dropArea');
    const fileInput = document.getElementById('fileInput');
    const browseBtn = document.getElementById('browseBtn');
    const filesGrid = document.getElementById('filesGrid');
    const progressContainer = document.getElementById('progressContainer');
    const progressFill = document.getElementById('progressFill');
    const progressText = document.getElementById('progressText');
    const statusMessage = document.getElementById('statusMessage');
    const breadcrumb = document.getElementById('breadcrumb');
    const createFolderBtn = document.getElementById('createFolderBtn');

    loginForm.addEventListener('submit', handleLogin);
    logoutBtn.addEventListener('click', handleLogout);
    browseBtn.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', handleFileSelect);
    createFolderBtn.addEventListener('click', createFolder);

    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, preventDefaults, false);
    });
    ['dragenter', 'dragover'].forEach(eventName => {
        dropArea.addEventListener(eventName, () => dropArea.classList.add('dragover'), false);
    });
    ['dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, () => dropArea.classList.remove('dragover'), false);
    });
    dropArea.addEventListener('drop', handleDrop, false);

    checkSession();

    async function checkSession() {
        try {
            const response = await fetch('/api/session');
            const data = await response.json();
            if (data.authenticated) {
                showApp(data.username);
            } else {
                showLogin();
            }
        } catch (error) {
            showLogin();
            showMessage('无法检查登录状态', 'error');
        }
    }

    async function handleLogin(event) {
        event.preventDefault();
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    username: usernameInput.value,
                    password: passwordInput.value
                })
            });
            const data = await response.json();
            if (!response.ok || !data.success) {
                showMessage(data.message || '登录失败', 'error');
                return;
            }
            passwordInput.value = '';
            showApp(data.username);
        } catch (error) {
            showMessage('登录失败', 'error');
        }
    }

    async function handleLogout() {
        try {
            await fetch('/api/logout', { method: 'POST' });
        } finally {
            showLogin();
        }
    }

    function showLogin() {
        loginSection.hidden = false;
        appSection.hidden = true;
        currentPath = '';
        filesGrid.replaceChildren();
        if (refreshTimer) {
            clearInterval(refreshTimer);
            refreshTimer = null;
        }
    }

    function showApp(username) {
        loginSection.hidden = true;
        appSection.hidden = false;
        currentUser.textContent = username ? `当前用户：${username}` : '';
        loadFiles();
        if (!refreshTimer) {
            refreshTimer = setInterval(loadFiles, 30000);
        }
    }

    async function createFolder() {
        const folderName = prompt('请输入新文件夹名称：');
        if (!folderName) return;

        try {
            const res = await fetch('/api/create_folder', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ path: currentPath, folder_name: folderName })
            });
            const data = await res.json();
            if (res.ok && data.success) {
                showMessage(data.message, 'success');
                loadFiles();
            } else {
                showMessage(data.message || '创建文件夹失败', 'error');
            }
        } catch (e) {
            showMessage('创建文件夹失败', 'error');
        }
    }

    function updateBreadcrumb() {
        breadcrumb.replaceChildren();
        const home = makeLinkButton('首页', () => navigateTo(''));
        breadcrumb.appendChild(home);

        if (!currentPath) return;

        const parts = currentPath.split('/').filter(Boolean);
        let accumPath = '';
        parts.forEach(part => {
            accumPath += (accumPath ? '/' : '') + part;
            const targetPath = accumPath;
            breadcrumb.appendChild(document.createTextNode(' / '));
            breadcrumb.appendChild(makeLinkButton(part, () => navigateTo(targetPath)));
        });
    }

    function makeLinkButton(text, onClick) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'link-btn';
        button.textContent = text;
        button.addEventListener('click', onClick);
        return button;
    }

    function navigateTo(path) {
        currentPath = path;
        loadFiles();
    }

    function enterFolder(folderName) {
        currentPath = currentPath ? `${currentPath}/${folderName}` : folderName;
        loadFiles();
    }

    function preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    function handleDrop(e) {
        handleFiles(e.dataTransfer.files);
    }

    function handleFileSelect(e) {
        handleFiles(e.target.files);
        fileInput.value = '';
    }

    function handleFiles(files) {
        [...files].forEach(uploadFile);
    }

    async function uploadFile(file) {
        const allowedExts = [
            'jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp',
            'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx',
            'md', 'markdown', 'txt', 'csv', 'json', 'xml', 'yaml', 'yml',
            'go', 'py', 'js', 'ts', 'jsx', 'tsx', 'java', 'c', 'cpp', 'h', 'hpp',
            'cs', 'rs', 'php', 'rb', 'sh', 'ps1', 'bat', 'sql', 'html', 'css',
            'zip', 'rar', '7z', 'tar', 'gz', 'tgz', 'bz2', 'xz'
        ];
        const ext = file.name.split('.').pop().toLowerCase();
        if (!allowedExts.includes(ext)) {
            showMessage(`文件 "${file.name}" 类型不被支持`, 'error');
            return;
        }

        const formData = new FormData();
        formData.append('file', file);

        try {
            showStatus('上传中...', 'loading');
            progressContainer.style.display = 'flex';

            const xhr = new XMLHttpRequest();
            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    updateProgress(Math.round((e.loaded / e.total) * 100));
                }
            });

            xhr.addEventListener('load', () => {
                let response = {};
                try {
                    response = JSON.parse(xhr.responseText || '{}');
                } catch (error) {
                    response = {};
                }
                if (xhr.status === 401) {
                    showMessage('登录已失效，请重新登录', 'error');
                    showLogin();
                } else if (xhr.status >= 200 && xhr.status < 300 && response.success) {
                    const finalName = response.name || file.name;
                    showMessage(`文件 "${finalName}" 上传成功！`, 'success');
                    loadFiles();
                } else {
                    showMessage(`上传失败: ${response.message || xhr.statusText}`, 'error');
                }
                progressContainer.style.display = 'none';
                updateProgress(0);
            });

            xhr.addEventListener('error', () => {
                showMessage('上传失败: 网络错误', 'error');
                progressContainer.style.display = 'none';
                updateProgress(0);
            });

            xhr.open('POST', '/api/upload?path=' + encodeURIComponent(currentPath));
            xhr.send(formData);
        } catch (error) {
            showMessage(`上传失败: ${error.message}`, 'error');
            progressContainer.style.display = 'none';
            updateProgress(0);
        }
    }

    function updateProgress(percent) {
        progressFill.style.width = percent + '%';
        progressText.textContent = percent + '%';
    }

    async function loadFiles() {
        try {
            updateBreadcrumb();
            const response = await fetch(`/api/files?path=${encodeURIComponent(currentPath)}`);
            if (response.status === 401) {
                showMessage('登录已失效，请重新登录', 'error');
                showLogin();
                return;
            }
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const files = await response.json();
            renderFiles(files);
        } catch (error) {
            console.error('加载文件列表失败:', error);
            showMessage('加载文件列表失败', 'error');
        }
    }

    function renderFiles(files) {
        filesGrid.replaceChildren();

        if (files.length === 0) {
            const empty = document.createElement('p');
            empty.className = 'empty-state';
            empty.textContent = '暂无文件或空文件夹';
            filesGrid.appendChild(empty);
            return;
        }

        files.sort((a, b) => {
            if (a.isDir === b.isDir) return a.name.localeCompare(b.name);
            return a.isDir ? -1 : 1;
        });

        files.forEach(file => filesGrid.appendChild(createFileItem(file)));
    }

    function createFileItem(file) {
        const fileItem = document.createElement('div');
        fileItem.className = 'file-item';

        if (file.isDir) {
            const icon = document.createElement('button');
            icon.type = 'button';
            icon.className = 'file-icon folder-icon';
            icon.textContent = '📁';
            icon.addEventListener('click', () => enterFolder(file.name));
            fileItem.appendChild(icon);

            fileItem.appendChild(createInfo(file.name, '文件夹', formatDate(file.ModTime), () => enterFolder(file.name)));
            fileItem.appendChild(createActions([{ label: '删除', className: 'delete-btn', onClick: () => deleteFile(file.name, true) }]));
            return fileItem;
        }

        const ext = file.name.split('.').pop().toLowerCase();
        const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp'];
        let previewImage = null;
        let defaultIcon = null;

        if (imageExts.includes(ext)) {
            previewImage = document.createElement('img');
            previewImage.src = file.URL;
            previewImage.alt = file.name;
            previewImage.className = 'preview';
            previewImage.hidden = true;

            defaultIcon = document.createElement('div');
            defaultIcon.className = 'file-icon default-icon';
            defaultIcon.textContent = '🖼️';
            fileItem.appendChild(previewImage);
            fileItem.appendChild(defaultIcon);
        } else {
            const icon = document.createElement('div');
            icon.className = 'file-icon';
            icon.textContent = getFileIcon(ext);
            fileItem.appendChild(icon);
        }

        fileItem.appendChild(createInfo(file.name, formatFileSize(file.Size), formatDate(file.ModTime)));

        const actions = [];
        if (previewImage && defaultIcon) {
            actions.push({
                label: '预览',
                className: 'preview-btn',
                onClick: (button) => togglePreview(button, previewImage, defaultIcon)
            });
        }
        actions.push({ label: '下载', className: 'download-btn', onClick: () => { window.location.href = file.URL; } });
        actions.push({ label: '删除', className: 'delete-btn', onClick: () => deleteFile(file.name, false) });
        fileItem.appendChild(createActions(actions));
        return fileItem;
    }

    function createInfo(name, line1, line2, onTitleClick) {
        const info = document.createElement('div');
        info.className = 'file-info';

        const title = document.createElement(onTitleClick ? 'button' : 'h3');
        if (onTitleClick) {
            title.type = 'button';
            title.className = 'file-title-btn';
            title.addEventListener('click', onTitleClick);
        }
        title.title = name;
        title.textContent = truncateFileName(name, 20);
        info.appendChild(title);

        const first = document.createElement('p');
        first.textContent = line1;
        info.appendChild(first);

        const second = document.createElement('p');
        second.textContent = line2;
        info.appendChild(second);

        return info;
    }

    function createActions(actions) {
        const actionWrap = document.createElement('div');
        actionWrap.className = 'file-actions';

        actions.forEach(action => {
            const button = document.createElement('button');
            button.type = 'button';
            button.className = `action-btn ${action.className}`;
            button.textContent = action.label;
            button.addEventListener('click', () => action.onClick(button));
            actionWrap.appendChild(button);
        });

        return actionWrap;
    }

    function getFileIcon(ext) {
        const icons = {
            jpg: '🖼️', jpeg: '🖼️', png: '🖼️', gif: '🖼️', bmp: '🖼️', webp: '🖼️',
            pdf: '📄', doc: '📝', docx: '📝', txt: '📋',
            xls: '📊', xlsx: '📊', csv: '📊',
            ppt: '📽️', pptx: '📽️',
            md: '📋', markdown: '📋', json: '{}', xml: '<>', yaml: 'YML', yml: 'YML',
            go: 'GO', py: 'PY', js: 'JS', ts: 'TS', jsx: 'JSX', tsx: 'TSX',
            java: 'JAVA', c: 'C', cpp: 'C++', h: 'H', hpp: 'H++', cs: 'CS',
            rs: 'RS', php: 'PHP', rb: 'RB', sh: 'SH', ps1: 'PS', bat: 'BAT',
            sql: 'SQL', html: 'HTML', css: 'CSS',
            zip: '📦', rar: '📦', '7z': '📦', tar: '📦', gz: '📦', tgz: '📦', bz2: '📦', xz: '📦',
            mp3: '🎵', wav: '🎵', flac: '🎵',
            mp4: '🎬', avi: '🎬', mov: '🎬'
        };
        return icons[ext] || '📄';
    }

    function truncateFileName(name, maxLength) {
        if (name.length <= maxLength) return name;
        return name.substr(0, maxLength - 3) + '...';
    }

    function formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    function formatDate(dateString) {
        const date = new Date(dateString);
        return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
    }

    function togglePreview(btn, img, icon) {
        if (img.hidden) {
            img.hidden = false;
            icon.hidden = true;
            btn.textContent = '隐藏';
            btn.classList.add('active');
        } else {
            img.hidden = true;
            icon.hidden = false;
            btn.textContent = '预览';
            btn.classList.remove('active');
        }
    }

    async function deleteFile(filename, isDir) {
        const promptText = isDir
            ? `确定要删除空文件夹 "${filename}" 吗？非空文件夹不会被删除。`
            : `确定要删除文件 "${filename}" 吗？此操作不可撤销。`;
        if (!confirm(promptText)) return;

        try {
            const response = await fetch(`/api/delete/${encodeURIComponent(filename)}?path=${encodeURIComponent(currentPath)}`, {
                method: 'DELETE'
            });
            const result = await response.json();

            if (response.status === 401) {
                showMessage('登录已失效，请重新登录', 'error');
                showLogin();
            } else if (response.ok && result.success) {
                showMessage(result.message, 'success');
                loadFiles();
            } else {
                showMessage(result.message || '删除失败', 'error');
            }
        } catch (error) {
            showMessage(`删除失败: ${error.message}`, 'error');
        }
    }

    function showMessage(message, type) {
        statusMessage.textContent = message;
        statusMessage.className = `status-message ${type}`;
        statusMessage.classList.add('show');

        setTimeout(() => {
            statusMessage.classList.remove('show');
        }, 3000);
    }

    function showStatus(message, type) {
        statusMessage.textContent = message;
        statusMessage.className = `status-message ${type}`;
        statusMessage.classList.add('show');
    }
});
