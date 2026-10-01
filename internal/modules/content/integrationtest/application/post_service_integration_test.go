package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentcontract "github.com/dujiao-next/internal/modules/content/contract"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	"github.com/dujiao-next/internal/modules/content/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPostServiceForTest(t *testing.T) (*contentapp.PostService, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:post_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&contentdomain.PostCategory{}, &contentdomain.Post{}, &contentdomain.PostProduct{}); err != nil {
		t.Fatalf("auto migrate post tables failed: %v", err)
	}

	postStore := gormstore.NewPostStore(db)
	return contentapp.NewPostService(
		postStore,
		postStore,
		gormstore.NewPostCategoryStore(db),
		contentapp.SystemClock{},
	), db
}

func createPostCategoryFixture(t *testing.T, db *gorm.DB, slug string, parentID *uint) contentdomain.PostCategory {
	t.Helper()

	category := contentdomain.PostCategory{
		ParentID: parentID,
		Slug:     slug,
		NameJSON: jsonmap.JSON{
			"zh-CN": slug,
		},
		IsActive: true,
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create post category fixture failed: %v", err)
	}
	return category
}

func createPostFixture(t *testing.T, db *gorm.DB, slug string, postType string, categoryID *uint) contentdomain.Post {
	t.Helper()

	post := contentdomain.Post{
		Slug:       slug,
		Type:       postType,
		TitleJSON:  jsonmap.JSON{"zh-CN": slug},
		CategoryID: categoryID,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post fixture failed: %v", err)
	}
	return post
}

func TestPostServiceCreateRejectsNoticeCategory(t *testing.T) {
	svc, db := newPostServiceForTest(t)
	leaf := createPostCategoryFixture(t, db, "announcements", nil)

	_, err := svc.Create(context.Background(), contentapp.CreatePostInput{
		Slug:       "notice-with-category",
		Type:       constants.PostTypeNotice,
		TitleJSON:  map[string]interface{}{"zh-CN": "notice-with-category"},
		CategoryID: &leaf.ID,
	})
	if err != contentcontract.ErrPostNoticeCategoryUnsupported {
		t.Fatalf("expected domaincontent.ErrPostNoticeCategoryUnsupported, got %v", err)
	}
}

func TestPostServiceListPublicFiltersDraftsAndOrdersByPublishedAt(t *testing.T) {
	svc, db := newPostServiceForTest(t)
	now := time.Now().UTC()
	olderPublishedAt := now.Add(-2 * time.Hour)
	newerPublishedAt := now.Add(-time.Hour)

	fixtures := []contentdomain.Post{
		{
			Slug:        "older-published",
			Type:        constants.PostTypeNotice,
			TitleJSON:   jsonmap.JSON{"zh-CN": "older"},
			IsPublished: true,
			PublishedAt: &olderPublishedAt,
		},
		{
			Slug:        "newer-published",
			Type:        constants.PostTypeNotice,
			TitleJSON:   jsonmap.JSON{"zh-CN": "newer"},
			IsPublished: true,
			PublishedAt: &newerPublishedAt,
		},
		{
			Slug:        "draft",
			Type:        constants.PostTypeNotice,
			TitleJSON:   jsonmap.JSON{"zh-CN": "draft"},
			IsPublished: false,
		},
	}
	for i := range fixtures {
		if err := db.Create(&fixtures[i]).Error; err != nil {
			t.Fatalf("create post fixture %q: %v", fixtures[i].Slug, err)
		}
	}

	posts, total, err := svc.ListPublic(context.Background(), contentapp.PublicPostQuery{
		Type:     constants.PostTypeNotice,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("list public posts: %v", err)
	}
	if total != 2 || len(posts) != 2 {
		t.Fatalf("expected two published posts, total=%d posts=%#v", total, posts)
	}
	if posts[0].Slug != "newer-published" || posts[1].Slug != "older-published" {
		t.Fatalf("unexpected public post order: %v, %v", posts[0].Slug, posts[1].Slug)
	}
}

func TestPostServiceGetPublicBySlugHidesDraftsAndMissingPosts(t *testing.T) {
	svc, db := newPostServiceForTest(t)
	_ = createPostFixture(t, db, "draft-post", constants.PostTypeNotice, nil)

	if _, err := svc.GetPublicBySlug(context.Background(), "draft-post"); err != contentcontract.ErrNotFound {
		t.Fatalf("expected draft to be hidden as not found, got %v", err)
	}
	if _, err := svc.GetPublicBySlug(context.Background(), "missing-post"); err != contentcontract.ErrNotFound {
		t.Fatalf("expected missing post to return domaincontent.ErrNotFound, got %v", err)
	}
}

func TestPostServiceCreateRejectsDuplicateSlug(t *testing.T) {
	svc, db := newPostServiceForTest(t)
	_ = createPostFixture(t, db, "duplicate-slug", constants.PostTypeNotice, nil)

	_, err := svc.Create(context.Background(), contentapp.CreatePostInput{
		Slug:      "duplicate-slug",
		Type:      constants.PostTypeNotice,
		TitleJSON: map[string]interface{}{"zh-CN": "duplicate"},
	})
	if err != contentcontract.ErrSlugExists {
		t.Fatalf("expected domaincontent.ErrSlugExists, got %v", err)
	}
}

func TestPostServicePublishedAtIsSetOnlyOnFirstPublish(t *testing.T) {
	svc, _ := newPostServiceForTest(t)
	draft, err := svc.Create(context.Background(), contentapp.CreatePostInput{
		Slug:      "publish-transition",
		Type:      constants.PostTypeNotice,
		TitleJSON: map[string]interface{}{"zh-CN": "publish-transition"},
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if draft.PublishedAt != nil {
		t.Fatalf("draft should not have published_at, got %v", draft.PublishedAt)
	}

	published := true
	firstPublish, err := svc.Update(context.Background(), fmt.Sprintf("%d", draft.ID), contentapp.CreatePostInput{
		Slug:        draft.Slug,
		Type:        constants.PostTypeNotice,
		TitleJSON:   map[string]interface{}{"zh-CN": "first publish"},
		IsPublished: &published,
	})
	if err != nil {
		t.Fatalf("publish draft: %v", err)
	}
	if firstPublish.PublishedAt == nil {
		t.Fatal("first publish should set published_at")
	}
	firstPublishedAt := *firstPublish.PublishedAt

	published = false
	if _, err := svc.Update(context.Background(), fmt.Sprintf("%d", draft.ID), contentapp.CreatePostInput{
		Slug:        draft.Slug,
		Type:        constants.PostTypeNotice,
		TitleJSON:   map[string]interface{}{"zh-CN": "unpublished"},
		IsPublished: &published,
	}); err != nil {
		t.Fatalf("unpublish post: %v", err)
	}

	published = true
	republished, err := svc.Update(context.Background(), fmt.Sprintf("%d", draft.ID), contentapp.CreatePostInput{
		Slug:        draft.Slug,
		Type:        constants.PostTypeNotice,
		TitleJSON:   map[string]interface{}{"zh-CN": "republished"},
		IsPublished: &published,
	})
	if err != nil {
		t.Fatalf("republish post: %v", err)
	}
	if republished.PublishedAt == nil || !republished.PublishedAt.Equal(firstPublishedAt) {
		t.Fatalf("republish must preserve first published_at, first=%v got=%v", firstPublishedAt, republished.PublishedAt)
	}
}
