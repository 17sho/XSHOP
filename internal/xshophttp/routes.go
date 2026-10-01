// Package xshophttp is a super-administrator-only proxy, never an installer.
package xshophttp

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/xshopupgrade"
	"github.com/gin-gonic/gin"
	"io"
	"net"
	"net/http"
	"regexp"
	"time"
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Client() *http.Client {
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", xshopupgrade.SocketPath)
	}}}
}

// Register must be called on the existing JWT+RBAC authorized group. The extra
// guard forbids ordinary administrators even if a wildcard role is granted.
func Register(group gin.IRoutes, client *http.Client) {
	for _, action := range []string{"status", "check", "install", "restart"} {
		action := action
		method := http.MethodPost
		if action == "status" {
			method = http.MethodGet
		}
		group.Handle(method, "/xshop-upgrade/"+action, func(c *gin.Context) {
			super, ok := c.Get("admin_is_super")
			id, exists := c.Get("admin_id")
			adminID, idOK := id.(uint)
			if !ok || super != true || !exists || !idOK || adminID == 0 {
				response.ErrorWithHTTPStatus(c, 403, 403, "仅超级管理员可操作 XSHOP 升级")
				return
			}
			c.Header("Cache-Control", "no-store")
			if c.Request.URL.RawQuery != "" {
				response.ErrorWithHTTPStatus(c, 400, 400, "不允许升级参数")
				return
			}
			raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 257))
			if err != nil || len(raw) > 256 {
				response.ErrorWithHTTPStatus(c, 400, 400, "请求过大")
				return
			}
			if action != "install" {
				if len(raw) != 0 {
					response.ErrorWithHTTPStatus(c, 400, 400, "请求必须为空")
					return
				}
			} else {
				var payload struct {
					Digest string `json:"digest"`
				}
				if json.Unmarshal(raw, &payload) != nil || !hashPattern.MatchString(payload.Digest) {
					response.ErrorWithHTTPStatus(c, 400, 400, "更新身份无效")
					return
				}
				expected, _ := json.Marshal(payload)
				if !bytes.Equal(raw, expected) {
					response.ErrorWithHTTPStatus(c, 400, 400, "更新请求格式无效")
					return
				}
			}
			req, err := http.NewRequestWithContext(c.Request.Context(), method, "http://helper/"+action, bytes.NewReader(raw))
			if err != nil {
				response.ErrorWithHTTPStatus(c, 400, 400, "请求无效")
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				c.JSON(503, response.Response{StatusCode: 503, Msg: "升级辅助服务尚未就绪", Data: nil})
				return
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(io.LimitReader(resp.Body, 8193))
			if err != nil || len(data) > 8192 || resp.StatusCode >= 400 {
				c.JSON(409, response.Response{StatusCode: 409, Msg: "升级请求被拒绝；请重新检查或查看状态", Data: nil})
				return
			}
			var payload any
			if json.Unmarshal(data, &payload) != nil {
				c.JSON(503, response.Response{StatusCode: 503, Msg: "升级状态无效", Data: nil})
				return
			}
			if (action == "install" || action == "restart") && resp.StatusCode == 202 {
				c.JSON(202, response.Response{StatusCode: 0, Msg: "success", Data: payload})
				return
			}
			response.Success(c, payload)
		})
	}
}
