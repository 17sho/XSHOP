package integrationtest

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/dujiao-next/internal/app/container"
	userauthwiring "github.com/dujiao-next/internal/bootstrap/userauth"
	githubauthapp "github.com/dujiao-next/internal/modules/identity/githubauth/application"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	memberlevelapp "github.com/dujiao-next/internal/modules/memberlevel/application"
	memberleveldomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	memberlevelstore "github.com/dujiao-next/internal/modules/memberlevel/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func TestCurrentProfileReconcilesHistoricalMemberSpend(t *testing.T) {
	auth, otp, users, db := newUser2FATestServices(t)
	if err := db.AutoMigrate(&memberleveldomain.MemberLevel{}, &memberleveldomain.MemberLevelPrice{}); err != nil {
		t.Fatal(err)
	}
	ordinary := memberleveldomain.MemberLevel{NameJSON: jsonmap.JSON{"zh-CN": "test"}, Slug: "ordinary", SortOrder: 0, IsDefault: true, IsActive: true}
	if err := db.Create(&ordinary).Error; err != nil {
		t.Fatal(err)
	}
	user := userdomain.User{Email: "historical-profile@example.test", PasswordHash: "test-hash", Status: "active", MemberLevelID: ordinary.ID, TotalSpent: money.FromDecimal(decimal.RequireFromString("315.70"))}
	if err := users.Create(&user); err != nil {
		t.Fatal(err)
	}
	// The qualifying level is created after the spending, with no new payment event.
	silver := memberleveldomain.MemberLevel{NameJSON: jsonmap.JSON{"zh-CN": "test"}, Slug: "silver", SortOrder: 10, IsActive: true, SpendThreshold: money.FromDecimal(decimal.NewFromInt(300))}
	if err := db.Create(&silver).Error; err != nil {
		t.Fatal(err)
	}
	levels := memberlevelapp.NewService(memberlevelstore.NewLevelStore(db), memberlevelstore.NewPriceStore(db), memberlevelstore.NewUserStore(db))
	h := userauthwiring.New(&container.Container{UserAuthService: auth, UserTOTPService: otp, UserStore: users, MemberLevelService: levels, GitHubAuthService: &githubauthapp.Service{}}).Profile
	r := gin.New()
	r.GET("/profile", func(c *gin.Context) { c.Set("user_id", user.ID) }, h.GetCurrentUser)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/profile", nil))
		if w.Code != 200 {
			t.Fatalf("profile status=%d body=%s", w.Code, w.Body.String())
		}
		var response struct {
			Data struct {
				MemberLevelID uint `json:"member_level_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Data.MemberLevelID != silver.ID {
			t.Fatalf("profile level=%d, want silver=%d; body=%s", response.Data.MemberLevelID, silver.ID, w.Body.String())
		}
		persisted, err := users.GetByID(user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.MemberLevelID != silver.ID {
			t.Fatalf("persisted level=%d, want silver=%d", persisted.MemberLevelID, silver.ID)
		}
		if !persisted.TotalSpent.Decimal.Equal(decimal.RequireFromString("315.70")) {
			t.Fatalf("profile read changed spending: %s", persisted.TotalSpent.StringFixed(2))
		}
		if !persisted.TotalRecharged.Decimal.IsZero() {
			t.Fatalf("profile read changed recharge: %s", persisted.TotalRecharged.StringFixed(2))
		}
	}
}
