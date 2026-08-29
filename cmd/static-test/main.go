// 临时实验:复刻线上 serveFromDist 逻辑的独立静态服务(9999 端口)
package main

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MinimaxFlora/Docker_Manager_License/web"
)

func main() {
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		serveFromDist(c, distFS, strings.TrimPrefix(p, "/"))
	})
	log.Println("static test server on :9999")
	r.Run(":9999")
}

func serveFromDist(c *gin.Context, distFS fs.FS, name string) {
	if name == "" {
		name = "index.html"
	}
	if raw, err := fs.ReadFile(distFS, name); err == nil {
		c.Data(http.StatusOK, contentType(name), raw)
		return
	}
	raw, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", raw)
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
