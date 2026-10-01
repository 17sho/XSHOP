package userauthhttp

import (
	"errors"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	userpresenter "github.com/dujiao-next/internal/modules/identity/userauth/transport/presenter"
	settingsstorefront "github.com/dujiao-next/internal/modules/settings/schema/storefront"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/shared/jsonmap"

	"github.com/gin-gonic/gin"
)

var (
	ErrProfileEmpty = errors.New("user profile empty")
	ErrUserNotFound = errors.New("user not found")
)

// UserProfileService 是用户资料端点所需的最小端口。
type UserProfileService interface {
	GetUserByID(id uint) (*userdomain.User, error)
	ResolveEmailChangeMode(user *userdomain.User) (string, error)
	ResolvePasswordChangeMode(user *userdomain.User) (string, error)
	UpdateProfile(userID uint, nickname, locale *string) (*userdomain.User, error)
	GetPersonalCenterVisibility() (jsonmap.JSON, error)
}

// UserProfileHandler 处理当前用户资料 HTTP 请求。
type UserProfileHandler struct {
	service UserProfileService
}

func NewUserProfileHandler(service UserProfileService) *UserProfileHandler {
	if service == nil {
		panic("user profile handler: service is nil")
	}
	return &UserProfileHandler{service: service}
}

// UserProfileUpdateRequest 更新资料请求。
type UserProfileUpdateRequest struct {
	Nickname *string `json:"nickname"`
	Locale   *string `json:"locale"`
}

// GetCurrentUser 获取当前用户信息。
func (h *UserProfileHandler) GetCurrentUser(c *gin.Context) {
	id, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}

	user, err := h.service.GetUserByID(id)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	if user == nil {
		ginutil.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		return
	}

	profile, err := h.userProfileResponse(user)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, profile)
}

func (h *UserProfileHandler) userProfileResponse(user *userdomain.User) (userpresenter.UserProfileResp, error) {
	emailMode, err := h.service.ResolveEmailChangeMode(user)
	if err != nil {
		return userpresenter.UserProfileResp{}, err
	}
	passwordMode, err := h.service.ResolvePasswordChangeMode(user)
	if err != nil {
		return userpresenter.UserProfileResp{}, err
	}
	globalVisibility, err := h.service.GetPersonalCenterVisibility()
	if err != nil {
		return userpresenter.UserProfileResp{}, err
	}
	effectiveVisibility := settingsstorefront.ResolvePersonalCenterVisibility(globalVisibility, user.PersonalCenterVisibility)
	return userpresenter.NewUserProfileResp(user, emailMode, passwordMode, effectiveVisibility), nil
}

// UpdateUserProfile 更新用户资料。
func (h *UserProfileHandler) UpdateUserProfile(c *gin.Context) {
	id, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}

	var req UserProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}

	user, err := h.service.UpdateProfile(id, req.Nickname, req.Locale)
	if err != nil {
		switch {
		case errors.Is(err, ErrProfileEmpty):
			ginutil.RespondError(c, response.CodeBadRequest, "error.profile_empty", nil)
		case errors.Is(err, ErrUserNotFound):
			ginutil.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			ginutil.RespondError(c, response.CodeInternal, "error.user_update_failed", err)
		}
		return
	}

	profile, err := h.userProfileResponse(user)
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.user_update_failed", err)
		return
	}
	response.Success(c, profile)
}
