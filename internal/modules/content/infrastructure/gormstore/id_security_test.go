package gormstore

import (
	"context"
	"github.com/dujiao-next/internal/modules/content/domain"
	"testing"
)

func TestContentIDsAreNeverSQLConditions(t *testing.T) {
	db := setupContentStoreTest(t)
	if err := db.Create(&domain.Post{Slug: "id-test", TitleJSON: map[string]interface{}{"en-US": "Test"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.Banner{Name: "id-test"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1 OR 1=1", "id > 0", "0", "-1", "", "18446744073709551616", " 1", "+1"} {
		t.Run(id, func(t *testing.T) {
			post, _ := NewPostStore(db).GetByID(context.Background(), id)
			banner, _ := NewBannerStore(db).GetByID(context.Background(), id)
			if post != nil || banner != nil {
				t.Fatal("invalid ID selected a record")
			}
		})
	}
	post, err := NewPostStore(db).GetByID(context.Background(), "1")
	if err != nil || post == nil {
		t.Fatalf("valid ID: %v", err)
	}
}
