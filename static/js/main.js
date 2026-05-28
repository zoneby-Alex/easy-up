// static/js/main.js

document.addEventListener('DOMContentLoaded', function() {
    let currentPath = '';
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

    // 事件监听器
    browseBtn.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', handleFileSelect);

    if (createFolderBtn) {
        createFolderBtn.addEventListener('click', async () => {
            const folderName = prompt('请输入新文件夹名称：');
            if (!folderName) return;
            
            try {
                const res = await fetch('/api/create_folder', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ path: currentPath, folder_name: folderName })
                });
                const data = await res.json();
                if (data.success) {
                    showMessage(data.message, 'success');
                    loadFiles();
                } else {
                    showMessage(data.message, 'error');
                }
            } catch (e) {
                showMessage('创建文件夹失败', 'error');
            }
        });
    }

    function updateBreadcrumb() {
        if (!breadcrumb) return;
        if (!currentPath) {
            breadcrumb.innerHTML = '<span style="cursor:pointer;" onclick="navigateTo(\'\')">🏠 首页</span>';
        } else {
            let html = '<span style="cursor:pointer;" onclick="navigateTo(\'\')">🏠 首页</span>';
            const parts = currentPath.split('/');
            let accumPath = '';
            parts.forEach((part) => {
                if (!part) return;
                accumPath += (accumPath ? '/' : '') + part;
                html += ` / <span style="cursor:pointer;" onclick="navigateTo('${accumPath}')">${part}</span>`;
            });
            breadcrumb.innerHTML = html;
        }
    }

    window.navigateTo = function(path) {
        currentPath = path;
        loadFiles();
    };

    window.enterFolder = function(folderName) {
        if (currentPath) {
            currentPath += '/' + folderName;
        } else {
            currentPath = folderName;
        }
        loadFiles();
    };

    // 拖放事件
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, preventDefaults, false);
    });

    function preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    ['dragenter', 'dragover'].forEach(eventName => {
        dropArea.addEventListener(eventName, highlight, false);
    });

    ['dragleave', 'drop'].forEach(eventName => {
        dropArea.addEventListener(eventName, unhighlight, false);
    });

    function highlight() {
        dropArea.classList.add('dragover');
    }

    function unhighlight() {
        dropArea.classList.remove('dragover');
    }

    // 处理文件拖放
    dropArea.addEventListener('drop', handleDrop, false);

    function handleDrop(e) {
        const dt = e.dataTransfer;
        const files = dt.files;
        handleFiles(files);
    }

    function handleFileSelect(e) {
        const files = e.target.files;
        handleFiles(files);
    }

    function handleFiles(files) {
        [...files].forEach(uploadFile);
    }

    async function uploadFile(file) {
        // 验证文件类型
        const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/bmp', 'image/webp', 'image/svg+xml'];
        if (!allowedTypes.includes(file.type)) {
            showMessage(`文件 "${file.name}" 类型不被支持，请选择图片文件`, 'error');
            return;
        }

        const formData = new FormData();
        formData.append('file', file);

        try {
            showStatus('上传中...', 'loading');
            progressContainer.style.display = 'flex';

            const xhr = new XMLHttpRequest();

            // 监听上传进度
            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    const percentComplete = (e.loaded / e.total) * 100;
                    updateProgress(Math.round(percentComplete));
                }
            });

            // 处理响应
            xhr.addEventListener('load', () => {
                if (xhr.status === 200) {
                    const response = JSON.parse(xhr.responseText);
                    if (response.success) {
                        showMessage(`文件 "${file.name}" 上传成功！`, 'success');
                        loadFiles(); // 重新加载文件列表
                    } else {
                        showMessage(`上传失败: ${response.message}`, 'error');
                    }
                } else {
                    showMessage(`上传失败: ${xhr.statusText}`, 'error');
                }
                progressContainer.style.display = 'none';
            });

            // 处理错误
            xhr.addEventListener('error', () => {
                showMessage(`上传失败: 网络错误`, 'error');
                progressContainer.style.display = 'none';
            });

            xhr.open('POST', '/api/upload?path=' + encodeURIComponent(currentPath));
            xhr.send(formData);
        } catch (error) {
            showMessage(`上传失败: ${error.message}`, 'error');
            progressContainer.style.display = 'none';
        }
    }

    function updateProgress(percent) {
        progressFill.style.width = percent + '%';
        progressText.textContent = percent + '%';
    }

    // 加载文件列表
    async function loadFiles() {
        try {
            updateBreadcrumb();
            const response = await fetch(`/api/files?path=${encodeURIComponent(currentPath)}`);
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

    // 渲染文件列表
    function renderFiles(files) {
        filesGrid.innerHTML = '';

        if (files.length === 0) {
            filesGrid.innerHTML = '<p style="grid-column: 1 / -1; text-align: center; color: #7f8c8d;">暂无文件或空文件夹</p>';
            return;
        }

        // 文件夹优先
        files.sort((a, b) => {
            if (a.isDir === b.isDir) return a.name.localeCompare(b.name);
            return a.isDir ? -1 : 1;
        });

        files.forEach(file => {
            const fileItem = document.createElement('div');
            fileItem.className = 'file-item';

            if (file.isDir) {
                fileItem.innerHTML = `
                    <div class="file-icon" style="cursor:pointer;" onclick="enterFolder('${file.name}')">📁</div>
                    <div class="file-info">
                        <h3 title="${file.name}" style="cursor:pointer; color:#3498db;" onclick="enterFolder('${file.name}')">${truncateFileName(file.name, 20)}</h3>
                        <p>文件夹</p>
                        <p>${formatDate(file.ModTime)}</p>
                    </div>
                    <div class="file-actions">
                        <button class="action-btn delete-btn" onclick="deleteFile('${file.name}')">删除</button>
                    </div>
                `;
            } else {
                let fileContent = '';
                let actionButtons = '';
                const ext = file.name.split('.').pop().toLowerCase();
                const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp'];

                if (imageExts.includes(ext)) {
                    fileContent = `<img src="${file.URL}" alt="${file.name}" class="preview" style="display:none;" data-loaded="false" onload="this.dataset.loaded='true';" onerror="this.dataset.error='true';"><div class="file-icon default-icon">🖼️</div>`;
                    actionButtons = `<button class="action-btn preview-btn" onclick="togglePreview(this)">预览</button>`;
                } else {
                    fileContent = getFileIcon(ext);
                }

                const fileSize = formatFileSize(file.Size);

                fileItem.innerHTML = `
                    ${fileContent}
                    <div class="file-info">
                        <h3 title="${file.name}">${truncateFileName(file.name, 20)}</h3>
                        <p>${fileSize}</p>
                        <p>${formatDate(file.ModTime)}</p>
                    </div>
                    <div class="file-actions">
                        ${actionButtons}
                        <button class="action-btn download-btn" onclick="window.location.href='${file.URL}'">下载</button>
                        <button class="action-btn delete-btn" onclick="deleteFile('${file.name}')">删除</button>
                    </div>
                `;
            }
            filesGrid.appendChild(fileItem);
        });
    }

    // 获取文件类型图标
    function getFileIcon(ext) {
        const icons = {
            'jpg': '🖼️', 'jpeg': '🖼️', 'png': '🖼️', 'gif': '🖼️', 'bmp': '🖼️', 'webp': '🖼️',
            'pdf': '📄', 'doc': '📝', 'docx': '📝', 'txt': '📋',
            'zip': '📦', 'rar': '📦', '7z': '📦',
            'mp3': '🎵', 'wav': '🎵', 'flac': '🎵',
            'mp4': '🎬', 'avi': '🎬', 'mov': '🎬',
            'folder': '📁'
        };
        return `<div class="file-icon">${icons[ext] || '📄'}</div>`;
    }

    // 截断文件名
    function truncateFileName(name, maxLength) {
        if (name.length <= maxLength) return name;
        return name.substr(0, maxLength - 3) + '...';
    }

    // 格式化文件大小
    function formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    // 格式化日期
    function formatDate(dateString) {
        const date = new Date(dateString);
        return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
    }

    // 切换图片预览
    window.togglePreview = function(btn) {
        const fileItem = btn.closest('.file-item');
        const img = fileItem.querySelector('img.preview');
        const icon = fileItem.querySelector('.default-icon');
        
        if (img.style.display === 'none') {
            img.style.display = 'block';
            if (icon) icon.style.display = 'none';
            btn.textContent = '隐藏';
            btn.classList.add('active');
            btn.style.background = '#f39c12';
            btn.style.color = 'white';
        } else {
            img.style.display = 'none';
            if (icon) icon.style.display = 'block';
            btn.textContent = '预览';
            btn.classList.remove('active');
            btn.style.background = '';
            btn.style.color = '';
        }
    };

    // 删除文件或文件夹
    window.deleteFile = async function(filename) {
        if (!confirm(`确定要删除 "${filename}" 吗？此操作不可撤销。如果这是文件夹，将删除其内部所有内容！`)) {
            return;
        }

        try {
            const response = await fetch(`/api/delete/${encodeURIComponent(filename)}?path=${encodeURIComponent(currentPath)}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                const result = await response.json();
                showMessage(result.message, 'success');
                loadFiles(); // 重新加载文件列表
            } else {
                const error = await response.json();
                showMessage(error.message || '删除失败', 'error');
            }
        } catch (error) {
            showMessage(`删除失败: ${error.message}`, 'error');
        }
    };

    // 显示状态消息
    function showMessage(message, type) {
        statusMessage.textContent = message;
        statusMessage.className = `status-message ${type}`;

        // 显示消息
        statusMessage.classList.add('show');

        // 3秒后隐藏
        setTimeout(() => {
            statusMessage.classList.remove('show');
        }, 3000);
    }

    function showStatus(message, type) {
        statusMessage.textContent = message;
        statusMessage.className = `status-message ${type}`;
        statusMessage.classList.add('show');
    }

    // 页面加载时获取文件列表
    loadFiles();

    // 每30秒自动刷新一次文件列表
    setInterval(loadFiles, 30000);
});
