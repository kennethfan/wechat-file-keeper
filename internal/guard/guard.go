// Package guard 是清理操作的安全闸门。
//
// 设计原则：默认拒绝。只有同时满足以下条件的目标才被允许操作：
//  1. 微信进程未运行；
//  2. 路径是绝对路径，且不含 ".." 穿越成分；
//  3. 路径位于当前账号库的消息区之下，且不等于消息区根目录本身。
package guard

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	// ErrWeChatRunning 微信正在运行，此时清理可能导致微信异常。
	ErrWeChatRunning = errors.New("微信正在运行，请先退出微信")
	// ErrEmptyPath 路径为空。
	ErrEmptyPath = errors.New("路径为空")
	// ErrRelativePath 路径不是绝对路径。
	ErrRelativePath = errors.New("路径必须是绝对路径")
	// ErrTraversal 路径包含 .. 穿越成分。
	ErrTraversal = errors.New("路径包含 .. 穿越成分")
	// ErrOutsideRoot 路径不在消息区之下。
	ErrOutsideRoot = errors.New("路径不在允许的清理范围内")
	// ErrRootItself 目标就是消息区根目录本身。
	ErrRootItself = errors.New("拒绝操作消息区根目录本身")
	// ErrNotFile 目标不是普通文件。
	ErrNotFile = errors.New("目标不是普通文件")
)

// Checker 校验清理目标是否安全。
type Checker struct {
	// MsgRoot 是当前账号库的消息区绝对路径。
	MsgRoot string
	// WeChatRunning 表示微信进程是否在运行。
	WeChatRunning bool
}

// Check 校验单个目标路径。
//
// 多个目标时应先逐个 Check 再执行，避免部分删除后才发现越界。
func (c Checker) Check(path string) error {
	if c.WeChatRunning {
		return ErrWeChatRunning
	}
	if strings.TrimSpace(path) == "" {
		return ErrEmptyPath
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %s", ErrRelativePath, path)
	}
	// 先查穿越成分，再比对根目录，避免 "a/../b" 这类写法绕过检查。
	if hasDotDot(path) {
		return fmt.Errorf("%w: %s", ErrTraversal, path)
	}
	clean := filepath.Clean(path)

	root := filepath.Clean(c.MsgRoot)
	if root == "" {
		return fmt.Errorf("%w: 消息区根目录未设置", ErrOutsideRoot)
	}
	if clean == root {
		return fmt.Errorf("%w: %s", ErrRootItself, path)
	}
	// 必须位于 root 之下：加分隔符避免 /a/msgfoo 被误判为在 /a/msg 之下。
	if !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrOutsideRoot, path)
	}
	return nil
}

// CheckAll 校验一批目标，任一失败即返回该错误。
func (c Checker) CheckAll(paths []string) error {
	for _, p := range paths {
		if err := c.Check(p); err != nil {
			return err
		}
	}
	return nil
}

// hasDotDot 判断路径中是否含 ".." 成分。
func hasDotDot(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	return false
}
