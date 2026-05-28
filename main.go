package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const sessionCookieName = "easy_up_session"

type Config struct {
	Port            string       `json:"port"`
	UploadDir       string       `json:"upload_dir"`
	MaxFileSize     int64        `json:"max_file_size"`
	AllowDeletes    bool         `json:"allow_deletes"`
	Title           string       `json:"title"`
	Users           []UserConfig `json:"users"`
	SessionSecret   string       `json:"session_secret"`
	SessionTTLHours int          `json:"session_ttl_hours"`
}

type UserConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"Size"`
	ModTime time.Time `json:"ModTime"`
	URL     string    `json:"URL"`
	IsDir   bool      `json:"isDir"`
}

var (
	config        Config
	sessionSecret []byte
)

func initConfig() {
	config = Config{
		Port:            "8080",
		UploadDir:       "./uploads",
		MaxFileSize:     52428800,
		AllowDeletes:    true,
		Title:           "局域网图片传输工具",
		Users:           []UserConfig{{Username: "admin", Password: "admin123"}},
		SessionTTLHours: 12,
	}

	configFile, err := os.ReadFile("config.json")
	if err == nil {
		if err := json.Unmarshal(configFile, &config); err != nil {
			log.Printf("读取 config.json 失败，使用默认配置: %v", err)
		}
	}

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
	if config.SessionTTLHours <= 0 {
		config.SessionTTLHours = 12
	}
	if len(config.Users) == 0 {
		log.Println("警告: 未配置 users，所有登录请求都会失败")
	}

	sessionSecret = []byte(config.SessionSecret)
	if len(sessionSecret) == 0 {
		sessionSecret = make([]byte, 32)
		if _, err := rand.Read(sessionSecret); err != nil {
			sessionSecret = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
		}
		log.Println("session_secret 未配置，已生成临时密钥；服务重启后需要重新登录")
	}

	if err := os.MkdirAll(config.UploadDir, os.ModePerm); err != nil {
		log.Fatalf("无法创建上传目录: %v", err)
	}
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
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid path")
	}
	return absTarget, nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("JSON 响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"success": false, "message": message})
}

func authenticate(username, password string) bool {
	if username == "" || strings.Contains(username, "|") {
		return false
	}
	for _, user := range config.Users {
		if user.Username == username && subtle.ConstantTimeCompare([]byte(user.Password), []byte(password)) == 1 {
			return true
		}
	}
	return false
}

func signSessionPayload(payload string) string {
	mac := hmac.New(sha256.New, sessionSecret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func createSessionToken(username string) string {
	expires := time.Now().Add(time.Duration(config.SessionTTLHours) * time.Hour).Unix()
	payload := username + "|" + strconv.FormatInt(expires, 10)
	signature := signSessionPayload(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))
}

func parseSessionToken(token string) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return "", false
	}
	username, expiryText, signature := parts[0], parts[1], parts[2]
	if username == "" || strings.Contains(username, "|") {
		return "", false
	}
	expires, err := strconv.ParseInt(expiryText, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}
	expected := signSessionPayload(username + "|" + expiryText)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return "", false
	}
	return username, true
}

func currentUsername(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return parseSessionToken(cookie.Value)
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUsername(r); !ok {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		next(w, r)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}
	if !authenticate(req.Username, req.Password) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    createSessionToken(req.Username),
		Path:     "/",
		MaxAge:   config.SessionTTLHours * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "username": req.Username})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	if username, ok := currentUsername(r); ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": true, "username": username})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
}

func allowedUploadExt(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return true
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return true
	case ".md", ".markdown", ".txt", ".csv", ".json", ".xml", ".yaml", ".yml":
		return true
	case ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".c", ".cpp", ".h", ".hpp", ".cs", ".rs", ".php", ".rb", ".sh", ".ps1", ".bat", ".sql", ".html", ".css":
		return true
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz":
		return true
	default:
		return false
	}
}

func downloadURL(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, segment := range strings.Split(filepath.ToSlash(part), "/") {
			if segment != "" {
				escaped = append(escaped, url.PathEscape(segment))
			}
		}
	}
	return "/download/" + strings.Join(escaped, "/")
}

func uniqueFilePath(dir, filename string) (string, string, error) {
	baseName := filepath.Base(filename)
	if baseName == "." || baseName == string(os.PathSeparator) || baseName == "" {
		return "", "", errors.New("invalid filename")
	}
	ext := filepath.Ext(baseName)
	name := strings.TrimSuffix(baseName, ext)
	for i := 0; i < 10000; i++ {
		candidateName := baseName
		if i > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", name, i, ext)
		}
		candidatePath := filepath.Join(dir, candidateName)
		file, err := os.OpenFile(candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		if err == nil {
			file.Close()
			return candidatePath, candidateName, nil
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return "", "", err
	}
	return "", "", errors.New("too many duplicate filenames")
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, config.MaxFileSize+1024*1024)
	if err := r.ParseMultipartForm(config.MaxFileSize); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "文件大小超出限制")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取文件失败")
		return
	}
	defer file.Close()

	if !allowedUploadExt(header.Filename) {
		writeError(w, http.StatusBadRequest, "不支持的文件类型")
		return
	}

	reqPath := r.URL.Query().Get("path")
	targetDir, err := getSafePath(config.UploadDir, reqPath)
	if err != nil {
		writeError(w, http.StatusForbidden, "非法路径")
		return
	}

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建上传目录")
		return
	}

	destPath, finalName, err := uniqueFilePath(targetDir, header.Filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存文件")
		return
	}

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存文件")
		return
	}

	limited := &io.LimitedReader{R: file, N: config.MaxFileSize + 1}
	written, err := io.Copy(destFile, limited)
	closeErr := destFile.Close()
	if err != nil {
		os.Remove(destPath)
		writeError(w, http.StatusInternalServerError, "保存文件失败")
		return
	}
	if written > config.MaxFileSize {
		os.Remove(destPath)
		writeError(w, http.StatusRequestEntityTooLarge, "文件大小超出限制")
		return
	}
	if closeErr != nil {
		os.Remove(destPath)
		writeError(w, http.StatusInternalServerError, "保存文件失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "上传成功", "name": finalName})
}

func handleFilesList(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Query().Get("path")
	targetDir, err := getSafePath(config.UploadDir, reqPath)
	if err != nil {
		writeError(w, http.StatusForbidden, "非法路径")
		return
	}

	var files []FileInfo
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			files = []FileInfo{}
		} else {
			writeError(w, http.StatusInternalServerError, "无法读取目录")
			return
		}
	} else {
		for _, entry := range entries {
			info, err := entry.Info()
			if err == nil {
				urlPath := downloadURL(reqPath, info.Name())
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

	writeJSON(w, http.StatusOK, files)
}

func handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path       string `json:"path"`
		FolderName string `json:"folder_name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	folderName := filepath.Base(req.FolderName)
	if folderName == "" || folderName == "." || folderName == "/" || folderName == "\\" {
		writeError(w, http.StatusBadRequest, "无效的文件夹名称")
		return
	}

	targetDir, err := getSafePath(config.UploadDir, filepath.Join(req.Path, folderName))
	if err != nil {
		writeError(w, http.StatusForbidden, "非法路径")
		return
	}

	if err := os.Mkdir(targetDir, os.ModePerm); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeError(w, http.StatusConflict, "文件夹已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "创建文件夹失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "创建文件夹成功"})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if !config.AllowDeletes {
		writeError(w, http.StatusForbidden, "不允许删除文件")
		return
	}

	reqPath := r.URL.Query().Get("path")
	vars := mux.Vars(r)
	filename := filepath.Base(vars["filename"])

	filePath, err := getSafePath(config.UploadDir, filepath.Join(reqPath, filename))
	if err != nil {
		writeError(w, http.StatusForbidden, "非法路径")
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "文件不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "无法读取文件信息")
		return
	}

	if info.IsDir() {
		if err := os.Remove(filePath); err != nil {
			writeError(w, http.StatusConflict, "文件夹不为空")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "删除成功"})
		return
	}

	if err := os.Remove(filePath); err != nil {
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "删除成功"})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
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

	if strings.EqualFold(filepath.Ext(filePath), ".svg") {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(filePath)+"\"")
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	http.ServeFile(w, r, filePath)
}

func main() {
	initConfig()

	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, "static/index.html")
	})
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.HandleFunc("/api/login", handleLogin).Methods("POST")
	r.HandleFunc("/api/logout", handleLogout).Methods("POST")
	r.HandleFunc("/api/session", handleSession).Methods("GET")
	r.HandleFunc("/api/upload", requireAuth(handleUpload)).Methods("POST")
	r.HandleFunc("/api/files", requireAuth(handleFilesList)).Methods("GET")
	r.HandleFunc("/api/create_folder", requireAuth(handleCreateFolder)).Methods("POST")
	r.HandleFunc("/api/delete/{filename}", requireAuth(handleDelete)).Methods("DELETE", "OPTIONS")
	r.PathPrefix("/download/").HandlerFunc(requireAuth(handleDownload)).Methods("GET")

	fmt.Printf("easy-up 服务器启动在 http://localhost:%s\n", config.Port)
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

	log.Fatal(http.ListenAndServe(":"+config.Port, r))
}
