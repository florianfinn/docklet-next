// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original test-only HTTP bridge for the web adapter's real CSRF contract.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/labstack/echo/v5"
)

func main() {
	var input struct {
		Method  string      `json:"method"`
		Path    string      `json:"path"`
		Headers http.Header `json:"headers"`
		Body    string      `json:"body"`
	}
	if len(os.Args) != 2 || json.Unmarshal([]byte(os.Args[1]), &input) != nil {
		os.Exit(2)
	}
	router := echo.New()
	router.Use(middleware.NewCSRFMiddleware(&config.Config{AuthMode: "none", AppUrl: "https://hub.example.com", Environment: "production"}).Add())
	router.GET("/api/auth/mode", func(c *echo.Context) error { return c.JSON(200, map[string]string{"mode": "none"}) })
	router.POST("/api/backups", func(c *echo.Context) error {
		return c.JSON(202, map[string]string{"id": "example", "status": "running"})
	})
	router.PUT("/api/backups/policies", func(c *echo.Context) error { return c.JSON(200, map[string]any{"policies": []any{}}) })
	operation := func(c *echo.Context) error {
		return c.JSON(202, map[string]string{"backupId": "example", "activityId": "example-activity", "status": "running"})
	}
	router.POST("/api/backups/example/restore", operation)
	router.POST("/api/backups/upload", operation)
	router.DELETE("/api/backups/example", operation)
	req := httptest.NewRequest(input.Method, input.Path, strings.NewReader(input.Body))
	for name, values := range input.Headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	output, err := json.Marshal(struct {
		Status  int         `json:"status"`
		Headers http.Header `json:"headers"`
		Body    string      `json:"body"`
	}{rec.Code, rec.Header(), rec.Body.String()})
	if err != nil {
		os.Exit(2)
	}
	fmt.Println(string(output))
}
