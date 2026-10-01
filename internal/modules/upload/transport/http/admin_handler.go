package uploadhttp

import (
	"context"
	"errors"
	"math"
	"mime/multipart"
	"net/http"

	"github.com/dujiao-next/internal/logger"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	uploadcontract "github.com/dujiao-next/internal/modules/upload/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"

	"github.com/gin-gonic/gin"
)

// FileUploader 是文件落盘端口。
type FileUploader interface {
	SaveFileWithMeta(file *multipart.FileHeader, scene string) (*uploadcontract.Result, error)
}

// MediaRecorder 是上传 HTTP 消费方所需的最小 Content 写入接口。
type MediaRecorder interface {
	RecordMedia(ctx context.Context, result contentapp.UploadResult, scene string) (*contentdomain.Media, error)
}

// AdminHandler 处理后台文件上传请求。
type AdminHandler struct {
	uploader FileUploader
	media    MediaRecorder
}

func NewAdminHandler(uploader FileUploader, media MediaRecorder) *AdminHandler {
	if uploader == nil || media == nil {
		panic("upload admin handler: required dependency is nil")
	}
	return &AdminHandler{uploader: uploader, media: media}
}

// UploadFile 文件上传
func (h *AdminHandler) UploadFile(c *gin.Context) {
	maxSize := int64(10 << 20)
	if policy, ok := h.uploader.(interface{ MaxUploadSize() int64 }); ok {
		maxSize = policy.MaxUploadSize()
	}
	if !LimitRequestBody(c, maxSize) {
		return
	}
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	file, err := c.FormFile("file")
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			rejectOversizedUpload(c)
			return
		}
		ginutil.RespondError(c, response.CodeBadRequest, "error.file_missing", nil)
		return
	}
	scene := c.DefaultPostForm("scene", "common")

	result, err := h.uploader.SaveFileWithMeta(file, scene)
	if err != nil {
		if isUploadValidationError(err) {
			ginutil.RespondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
			return
		}
		ginutil.RespondError(c, response.CodeInternal, "error.upload_failed", err)
		return
	}

	var mediaID uint
	media, err := h.media.RecordMedia(c.Request.Context(), contentapp.UploadResult{
		URL:      result.URL,
		Filename: result.Filename,
		MimeType: result.MimeType,
		Size:     result.Size,
		Width:    result.Width,
		Height:   result.Height,
	}, scene)
	if err != nil {
		logger.Warnw("upload_record_media_failed", "error", err, "url", result.URL)
	} else if media != nil {
		mediaID = media.ID
	}

	response.Success(c, gin.H{
		"url":      result.URL,
		"filename": result.Filename,
		"size":     result.Size,
		"media_id": mediaID,
	})
}

// LimitRequestBody must run before any multipart/form parsing. The selected
// file policy is separate; this ceiling also covers unused parts and fields.
func LimitRequestBody(c *gin.Context, maxFileBytes int64) bool {
	const overhead = int64(64 << 10)
	if maxFileBytes < 0 {
		maxFileBytes = 0
	}
	if maxFileBytes > math.MaxInt64-overhead {
		maxFileBytes = math.MaxInt64 - overhead
	}
	limit := maxFileBytes + overhead
	if c.Request.ContentLength > limit {
		rejectOversizedUpload(c)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	return true
}
func rejectOversizedUpload(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"status_code": http.StatusRequestEntityTooLarge, "msg": "upload request too large", "data": nil})
}

func isUploadValidationError(err error) bool {
	var marker interface{ UploadValidationError() }
	return errors.As(err, &marker)
}
