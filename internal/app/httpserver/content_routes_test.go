package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/authz"
	contenttransport "github.com/dujiao-next/internal/modules/content/transport/http"
	admincontract "github.com/dujiao-next/internal/modules/identity/admin/contract"
	admindomain "github.com/dujiao-next/internal/modules/identity/admin/domain"
	adminstore "github.com/dujiao-next/internal/modules/identity/admin/infrastructure/gormstore"
	adminauthapp "github.com/dujiao-next/internal/modules/identity/adminauth/application"
	"github.com/dujiao-next/internal/modules/identity/jwttoken"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func TestUploadRoutesBoundUnusedMultipartAndKeepOrdinaryBodies(t *testing.T) {
	for _, route := range []string{"/api/v1/admin/upload", "/api/v1/reseller/upload", "/api/v1/other"} {
		for _, unknown := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chunked=%v", route, unknown), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, _ := writer.CreateFormFile("file", "tiny.png")
				_, _ = part.Write([]byte("fixture"))
				unused, _ := writer.CreateFormFile("unused", "unused.bin")
				_, _ = unused.Write(bytes.Repeat([]byte("x"), 128<<10))
				_ = writer.Close()
				router := gin.New()
				router.Use(uploadRequestBodyLimit(1024))
				saved := false
				router.POST(route, func(c *gin.Context) {
					if _, err := c.FormFile("file"); err != nil {
						c.Status(400)
						return
					}
					saved = true
					c.Status(204)
				})
				req := httptest.NewRequest(http.MethodPost, route, bytes.NewReader(body.Bytes()))
				req.Header.Set("Content-Type", writer.FormDataContentType())
				if unknown {
					req.ContentLength = -1
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if route == "/api/v1/other" {
					if !saved || rec.Code != 204 {
						t.Fatalf("unrelated route changed: %d", rec.Code)
					}
				} else if saved || rec.Code < 400 {
					t.Fatalf("upload whole-body cap bypassed: saved=%v status=%d", saved, rec.Code)
				}
			})
		}
	}
}

func TestUploadBodyGuardPreservesValidAdminAndResellerMultipart(t *testing.T) {
	for _, route := range []string{"/api/v1/admin/upload", "/api/v1/reseller/upload"} {
		for _, unknown := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chunked=%v", route, unknown), func(t *testing.T) {
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, err := writer.CreateFormFile("file", "tiny.png")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = part.Write([]byte("fixture")); err != nil {
					t.Fatal(err)
				}
				if err = writer.WriteField("scene", "branding"); err != nil {
					t.Fatal(err)
				}
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
				router := gin.New()
				router.Use(uploadRequestBodyLimit(1024))
				saved := false
				router.POST(route, func(c *gin.Context) {
					file, err := c.FormFile("file")
					if err != nil {
						t.Error(err)
						c.Status(400)
						return
					}
					if file.Filename != "tiny.png" || file.Size != 7 || c.PostForm("scene") != "branding" {
						t.Error("valid multipart fields changed")
					}
					saved = true
					c.Status(204)
				})
				req := httptest.NewRequest(http.MethodPost, route, bytes.NewReader(body.Bytes()))
				req.Header.Set("Content-Type", writer.FormDataContentType())
				if unknown {
					req.ContentLength = -1
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if !saved || rec.Code != 204 {
					t.Fatalf("valid upload denied: saved=%v status=%d", saved, rec.Code)
				}
			})
		}
	}
}

const contentAdminJWTSecret = "content-admin-route-contract-secret"

