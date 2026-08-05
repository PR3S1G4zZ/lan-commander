package main

import (
	"context"
	"crypto/tls"
	"embed"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

type Status struct {
	Active       bool   `json:"active"`
	Port         int    `json:"port"`
	Secure       bool   `json:"secure"`
	CheckedAt    string `json:"checked_at"`
	Message      string `json:"message"`
	ManagedBy    string `json:"managed_by"`
	AgentVersion string `json:"agent_version"`
}

type App struct {
	ctx          context.Context
	port         int
	secure       bool
	managedBy    string
	agentVersion string
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetStatus checks the service directly from the desktop app. No browser or
// visible localhost page is involved; the HTTP call stays inside the local
// process boundary and is never exposed by this UI.
func (a *App) GetStatus() Status {
	status := Status{
		Port:         a.port,
		Secure:       a.secure,
		CheckedAt:    time.Now().Format(time.RFC3339),
		ManagedBy:    a.managedBy,
		AgentVersion: a.agentVersion,
	}

	scheme := "http"
	transport := http.DefaultTransport
	if a.secure {
		scheme = "https"
		transport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}}
	}

	client := &http.Client{Timeout: 2 * time.Second, Transport: transport}
	url := fmt.Sprintf("%s://127.0.0.1:%d/health", scheme, a.port)
	resp, err := client.Get(url)
	if err != nil {
		status.Message = "Servicio no disponible"
		return status
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		status.Active = true
		status.Message = "Servicio activo y protegido"
		return status
	}
	status.Message = "Servicio respondió con un error"
	return status
}

func (a *App) Quit() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func main() {
	port := flag.Int("port", 9474, "Agent service port")
	secure := flag.Bool("secure", false, "Use HTTPS for the local health check")
	managedBy := flag.String("managed-by-notice", "", "Organisation shown in the desktop UI")
	version := flag.String("version", "1.0.0", "Agent version shown in the desktop UI")
	flag.Parse()

	app := &App{port: *port, secure: *secure, managedBy: *managedBy, agentVersion: *version}
	if err := wails.Run(&options.App{
		Title:            "LAN Commander Agent",
		Width:            560,
		Height:           460,
		MinWidth:         460,
		MinHeight:        380,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	}); err != nil {
		panic(err)
	}
}
