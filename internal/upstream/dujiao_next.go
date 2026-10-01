package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"

	"github.com/dujiao-next/internal/logger"

	"github.com/google/uuid"
)

// upstreamHTTPError 上游返回非 200 时的结构化错误
type upstreamHTTPError struct {
	Status  int
	Code    string
	Message string
	Body    string
}

func (e *upstreamHTTPError) Error() string {
	// Remote error bodies/messages may echo delivery data or credentials.
	// Keep structured codes for internal classification, never public/log formatting.
	return fmt.Sprintf("upstream responded with status %d", e.Status)
}

// extractUpstreamErrorCode 从错误链中提取 upstreamHTTPError.Code
func extractUpstreamErrorCode(err error) string {
	var ue *upstreamHTTPError
	if errors.As(err, &ue) {
		return ue.Code
	}
	return ""
}

// safeRequestError accepts only static operation labels. URL and transport errors
// can contain credentials, including in their causes; never retain that chain.
// Re-wrap only known context sentinels so cancellation remains classifiable.
func safeRequestError(operation string, err error) error {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			return fmt.Errorf("%s: %w", operation, cause)
		}
	}
	return fmt.Errorf("%s failed", operation)
}

// DujiaoNextAdapter Dujiao-Next 协议适配器
type DujiaoNextAdapter struct {
	baseURL     string
	apiKey      string
	apiSecret   string
	uploadsDir  string
	client      *http.Client
	imageClient *http.Client
}

