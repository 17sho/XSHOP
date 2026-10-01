package settingshttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	ginutil "github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
)

type RegistrationEmailTemplateAdminService interface {
	GetRegistrationEmailTemplateSetting() (settingsmessaging.RegistrationEmailTemplateSetting, error)
	PatchRegistrationEmailTemplateSetting(settingsmessaging.RegistrationEmailTemplateSettingPatch) (settingsmessaging.RegistrationEmailTemplateSetting, error)
}

type RegistrationEmailTemplateHandler struct {
	templates RegistrationEmailTemplateAdminService
}

func NewRegistrationEmailTemplateHandler(templates RegistrationEmailTemplateAdminService) *RegistrationEmailTemplateHandler {
	if templates == nil {
		panic("settings registration-email-template handler: templates is nil")
	}
	return &RegistrationEmailTemplateHandler{templates: templates}
}

func (h *RegistrationEmailTemplateHandler) GetRegistrationEmailTemplate(c *gin.Context) {
	setting, err := h.templates.GetRegistrationEmailTemplateSetting()
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	response.Success(c, settingsmessaging.EncodeRegistrationEmailTemplateSetting(setting))
}

func (h *RegistrationEmailTemplateHandler) GetRegistrationEmailTemplateDefaults(c *gin.Context) {
	response.Success(c, settingsmessaging.EncodeRegistrationEmailTemplateSetting(settingsmessaging.DefaultRegistrationEmailTemplateSetting()))
}

func (h *RegistrationEmailTemplateHandler) UpdateRegistrationEmailTemplate(c *gin.Context) {
	// Three 200KB HTML templates plus plain text/JSON overhead. Bound the entire
	// input and require a single JSON object, not a valid prefix of a bad request.
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024))
	var patch settingsmessaging.RegistrationEmailTemplateSettingPatch
	if err == nil {
		err = json.Unmarshal(data, &patch)
	}
	if err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	setting, err := h.templates.PatchRegistrationEmailTemplateSetting(patch)
	if err != nil {
		if errors.Is(err, settingsmessaging.ErrRegistrationEmailTemplateConfigInvalid) {
			ginutil.RespondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
		} else {
			ginutil.RespondError(c, response.CodeInternal, "error.settings_save_failed", err)
		}
		return
	}
	response.Success(c, settingsmessaging.EncodeRegistrationEmailTemplateSetting(setting))
}
