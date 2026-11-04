package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/discovery"
	"github.com/liyiligang/loom/internal/model"
)

type API struct {
	service   *controller.Service
	discovery *discovery.Scanner
	assets    fs.FS
	origins   map[string]bool
	hosts     map[string]bool
	ctx       context.Context
}

func New(ctx context.Context, service *controller.Service, scanner *discovery.Scanner, assets fs.FS, address string) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	a := &API{service: service, discovery: scanner, assets: assets, ctx: ctx, origins: map[string]bool{}, hosts: map[string]bool{}}
	_, port, _ := strings.Cut(address, ":")
	if target, err := url.Parse("http://" + address); err == nil {
		port = target.Port()
	}
	for _, host := range []string{address, "localhost:" + port} {
		a.hosts[host] = true
		a.origins["http://"+host] = true
	}
	for _, origin := range []string{"http://127.0.0.1:5173", "http://localhost:5173"} {
		a.origins[origin] = true
	}
	router := gin.New()
	router.Use(gin.Recovery(), a.guard)
	router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "version": model.Version}) })
	api := router.Group("/api/v1")
	api.GET("/bootstrap", func(c *gin.Context) { raw, err := service.Bootstrap(); a.respond(c, raw, err) })
	api.GET("/sessions/:id", func(c *gin.Context) { raw, err := service.Detail(c.Param("id")); a.respond(c, raw, err) })
	api.POST("/endpoints", func(c *gin.Context) { a.saveEndpoint(c, "") })
	api.PATCH("/endpoints/:id", func(c *gin.Context) { a.saveEndpoint(c, c.Param("id")) })
	api.POST("/endpoints/:id/connect", func(c *gin.Context) { a.result(c, service.Connect(c.Param("id"))) })
	api.POST("/endpoints/:id/disconnect", func(c *gin.Context) { a.result(c, service.Disconnect(c.Param("id"))) })
	api.POST("/sessions", func(c *gin.Context) {
		var input controller.SessionInput
		if !a.decode(c, &input) {
			return
		}
		id, err := service.CreateSession(c.Request.Context(), input)
		if err != nil {
			a.fail(c, err)
			return
		}
		c.JSON(201, gin.H{"id": id})
	})
	api.PATCH("/sessions/:id", func(c *gin.Context) {
		var input struct {
			Title string `json:"title"`
		}
		if !a.decode(c, &input) {
			return
		}
		a.result(c, service.Rename(c.Param("id"), input.Title))
	})
	api.POST("/sessions/:id/restore", func(c *gin.Context) { a.result(c, service.Restore(c.Request.Context(), c.Param("id"))) })
	api.POST("/sessions/:id/turns", func(c *gin.Context) {
		var input controller.TurnInput
		if !a.decode(c, &input) {
			return
		}
		id, err := service.Submit(c.Param("id"), input)
		if err != nil {
			a.fail(c, err)
			return
		}
		c.JSON(202, gin.H{"id": id})
	})
	api.POST("/turns/:id/interrupt", func(c *gin.Context) { a.result(c, service.Interrupt(c.Request.Context(), c.Param("id"))) })
	api.POST("/interactions/:id/respond", func(c *gin.Context) {
		var input controller.ReplyInput
		if !a.decode(c, &input) {
			return
		}
		a.result(c, service.Respond(c.Param("id"), input))
	})
	api.POST("/discovery", func(c *gin.Context) { c.JSON(202, scanner.Start()) })
	api.GET("/discovery/:id", func(c *gin.Context) {
		run := scanner.Get()
		if run.ID != c.Param("id") {
			a.fail(c, &controller.Error{Code: "not_found", Message: "Discovery run not found.", Status: 404})
			return
		}
		c.JSON(200, run)
	})
	api.GET("/events", a.events)
	router.NoRoute(a.static)
	return router
}

func (a *API) guard(c *gin.Context) {
	if !a.hosts[c.Request.Host] || (c.GetHeader("Origin") != "" && !a.origins[c.GetHeader("Origin")]) {
		c.AbortWithStatusJSON(403, gin.H{"error": gin.H{"code": "origin_rejected", "message": "Use the local Loom browser address."}})
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Frame-Options", "DENY")
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.Header("Cache-Control", "no-store")
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			media, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
			if media != "application/json" {
				c.AbortWithStatusJSON(415, gin.H{"error": gin.H{"code": "json_required", "message": "Use an application/json request body."}})
				return
			}
		}
	}
	c.Next()
}

func (a *API) decode(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		a.fail(c, &controller.Error{Code: "invalid_json", Message: "The request body is invalid or too large.", Status: 400})
		return false
	}
	return true
}

func (a *API) saveEndpoint(c *gin.Context, id string) {
	var input controller.EndpointInput
	if !a.decode(c, &input) {
		return
	}
	id, err := a.service.SaveEndpoint(id, input)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"id": id})
}

func (a *API) fail(c *gin.Context, err error) {
	status, code, message := 502, "agent_error", err.Error()
	var domain *controller.Error
	if errors.As(err, &domain) {
		status, code, message = domain.Status, domain.Code, domain.Message
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message, "requestId": model.ID()}})
}
func (a *API) result(c *gin.Context, err error) {
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) respond(c *gin.Context, raw []byte, err error) {
	if err != nil {
		a.fail(c, err)
		return
	}
	c.Data(200, "application/json", raw)
}

func (a *API) static(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || (c.Request.Method != "GET" && c.Request.Method != "HEAD") {
		c.Status(404)
		return
	}
	file := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
	if file == "." || file == "" {
		file = "index.html"
	}
	raw, err := fs.ReadFile(a.assets, file)
	if err != nil {
		if path.Ext(file) != "" {
			c.Status(404)
			return
		}
		file = "index.html"
		raw, err = fs.ReadFile(a.assets, file)
	}
	if err != nil {
		c.Status(404)
		return
	}
	if file == "index.html" {
		c.Header("Cache-Control", "no-cache")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	} else {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	}
	c.Data(200, mime.TypeByExtension(path.Ext(file)), raw)
}

func (a *API) events(c *gin.Context) {
	after, err := strconv.ParseInt(c.DefaultQuery("afterCursor", "0"), 10, 64)
	if err != nil || after < 0 {
		a.fail(c, &controller.Error{Code: "invalid_cursor", Message: "The event cursor is invalid.", Status: 400})
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return a.origins[r.Header.Get("Origin")] }}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		changed := a.service.Watch()
		events, err := a.service.Events(after)
		if err != nil {
			return
		}
		for _, event := range events {
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if conn.WriteJSON(event) != nil {
				return
			}
			after = event.ID
		}
		if len(events) > 0 {
			continue
		}
		select {
		case <-changed:
		case <-done:
			return
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
				return
			}
		}
	}
}
