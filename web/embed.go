// Package web 嵌入前端构建产物(dist/),单二进制部署。
//
// 构建顺序:先 `npm run build` 生成 web/dist,再 `go build`(embed 需要目录存在)。
// CI 中 go 检查阶段用 .gitkeep 占位满足 embed 约束(见 go-checks.yml)。
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
