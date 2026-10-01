package adminuserhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"bytes"
	externalidentitydomain "github.com/dujiao-next/internal/modules/identity/externalidentity/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	settingsstorefront "github.com/dujiao-next/internal/modules/settings/schema/storefront"
	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/gin-gonic/gin"
	"reflect"
)

type oauthIdentityUnbinderStub struct {
	googleErr    error
	googleUserID uint
}

func (s *oauthIdentityUnbinderStub) UnbindTelegram(uint) error {
	return nil
}

func (s *oauthIdentityUnbinderStub) UnbindGoogle(userID uint) error {
	s.googleUserID = userID
	return s.googleErr
}

func TestUnbindAdminUserGoogle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		id          string
		serviceErr  error
		wantCode    int
		wantMessage string
		wantUserID  uint
	}{
		{name: "success", id: "42", wantCode: response.CodeOK, wantMessage: "success", wantUserID: 42},
		{name: "invalid user id", id: "bad", wantCode: response.CodeBadRequest, wantMessage: "Invalid user ID"},
		{name: "user not found", id: "42", serviceErr: ErrNotFound, wantCode: response.CodeNotFound, wantMessage: "User not found", wantUserID: 42},
		{name: "user disabled", id: "42", serviceErr: ErrUserDisabled, wantCode: response.CodeBadRequest, wantMessage: "Account disabled", wantUserID: 42},
		{name: "google not bound", id: "42", serviceErr: ErrUserOAuthNotBound, wantCode: response.CodeBadRequest, wantMessage: "Current account is not bound to Google", wantUserID: 42},
		{name: "would lock account", id: "42", serviceErr: ErrGoogleUnbindLocked, wantCode: response.CodeBadRequest, wantMessage: "Set a local password or bind another usable login method before unbinding Google", wantUserID: 42},
		{name: "unexpected failure", id: "42", serviceErr: errors.New("database unavailable"), wantCode: response.CodeInternal, wantMessage: "Failed to update user profile", wantUserID: 42},
	}

	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			unbinder := &oauthIdentityUnbinderStub{googleErr: item.serviceErr}
			handler := &AdminHandler{oauthUnbinder: unbinder}
			router := gin.New()
			RegisterAdminRoutes(router.Group("/admin"), handler)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodDelete, "/admin/users/"+item.id+"/oauth/google", nil)
			request.Header.Set("Accept-Language", "en-US")

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusOK)
			}
			var body response.Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.StatusCode != item.wantCode {
				t.Fatalf("status_code = %d, want %d; body=%s", body.StatusCode, item.wantCode, recorder.Body.String())
			}
			if body.Msg != item.wantMessage {
				t.Fatalf("msg = %q, want %q", body.Msg, item.wantMessage)
			}
			if unbinder.googleUserID != item.wantUserID {
				t.Fatalf("UnbindGoogle userID = %d, want %d", unbinder.googleUserID, item.wantUserID)
			}
		})
	}
}

type adminVisibilityUsersStub struct{ user *userdomain.User }

func (s *adminVisibilityUsersStub) List(UserListFilter) ([]userdomain.User, int64, error) {
	return nil, 0, nil
}
func (s *adminVisibilityUsersStub) GetByID(uint) (*userdomain.User, error)      { return s.user, nil }
func (s *adminVisibilityUsersStub) GetByEmail(string) (*userdomain.User, error) { return nil, nil }
func (s *adminVisibilityUsersStub) UpdateFields(user *userdomain.User, fields ...string) error {
	s.user = user
	return nil
}
func (s *adminVisibilityUsersStub) BatchUpdateStatus([]uint, string) error { return nil }

type adminVisibilityWalletStub struct{}

func (adminVisibilityWalletStub) GetBalancesByUserIDs([]uint) (map[uint]money.Amount, error) {
	return nil, nil
}
func (adminVisibilityWalletStub) GetAccount(uint) (*walletdomain.Account, error) {
	return &walletdomain.Account{}, nil
}

type adminVisibilityOAuthStub struct{}

func (adminVisibilityOAuthStub) ListByUserID(uint) ([]externalidentitydomain.Identity, error) {
	return nil, nil
}

type adminVisibilitySettingsStub struct{ global jsonmap.JSON }

func (s adminVisibilitySettingsStub) GetPersonalCenterVisibility() (jsonmap.JSON, error) {
	return s.global, nil
}

func TestAdminUserDetailIncludesOverridesAndEffectiveVisibility(t *testing.T) {
	users := &adminVisibilityUsersStub{user: &userdomain.User{ID: 7, PersonalCenterVisibility: jsonmap.JSON{"orders": true}}}
	h := &AdminHandler{users: users, wallets: adminVisibilityWalletStub{}, oauth: adminVisibilityOAuthStub{}, visibility: adminVisibilitySettingsStub{global: jsonmap.JSON{"orders": false, "wallet": false}}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:id", h.GetAdminUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/7", nil))
	var body struct {
		Data AdminUserDetail `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	wantEffective := settingsstorefront.ResolvePersonalCenterVisibility(jsonmap.JSON{"orders": false, "wallet": false}, users.user.PersonalCenterVisibility)
	if !reflect.DeepEqual(body.Data.PersonalCenterVisibilityOverrides, users.user.PersonalCenterVisibility) || !reflect.DeepEqual(body.Data.PersonalCenterVisibilityEffective, wantEffective) {
		t.Fatalf("detail visibility = %#v / %#v", body.Data.PersonalCenterVisibilityOverrides, body.Data.PersonalCenterVisibilityEffective)
	}
}

func TestUpdateAdminUserStoresSparseVisibilityOverridesAndRejectsUnknownKeys(t *testing.T) {
	for _, tc := range []struct {
		name     string
		payload  string
		wantCode int
	}{
		{"valid sparse", `{"personal_center_visibility_overrides":{"orders":false}}`, 0},
		{"unknown key", `{"personal_center_visibility_overrides":{"unknown":false}}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := &adminVisibilityUsersStub{user: &userdomain.User{ID: 7}}
			h := &AdminHandler{users: users}
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.PUT("/users/:id", h.UpdateAdminUser)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/users/7", bytes.NewBufferString(tc.payload)))
			var body struct {
				StatusCode int `json:"status_code"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if tc.wantCode == 0 {
				if !reflect.DeepEqual(users.user.PersonalCenterVisibility, jsonmap.JSON{"orders": false}) {
					t.Fatalf("stored = %#v", users.user.PersonalCenterVisibility)
				}
			} else if body.StatusCode != tc.wantCode {
				t.Fatalf("status_code = %d, want %d; body=%s", body.StatusCode, tc.wantCode, w.Body.String())
			}
		})
	}
}
