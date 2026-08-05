package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

// Config contains the information displayed by the client-facing interface.
type Config struct {
	Port            int
	ManagedByNotice string
	AgentVersion    string
}

type pageData struct {
	ManagedByNotice string
	AgentVersion    string
	AgentPort       int
}

// Run starts the local client-facing interface in the current desktop session.
// The service remains responsible for privileged operations; this process only
// displays local status and the managed-device notice.
func Run(ctx context.Context, cfg Config) error {
	if !Available() {
		return fmt.Errorf("no graphical environment available")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("invalid agent port: %d", cfg.Port)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start local interface: %w", err)
	}

	page, err := template.New("agent-status").Parse(pageTemplate)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("prepare local interface: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, pageData{
			ManagedByNotice: cfg.ManagedByNotice,
			AgentVersion:    cfg.AgentVersion,
			AgentPort:       cfg.Port,
		})
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := map[string]any{"active": false}
		client := &http.Client{Timeout: 2 * time.Second}
		resp, requestErr := client.Get("http://127.0.0.1:" + strconv.Itoa(cfg.Port) + "/health")
		if resp != nil {
			_ = resp.Body.Close()
		}
		if requestErr == nil && resp != nil && resp.StatusCode == http.StatusOK {
			status["active"] = true
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	server := &http.Server{Handler: mux}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()

	address := "http://" + listener.Addr().String()
	if err := openBrowser(address); err != nil {
		_ = server.Shutdown(context.Background())
		return fmt.Errorf("open local interface: %w", err)
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErr:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func openBrowser(address string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "windows":
		command = "rundll32.exe"
		args = []string{"url.dll,FileProtocolHandler", address}
	case "darwin":
		command = "open"
		args = []string{address}
	default:
		command = "xdg-open"
		args = []string{address}
	}
	return exec.Command(command, args...).Start()
}

const pageTemplate = `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LAN Commander</title>
<style>
:root { color-scheme: light dark; font-family: system-ui, sans-serif; }
body { margin: 0; min-height: 100vh; background: #101827; color: #e8eef7; display: grid; place-items: center; }
main { width: min(680px, calc(100% - 32px)); background: #172235; border: 1px solid #30405c; border-radius: 18px; padding: 28px; box-shadow: 0 18px 50px #0005; }
h1 { margin: 0 0 8px; font-size: 28px; } p { color: #b7c3d6; line-height: 1.5; }
.card { margin-top: 22px; padding: 18px; border-radius: 12px; background: #202f47; }
.status { display: flex; gap: 10px; align-items: center; font-size: 18px; font-weight: 650; }
.dot { width: 12px; height: 12px; border-radius: 50%; background: #e8a63b; } .dot.ok { background: #42d392; }
.meta { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 16px; } .meta div { color: #b7c3d6; }
strong { color: #fff; display: block; margin-top: 4px; } footer { margin-top: 22px; font-size: 13px; color: #8d9bb0; }
</style>
</head>
<body>
<main>
<h1>LAN Commander</h1>
<p>Interfaz visual del agente para consultar el estado de este equipo.</p>
<div class="card">
<div class="status"><span id="dot" class="dot"></span><span id="status">Comprobando el servicio...</span></div>
<div id="checked" class="meta"></div>
</div>
{{if .ManagedByNotice}}<p><strong>Equipo gestionado</strong>{{.ManagedByNotice}}</p>{{end}}
<footer>Agente {{.AgentVersion}} · Puerto de administración {{.AgentPort}}</footer>
</main>
<script>
const status = document.getElementById('status');
const dot = document.getElementById('dot');
const checked = document.getElementById('checked');
async function refresh() {
  try {
    const response = await fetch('/api/status', {cache: 'no-store'});
    const data = await response.json();
    const active = data.active === true;
    status.textContent = active ? 'Servicio activo y escuchando en la red local' : 'Servicio no disponible';
    dot.classList.toggle('ok', active);
  } catch (_) {
    status.textContent = 'Interfaz local no disponible';
    dot.classList.remove('ok');
  }
  checked.innerHTML = '<div>Última comprobación<strong>' + new Date().toLocaleTimeString() + '</strong></div>';
}
refresh(); setInterval(refresh, 3000);
</script>
</body>
</html>`
