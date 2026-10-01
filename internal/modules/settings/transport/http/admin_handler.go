package settingshttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/constants"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	ginutil "github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/shared/jsonmap"

	"github.com/gin-gonic/gin"
)

// AdminService 是后台通用设置端口。
type AdminService interface {
	GetByKey(key string) (jsonmap.JSON, error)
	UpdateWithEffects(key string, value map[string]interface{}) (settingsapp.UpdateResult, error)
	InvalidateCallbackRoutesCache()
}

// AdminHandler 处理后台通用设置请求。
type AdminHandler struct {
	settings   AdminService
	authorizer SettingAuthorizer
	typed      map[string]TypedSettingRoute
}

// SettingAuthorizer applies the same route/method permissions as typed APIs.
type SettingAuthorizer interface {
	EnforceAdmin(uint, string, string) (bool, error)
}

// TypedSettingRoute binds a protected setting to its existing HTTP contract.
type TypedSettingRoute struct {
	Resource string
	Get      gin.HandlerFunc
	Update   gin.HandlerFunc
}

// ConfigureTypedRoutes is called once during server composition, before serving.
func (h *AdminHandler) ConfigureTypedRoutes(authorizer SettingAuthorizer, routes map[string]TypedSettingRoute) {
	h.authorizer = authorizer
	h.typed = make(map[string]TypedSettingRoute, len(routes)+1)
	for key, route := range routes {
		h.typed[key] = route
	}
	h.typed[constants.SettingKeySecurityCenterConfig] = TypedSettingRoute{Resource: "/admin/settings", Get: h.getSecurityCenterConfig, Update: h.updateSecurityCenterConfig}
}

func NewAdminHandler(settings AdminService) *AdminHandler {
	if settings == nil {
		panic("settings admin handler: settings is nil")
	}
	return &AdminHandler{settings: settings}
}

type updateRequest struct {
	Key   string          `json:"key" binding:"required"`
	Value json.RawMessage `json:"value" binding:"required"`
}

// decodeUpdateRequest retains raw nested JSON and rejects ambiguous envelopes.
func decodeUpdateRequest(data []byte, req *updateRequest) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return settingssecurity.ErrSecurityCenterConfigInvalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		// Match encoding/json's Unicode simple case folding for struct fields,
		// including the Kelvin-sign alias of K. Keep singleton aliases compatible.
		if strings.EqualFold(key, "key") {
			key = "key"
		} else if strings.EqualFold(key, "value") {
			key = "value"
		}
		if !ok || seen[key] {
			return settingssecurity.ErrSecurityCenterConfigInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return err
	}
	if err = decoder.Decode(new(interface{})); err != io.EOF {
		return settingssecurity.ErrSecurityCenterConfigInvalid
	}
	return json.Unmarshal(data, req)
}

// genericSettingKey is an explicit public-admin API allowlist, not the internal
// persistence registry: protected settings must use their typed handlers.
func genericSettingKey(key string) bool {
	switch key {
	case constants.SettingKeySiteConfig, constants.SettingKeyOrderConfig,
		constants.SettingKeyDashboardConfig, constants.SettingKeyPersonalCenterVisibilityConfig,
		constants.SettingKeyNavConfig, constants.SettingKeyWalletConfig, constants.SettingKeyPaymentConfig,
		constants.SettingKeyRegistrationConfig, constants.SettingKeyOrderRiskControlConfig,
		constants.SettingKeyUpstreamSyncConfig, constants.SettingKeyCallbackRoutesConfig,
		constants.SettingKeyHomeAnnouncement, constants.SettingKeyHomepageAd:
		return true
	default:
		return false
	}
}

