//go:build e2e

// assert.go：e2e 断言辅助（查表/查计数），供多个测试复用。
package fixtures

import (
	"testing"

	"gorm.io/gorm"
)

// TableExists 断言表存在于当前 database（pg 用 current_schema()）。
func TableExists(t *testing.T, db *gorm.DB, table string) bool {
	t.Helper()
	var cnt int
	err := db.Raw("SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?", table).Scan(&cnt).Error
	if err != nil {
		t.Fatalf("check table %q: %v", table, err)
	}
	return cnt == 1
}

// LikesCount 返回指定帖子的点赞计数（post_counters.likes_count）。
func LikesCount(t *testing.T, db *gorm.DB, postID string) int {
	t.Helper()
	var n int64
	if err := db.Table("post_counters").Where("post_id = ?", postID).Select("likes_count").Scan(&n).Error; err != nil {
		t.Fatalf("query likes_count for %q: %v", postID, err)
	}
	return int(n)
}

// LikeExists 断言 likes 表存在 (userID, postID) 记录。
func LikeExists(t *testing.T, db *gorm.DB, userID, postID string) bool {
	t.Helper()
	var cnt int64
	if err := db.Table("likes").Where("user_id = ? AND post_id = ?", userID, postID).Count(&cnt).Error; err != nil {
		t.Fatalf("query likes (%s,%s): %v", userID, postID, err)
	}
	return cnt == 1
}
