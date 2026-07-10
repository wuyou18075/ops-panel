package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDevice(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36"
	got := parseDevice(ua)
	if !strings.Contains(got, "Chrome") || !strings.Contains(got, "macOS") {
		t.Errorf("解析设备错: %s", got)
	}
}

func TestPanelLogin_InsertCapClear(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 120; i++ {
		insertPanelLogin(int64(i), "1.1.1.1", "中国·北京", "Chrome · macOS", "admin")
	}
	list := listPanelLogin()
	if len(list) != 100 {
		t.Fatalf("应裁剪到 100，got %d", len(list))
	}
	if list[0].TS < list[len(list)-1].TS {
		t.Errorf("应按时间倒序")
	}
	clearPanelLogin()
	if len(listPanelLogin()) != 0 {
		t.Errorf("清空失败")
	}
}

func TestSSHLogin_CapAndWeeklyFails(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Unix()
	for i := 0; i < 210; i++ {
		insertSSHLogin("n1", now, "1.1.1.1", "", "root", "password", false)
	}
	if got := len(listSSHLogin("n1")); got != 200 {
		t.Fatalf("应裁剪 200，got %d", got)
	}
	if wf := weeklySSHFails("n1", time.Now()); wf < 200 {
		t.Errorf("周失败数应≥200，got %d", wf)
	}
	resetSSHFails("n1", time.Now().Unix()+1) // 基线设为将来 → 计数归零
	if wf := weeklySSHFails("n1", time.Now()); wf != 0 {
		t.Errorf("重置后应为 0，got %d", wf)
	}
}