var contentAdminRouteContract = []adminRoute{
	{method: http.MethodGet, object: "/admin/posts"},
	{method: http.MethodPost, object: "/admin/posts"},
	{method: http.MethodPut, object: "/admin/posts/:id"},
	{method: http.MethodDelete, object: "/admin/posts/:id"},

	{method: http.MethodGet, object: "/admin/banners"},
	{method: http.MethodGet, object: "/admin/banners/:id"},
	{method: http.MethodPost, object: "/admin/banners"},
	{method: http.MethodPut, object: "/admin/banners/:id"},
	{method: http.MethodDelete, object: "/admin/banners/:id"},
	{method: http.MethodGet, object: "/admin/media"},
	{method: http.MethodPost, object: "/admin/media/batch-delete"},
	{method: http.MethodPut, object: "/admin/media/:id"},
	{method: http.MethodDelete, object: "/admin/media/:id"},
}

var contentPublicRouteContract = []adminRoute{
	{method: http.MethodGet, object: "/api/v1/public/posts"},
	{method: http.MethodGet, object: "/api/v1/public/posts/:slug"},
	{method: http.MethodGet, object: "/api/v1/public/banners"},
}

func TestContentRouteContractMatchesRouterSource(t *testing.T) {
	adminRoutes, err := extractAdminRoutesFromSource()
	if err != nil {
		t.Fatalf("extract admin routes: %v", err)
	}
	actualAdmin := make([]adminRoute, 0, len(contentAdminRouteContract))
	for _, route := range adminRoutes {
		if isContentAdminRoute(route.object) {
			actualAdmin = append(actualAdmin, route)
		}
	}
	assertRouteSetsEqual(t, contentAdminRouteContract, actualAdmin)

	publicRoutes, err := extractPublicContentRoutesFromSource()
	if err != nil {
		t.Fatalf("extract public content routes: %v", err)
	}
	assertRouteSetsEqual(t, contentPublicRouteContract, publicRoutes)
}

func TestContentRoutesStayInExpectedRouterTrustGroups(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve content route test filename")
	}
	routerDirectory := filepath.Dir(thisFile)
	registrations := []struct {
		file      string
		statement string
	}{
		{file: "routes_storefront.go", statement: "contenttransport.RegisterPublicRoutes(public, publicContentHandler)"},
		{file: "routes_admin.go", statement: "contenttransport.RegisterAdminRoutes(authorized, adminContentHandler)"},
	}

	for _, registration := range registrations {
		raw, err := os.ReadFile(filepath.Join(routerDirectory, registration.file))
		if err != nil {
			t.Fatalf("read production route file %s: %v", registration.file, err)
		}
		source := string(raw)
		if !strings.Contains(source, registration.statement) {
			t.Errorf("%s must contain %q; changing the route group can bypass the intended authentication or public boundary", registration.file, registration.statement)
		}
	}
}

func TestAdminContentRoutesRequireAuthenticationAndOperationsPermission(t *testing.T) {
	adminRepo, authzService, noPermissionToken, operationsToken := setupContentRouteAccessTest(t)

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: response.CodeUnauthorized},
		{name: "invalid token", token: "not-a-jwt", wantStatus: response.CodeUnauthorized},
		{name: "no permission", token: noPermissionToken, wantStatus: response.CodeForbidden},
		{name: "operations", token: operationsToken, wantStatus: response.CodeOK},
	}

	for _, route := range contentAdminRouteContract {
		route := route
		t.Run(route.method+" "+route.object, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					router := newContentRouteAccessRouter(adminRepo, authzService)
					path := strings.ReplaceAll("/api/v1"+route.object, ":id", "42")
					request := httptest.NewRequest(route.method, path, nil)
					if test.token != "" {
						request.Header.Set("Authorization", "Bearer "+test.token)
					}
					recorder := httptest.NewRecorder()
					router.ServeHTTP(recorder, request)

					var got struct {
						StatusCode int `json:"status_code"`
					}
					if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
						t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
					}
					if got.StatusCode != test.wantStatus {
						t.Fatalf("business status want %d got %d body=%s", test.wantStatus, got.StatusCode, recorder.Body.String())
					}
				})
			}
		})
	}
}