// dispatchTyped reuses typed validation, masking and runtime/cache effects. No
// protected or unknown key can fall back to raw persistence, even if unwired.
func (h *AdminHandler) dispatchTyped(c *gin.Context, key string, value json.RawMessage) {
	route, ok := h.typed[key]
	handler := route.Get
	if c.Request.Method == http.MethodPut {
		handler = route.Update
	}
	if !ok || route.Resource == "" || handler == nil || h.authorizer == nil {
		ginutil.RespondError(c, response.CodeForbidden, "error.forbidden", nil)
		return
	}
	if !ginutil.IsSuperAdmin(c) {
		adminID, valid := ginutil.GetAdminID(c)
		if !valid {
			return
		}
		if adminID == 0 {
			ginutil.RespondError(c, response.CodeForbidden, "error.forbidden", nil)
			return
		}
		allowed, err := h.authorizer.EnforceAdmin(adminID, route.Resource, c.Request.Method)
		if err != nil || !allowed {
			ginutil.RespondError(c, response.CodeForbidden, "error.forbidden", nil)
			return
		}
	}
	if c.Request.Method == http.MethodPut {
		// Preserve the original JSON: re-encoding a map would erase duplicate
		// fields and weaken strict typed validators (notably mail templates).
		body := value
		request := c.Request.Clone(c.Request.Context())
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		request.Header.Set("Content-Type", "application/json")
		original := c.Request
		c.Request = request
		defer func() { c.Request = original }()
	}
	handler(c)
}

// Get 获取设置。
func (h *AdminHandler) Get(c *gin.Context) {
	key := strings.TrimSpace(c.DefaultQuery("key", constants.SettingKeySiteConfig))
	if !genericSettingKey(key) {
		h.dispatchTyped(c, key, nil)
		return
	}

	value, err := h.settings.GetByKey(key)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	if value == nil {
		response.Success(c, gin.H{})
		return
	}

	response.Success(c, value)
}

// Security center settings deliberately reuse /admin/settings GET/PUT permissions.
func (h *AdminHandler) getSecurityCenterConfig(c *gin.Context) {
	raw, err := h.settings.GetByKey(constants.SettingKeySecurityCenterConfig)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	response.Success(c, settingssecurity.NormalizeSecurityCenterConfigJSON(raw))
}

func (h *AdminHandler) updateSecurityCenterConfig(c *gin.Context) {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	var value jsonmap.JSON
	if err == nil {
		value, err = settingssecurity.DecodeSecurityCenterConfig(data)
	}
	if err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	result, err := h.settings.UpdateWithEffects(constants.SettingKeySecurityCenterConfig, value)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.settings_save_failed", err)
		return
	}
	if result.HasEffect(settingsapp.EffectInvalidatePublicConfigCache) {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	response.Success(c, result.Value)
}

// Update 更新设置。
func (h *AdminHandler) Update(c *gin.Context) {
	var req updateRequest
	// Allow all localized order templates while bounding envelope parsing.
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024*1024))
	if err == nil {
		err = decodeUpdateRequest(data, &req)
	}
	if err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	key := strings.TrimSpace(req.Key)
	if key == "" || len(req.Value) == 0 || req.Value[0] != '{' ||
		key == constants.SettingKeyRegistrationEmailTemplateConfig && len(data) > 1024*1024 {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	if key == constants.SettingKeyGoogleAuthConfig || key == constants.SettingKeyGitHubAuthConfig {
		endpoint := "/admin/settings/google-auth"
		if key == constants.SettingKeyGitHubAuthConfig {
			endpoint = "/admin/settings/github-auth"
		}
		ginutil.RespondErrorWithMsg(
			c,
			response.CodeBadRequest,
			key+" must be updated through "+endpoint,
			nil,
		)
		return
	}

	if !genericSettingKey(key) {
		h.dispatchTyped(c, key, req.Value)
		return
	}
	var value map[string]interface{}
	if err := json.Unmarshal(req.Value, &value); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	result, err := h.settings.UpdateWithEffects(key, value)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.settings_save_failed", err)
		return
	}

	if result.HasEffect(settingsapp.EffectInvalidatePublicConfigCache) {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	if result.HasEffect(settingsapp.EffectInvalidateCallbackRoutesCache) {
		h.settings.InvalidateCallbackRoutesCache()
	}
	response.Success(c, result.Value)
}
