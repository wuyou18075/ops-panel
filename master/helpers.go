package main

import (
	"io/fs"
)

// fsSub 封装 fs.Sub，便于从 embed.FS 取子目录。
func fsSub(fsys fs.FS, dir string) (fs.FS, error) {
	return fs.Sub(fsys, dir)
}