// NewDujiaoNextAdapter 创建 Dujiao-Next 适配器
func NewDujiaoNextAdapter(conn *siteconnectiondomain.Connection, uploadsDir string) *DujiaoNextAdapter {
	return &DujiaoNextAdapter{
		baseURL:     strings.TrimRight(conn.BaseURL, "/"),
		apiKey:      conn.ApiKey,
		apiSecret:   conn.ApiSecret,
		uploadsDir:  uploadsDir,
		imageClient: newImageHTTPClient(),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Ping 连接测试
func (a *DujiaoNextAdapter) Ping(ctx context.Context) (*PingResult, error) {
	var result struct {
		OK bool `json:"ok"`
		PingResult
	}
	if err := a.doRequest(ctx, http.MethodPost, "/api/v1/upstream/ping", nil, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("ping failed")
	}
	return &result.PingResult, nil
}

// ListCategories 拉取上游分类列表
func (a *DujiaoNextAdapter) ListCategories(ctx context.Context) (*CategoryListResult, error) {
	var result struct {
		OK         bool               `json:"ok"`
		Categories []UpstreamCategory `json:"categories"`
	}
	if err := a.doRequest(ctx, http.MethodGet, "/api/v1/upstream/categories", nil, &result); err != nil {
		// 旧版上游不支持分类 API，返回空列表
		var ue *upstreamHTTPError
		if errors.As(err, &ue) && ue.Status == http.StatusNotFound {
			return &CategoryListResult{Supported: false, Categories: []UpstreamCategory{}}, nil
		}
		return nil, err
	}
	return &CategoryListResult{Supported: true, Categories: result.Categories}, nil
}

// ListProducts 拉取上游商品列表
func (a *DujiaoNextAdapter) ListProducts(ctx context.Context, opts ListProductsOpts) (*ProductListResult, error) {
	path := fmt.Sprintf("/api/v1/upstream/products?page=%d&page_size=%d", opts.Page, opts.PageSize)
	if opts.UpdatedAfter != nil {
		path += "&updated_after=" + opts.UpdatedAfter.Format(time.RFC3339)
	}
	if opts.IncludeInactive {
		path += "&include_inactive=true"
	}
	var result ProductListResult
	if err := a.doRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetProduct 获取单个商品详情
// 上游已删除（软删）→ 返回 ErrUpstreamProductDeleted
// 旧版上游对下架商品也返回 404 product_unavailable → 返回 ErrUpstreamProductUnavailable
// 新版上游下架商品改为 200 + is_active=false，调用方应根据 IsActive 字段判断
func (a *DujiaoNextAdapter) GetProduct(ctx context.Context, productID uint) (*UpstreamProduct, error) {
	path := fmt.Sprintf("/api/v1/upstream/products/%d", productID)
	var result struct {
		OK      bool            `json:"ok"`
		Product UpstreamProduct `json:"product"`
	}
	if err := a.doRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		// 解析上游返回的 error_code 归一化为哨兵错误
		switch extractUpstreamErrorCode(err) {
		case "product_deleted", "product_not_found":
			return nil, ErrUpstreamProductDeleted
		case "product_unavailable":
			return nil, ErrUpstreamProductUnavailable
		}
		return nil, err
	}
	return &result.Product, nil
}

// CreateOrder 发起采购单
func (a *DujiaoNextAdapter) CreateOrder(ctx context.Context, req CreateUpstreamOrderReq) (*CreateUpstreamOrderResp, error) {
	var result CreateUpstreamOrderResp
	if err := a.doRequest(ctx, http.MethodPost, "/api/v1/upstream/orders", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOrder 查询上游订单状态
func (a *DujiaoNextAdapter) GetOrder(ctx context.Context, orderID uint) (*UpstreamOrderDetail, error) {
	path := fmt.Sprintf("/api/v1/upstream/orders/%d", orderID)
	var result UpstreamOrderDetail
	if err := a.doRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelOrder 取消采购单
func (a *DujiaoNextAdapter) CancelOrder(ctx context.Context, orderID uint) error {
	path := fmt.Sprintf("/api/v1/upstream/orders/%d/cancel", orderID)
	var result struct {
		OK bool `json:"ok"`
	}
	if err := a.doRequest(ctx, http.MethodPost, path, nil, &result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("cancel order failed")
	}
	return nil
}

// DownloadImage 下载图片到本地
func (a *DujiaoNextAdapter) DownloadImage(ctx context.Context, imageURL string) (string, error) {
	// 相对路径转绝对 URL
	fullURL := imageURL
	if strings.HasPrefix(imageURL, "/") {
		fullURL = a.baseURL + imageURL
	}

	u, err := url.Parse(fullURL)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("invalid image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", safeRequestError("create download request", err)
	}

	client := a.imageClient
	if client == nil {
		client = a.client
	} // explicit test adapter injection only; constructors always supply guarded media transport
	resp, err := client.Do(req)
	if err != nil {
		return "", safeRequestError("download image", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image: status %d", resp.StatusCode)
	}

	const maxImageBytes = 10 * 1024 * 1024
	if resp.ContentLength > maxImageBytes {
		return "", errors.New("image exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return "", safeRequestError("read image", err)
	}
	if len(data) > maxImageBytes {
		return "", errors.New("image exceeds size limit")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return "", errors.New("invalid image")
	}
	ext := map[string]string{"png": ".png", "jpeg": ".jpg", "gif": ".gif", "webp": ".webp"}[format]
	if ext == "" {
		return "", errors.New("unsupported image")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", errors.New("invalid image")
	}

	filename := uuid.New().String() + ext
	dir := filepath.Join(a.uploadsDir, "upstream")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create uploads dir: %w", err)
	}

	filePath := filepath.Join(dir, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	committed := false
	defer func() {
		f.Close()
		if !committed {
			os.Remove(filePath)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return "", errors.New("write image failed")
	}
	if err := f.Close(); err != nil {
		return "", errors.New("close image failed")
	}
	committed = true

	// 返回相对路径
	return "/uploads/upstream/" + filename, nil
}

// Media requests never use environment proxies; DNS answers are checked once
// and the vetted IP itself is dialed (no second-resolution/rebinding window).
func newImageHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !publicImageIP(ip.IP) {
					return nil, errors.New("image target forbidden")
				}
			}
			for _, ip := range ips {
				conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if e == nil {
					return conn, nil
				}
			}
			return nil, errors.New("image dial failed")
		},
	}}
}
func publicImageIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/23", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "3fff::/20", "5f00::/16"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// doRequest 发送签名请求
func (a *DujiaoNextAdapter) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	// 签名用的 path 不含 query string
	signPath := path
	if idx := strings.Index(path, "?"); idx > 0 {
		signPath = path[:idx]
	}

	timestamp := time.Now().Unix()
	signature := Sign(a.apiSecret, method, signPath, timestamp, bodyBytes)

	url := a.baseURL + path
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return safeRequestError("create request", err)
	}

	req.Header.Set(HeaderApiKey, a.apiKey)
	req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(HeaderSignature, signature)
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return safeRequestError("send request", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Warnw("upstream_request_error",
			"method", method, "path", path,
			"status", resp.StatusCode, "response_bytes", len(respBody))
		// 尝试解析结构化错误响应
		var errPayload struct {
			ErrorCode    string `json:"error_code"`
			ErrorMessage string `json:"error_message"`
		}
		_ = json.Unmarshal(respBody, &errPayload)
		return &upstreamHTTPError{
			Status:  resp.StatusCode,
			Code:    errPayload.ErrorCode,
			Message: errPayload.ErrorMessage,
			Body:    string(respBody),
		}
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}