func extractPublicContentRoutesFromSource() ([]adminRoute, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("resolve content route test filename")
	}
	routerSource := filepath.Join(filepath.Dir(thisFile), "..", "..", "modules", "content", "transport", "http", "routes.go")
	raw, err := os.ReadFile(routerSource)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`public\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	routes := make([]adminRoute, 0, len(contentPublicRouteContract))
	for _, match := range matches {
		path := "/api/v1/public" + match[2]
		if isContentPublicRoute(path) {
			routes = append(routes, adminRoute{method: match[1], object: path})
		}
	}
	return routes, nil
}

func isContentAdminRoute(object string) bool {
	return object == "/admin/posts" || strings.HasPrefix(object, "/admin/posts/") ||
		object == "/admin/post-categories" || strings.HasPrefix(object, "/admin/post-categories/") ||
		object == "/admin/banners" || strings.HasPrefix(object, "/admin/banners/") ||
		object == "/admin/media" || strings.HasPrefix(object, "/admin/media/")
}

func isContentPublicRoute(path string) bool {
	return path == "/api/v1/public/posts" || path == "/api/v1/public/posts/:slug" ||
		path == "/api/v1/public/banners" || path == "/api/v1/public/post-categories"
}

func assertRouteSetsEqual(t *testing.T, want, got []adminRoute) {
	t.Helper()
	normalize := func(routes []adminRoute) []string {
		result := make([]string, 0, len(routes))
		for _, route := range routes {
			result = append(result, route.method+" "+route.object)
		}
		sort.Strings(result)
		return result
	}
	wantRoutes := normalize(want)
	gotRoutes := normalize(got)
	if strings.Join(wantRoutes, "\n") != strings.Join(gotRoutes, "\n") {
		t.Fatalf("content route contract mismatch\nwant:\n%s\ngot:\n%s", strings.Join(wantRoutes, "\n"), strings.Join(gotRoutes, "\n"))
	}
}

func setupContentRouteAccessTest(t *testing.T) (admincontract.Store, *authz.Service, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:content_route_access_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&admindomain.Admin{}); err != nil {
		t.Fatalf("auto migrate admins: %v", err)
	}

	adminRepo := adminstore.New(db)
	noPermission := &admindomain.Admin{Username: "content-none", PasswordHash: "hash"}
	operations := &admindomain.Admin{Username: "content-operations", PasswordHash: "hash"}
	if err := adminRepo.Create(noPermission); err != nil {
		t.Fatalf("create no-permission admin: %v", err)
	}
	if err := adminRepo.Create(operations); err != nil {
		t.Fatalf("create operations admin: %v", err)
	}

	authzService, err := authz.NewService(db)
	if err != nil {
		t.Fatalf("create authz service: %v", err)
	}
	if err := authzService.BootstrapBuiltinRoles(); err != nil {
		t.Fatalf("bootstrap builtin roles: %v", err)
	}
	if err := authzService.SetAdminRoles(operations.ID, []string{"operations"}); err != nil {
		t.Fatalf("assign operations role: %v", err)
	}

	return adminRepo, authzService,
		signContentAdminToken(t, noPermission),
		signContentAdminToken(t, operations)
}

func signContentAdminToken(t *testing.T, admin *admindomain.Admin) string {
	t.Helper()
	now := time.Now()
	claims := adminauthapp.JWTClaims{
		AdminID:      admin.ID,
		Username:     admin.Username,
		TokenVersion: admin.TokenVersion,
		Typ:          jwttoken.TypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(contentAdminJWTSecret))
	if err != nil {
		t.Fatalf("sign admin token: %v", err)
	}
	return signed
}

func newContentRouteAccessRouter(adminRepo admincontract.Store, authzService *authz.Service) *gin.Engine {
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware(contentAdminJWTSecret, adminRepo), middleware.AdminRBACMiddleware(authzService))
	admin.Use(func(c *gin.Context) {
		response.Success(c, nil)
		c.Abort()
	})
	contenttransport.RegisterAdminRoutes(admin, contenttransport.NewAdminHandler(nil, nil, nil, nil))
	return router
}
