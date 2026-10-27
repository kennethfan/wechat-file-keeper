package ui

import (
	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/view"
)

// enumerateLibraries 枚举 root 下全部账号库并算出各自实占大小。
//
// 统计大小要遍历整个账号库，放在启动阶段会拖慢界面，因此调用方
// 应在后台协程里执行。统计失败的账号库仍会列出，字节数记 0——
// 让用户看到它存在，好过静默丢弃。
func enumerateLibraries(root string) ([]view.LibraryOption, error) {
	libs, err := account.Discover(root)
	if err != nil {
		return nil, err
	}

	options := make([]view.LibraryOption, 0, len(libs))
	for _, lib := range libs {
		size, sizeErr := account.SizeOf(lib)
		if sizeErr != nil {
			size = 0
		}
		options = append(options, view.LibraryOption{Library: lib, Bytes: size})
	}
	return options, nil
}
