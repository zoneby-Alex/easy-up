package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type Config struct {
	Port         string `json:"port"`
	UploadDir    string `json:"upload_dir"`
	MaxFileSize  int64  `json:"max_file_size"`
	AllowDeletes bool   `json:"allow_deletes"`
	Title        string `json:"title"`
}

type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"Size"`
	ModTime time.Time `json:"ModTime"`
	URL     string    `json:"URL"`
	IsDir   bool      `json:"isDir"`
}

var config Config

func initConfig() {
	// 默认配置
	config = Config{
		Port:         "8080",
		UploadDir:    "./uploads",
		MaxFileSize:  52428800,
		AllowDeletes: true,
		Title:        "局域网图片传输工具",
	}

	// 尝试从文件读取
	configFile, err := os.ReadFile("config.json")
	if err == nil {
		json.Unmarshal(configFile, &config)
	}

	// 命令行参数覆盖
	port := flag.String("port", "", "指定服务器端口")
	dir := flag.String("dir", "", "指定上传文件目录")
	maxSize := flag.Int64("max-size", 0, "指定最大文件大小")
	title := flag.String("title", "", "指定页面标题")

	flag.Parse()

	if *port != "" {
		config.Port = *port
	}
	if *dir != "" {
		config.UploadDir = *dir
	}
	if *maxSize != 0 {
		config.MaxFileSize = *maxSize
	}
	if *title != "" {
		config.Title = *title
	}

	// 确保上传目录存在
	os.MkdirAll(config.UploadDir, os.ModePerm)
}

func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil && !ipnet.IP.IsLinkLocalUnicast() {
				// 过滤掉 docker, WSL 等常见虚拟网卡 IP 段
				ipStr := ipnet.IP.String()
				if !strings.HasPrefix(ipStr, "172.") && !strings.HasPrefix(ipStr, "192.168.56.") {
					ips = append(ips, ipStr)
				}
			}
		}
	}
	return ips
}

func getSafePath(baseDir, relativePath string) (string, error) {
	cleanPath := filepath.Clean(filepath.Join(baseDir, relativePath))
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTarget, absBase) {
		return "", fmt.Errorf("invalid path")
	}
	return absTarget, nil
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(config.MaxFileSize)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "文件大小超出限制"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "读取文件失败"})
		return
	}
	defer file.Close()

	// 检查文件类型 (只允许图片和部分格式)
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".bmp": true, ".webp": true, ".svg": true,
	}
	if !allowedExts[ext] {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "不支持的文件类型"})
		return
	}

	reqPath := r.URL.Query().Get("path")
	targetDir, err := getSafePath(config.UploadDir, reqPath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "非法路径"})
		return
	}

	// 如果目录不存在则创建
	os.MkdirAll(targetDir, os.ModePerm)

	filename := filepath.Base(header.Filename) // 防路径遍历
	destPath := filepath.Join(targetDir, filename)

	destFile, err := os.Create(destPath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "无法保存文件"})
		return
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "保存文件失败"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "上传成功"})
}

func handleFilesList(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Query().Get("path")
	targetDir, err := getSafePath(config.UploadDir, reqPath)
	if err != nil {
		http.Error(w, "非法路径", http.StatusForbidden)
		return
	}

	var files []FileInfo
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			files = []FileInfo{}
		} else {
			http.Error(w, "无法读取目录", http.StatusInternalServerError)
			return
		}
	} else {
		for _, entry := range entries {
			info, err := entry.Info()
			if err == nil {
				urlPath := "/download/" + info.Name()
				if reqPath != "" {
					// 拼接相对路径
					urlPath = "/download/" + reqPath + "/" + info.Name()
				}
				files = append(files, FileInfo{
					Name:    info.Name(),
					Size:    info.Size(),
					ModTime: info.ModTime(),
					URL:     urlPath,
					IsDir:   entry.IsDir(),
				})
			}
		}
	}

	if files == nil {
		files = []FileInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path       string `json:"path"`
		FolderName string `json:"folder_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "无效的请求格式"})
		return
	}

	folderName := filepath.Base(req.FolderName)
	if folderName == "" || folderName == "." || folderName == "/" || folderName == "\\" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "无效的文件夹名称"})
		return
	}

	targetDir, err := getSafePath(config.UploadDir, filepath.Join(req.Path, folderName))
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "非法路径"})
		return
	}

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "创建文件夹失败"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "创建文件夹成功"})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if !config.AllowDeletes {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "不允许删除文件"})
		return
	}

	reqPath := r.URL.Query().Get("path")
	vars := mux.Vars(r)
	filename := filepath.Base(vars["filename"])

	filePath, err := getSafePath(config.UploadDir, filepath.Join(reqPath, filename))
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "非法路径"})
		return
	}

	if err := os.RemoveAll(filePath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "删除失败"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "删除成功"})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	// r.URL.Path contains the full path including the prefix
	// We extract the subpath after /download/
	subPath := strings.TrimPrefix(r.URL.Path, "/download/")
	if subPath == "" || subPath == "/" {
		http.Error(w, "File not specified", http.StatusBadRequest)
		return
	}

	filePath, err := getSafePath(config.UploadDir, subPath)
	if err != nil {
		http.Error(w, "非法路径", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, filePath)
}

func main() {
	initConfig()

	r := mux.NewRouter()

	// 静态文件和前端页面
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.html")
	})
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// API 路由
	r.HandleFunc("/api/upload", handleUpload).Methods("POST")
	r.HandleFunc("/api/files", handleFilesList).Methods("GET")
	r.HandleFunc("/api/create_folder", handleCreateFolder).Methods("POST")
	r.HandleFunc("/api/delete/{filename}", handleDelete).Methods("DELETE", "OPTIONS")
	// Using PathPrefix for download to handle arbitrary subdirectories
	r.PathPrefix("/download/").HandlerFunc(handleDownload).Methods("GET")

	// 打印启动信息和可用 IP (修复无效链接问题)
	fmt.Printf("easy-up v2 服务器启动在 http://localhost:%s\n", config.Port)
	fmt.Printf("上传目录: %s\n", config.UploadDir)
	fmt.Printf("最大文件大小: %d bytes (%.2f MB)\n", config.MaxFileSize, float64(config.MaxFileSize)/1024/1024)
	fmt.Println("\n可通过以下真实局域网地址访问：")

	ips := getLocalIPs()
	for _, ip := range ips {
		fmt.Printf("  http://%s:%s\n", ip, config.Port)
	}
	if len(ips) == 0 {
		fmt.Println("  (未能检测到有效的局域网 IP，请确认已连接网络)")
	}
	fmt.Println()

	// 启动服务
	log.Fatal(http.ListenAndServe(":"+config.Port, r))
}
