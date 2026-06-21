package response

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PageResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	Size    int         `json:"size"`
}

// sanitizeErrorMessage 脱敏错误信息
func sanitizeErrorMessage(message string) string {
	// 移除内部 IP 地址
	message = removeInternalIPs(message)
	// 移除文件路径
	message = removeFilePaths(message)
	return message
}

// removeInternalIPs 移除内部IP地址
func removeInternalIPs(message string) string {
	// 简单的 IP 脱敏
	parts := message
	for _, pattern := range []string{"10.", "172.16.", "172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168."} {
		for {
			idx := strings.Index(parts, pattern)
			if idx == -1 {
				break
			}
			// 找到IP地址的结束位置
			end := idx + len(pattern)
			for end < len(parts) && (parts[end] == '.' || (parts[end] >= '0' && parts[end] <= '9')) {
				end++
			}
			// 替换为 *
			parts = parts[:idx] + "***" + parts[end:]
		}
	}
	return parts
}

// removeFilePaths 移除文件路径
func removeFilePaths(message string) string {
	// 移除常见的文件路径模式
	paths := []string{"/root/", "/home/", "/opt/", "/var/", "/etc/"}
	result := message
	for _, path := range paths {
		for {
			idx := strings.Index(result, path)
			if idx == -1 {
				break
			}
			// 找到路径的结束位置（空格或换行）
			end := idx + len(path)
			for end < len(result) && result[end] != ' ' && result[end] != '\n' && result[end] != '"' {
				end++
			}
			result = result[:idx] + "[path]" + result[end:]
		}
	}
	return result
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

func NoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, nil)
}

func Error(c *gin.Context, code int, message string) {
	c.JSON(code, Response{
		Code:    code,
		Message: sanitizeErrorMessage(message),
	})
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, message)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, message)
}

func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, message)
}

func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, message)
}

func PageSuccess(c *gin.Context, data interface{}, total int64, page, size int) {
	c.JSON(http.StatusOK, PageResponse{
		Code:    0,
		Message: "success",
		Data:    data,
		Total:   total,
		Page:    page,
		Size:    size,
	})
}
