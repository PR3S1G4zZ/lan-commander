# Modo cliente del agente — Fase 1 (Transparencia) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** El usuario de cualquier PC gestionada ve que el agente corre y qué se hizo en su equipo, mediante un ícono de bandeja y un registro de actividad local.

**Architecture:** El binario `lan-agent` gana un modo `--ui` que corre como proceso separado en la sesión del usuario. El servicio registra cada acción del administrador en un archivo JSONL y la difunde por un canal local (named pipe en Windows, socket Unix en Linux) con una superficie de solo dos operaciones de lectura. La difusión nunca bloquea: la persistencia es la fuente de verdad.

**Tech Stack:** Go 1.26.3, `fyne.io/fyne/v2` (bandeja y ventana), `github.com/Microsoft/go-winio` (named pipe con ACL), biblioteca estándar para el resto.

**Spec:** `docs/superpowers/specs/2026-07-27-agent-client-ui-design.md`

## Global Constraints

- Módulo Go del agente: `github.com/mediacode/lan-commander/agent`. Todo import interno usa ese prefijo.
- Go 1.26.3. No introducir dependencias más allá de `fyne.io/fyne/v2` y `github.com/Microsoft/go-winio`.
- Terminología fija en todo texto visible al usuario: "personal de sistemas" (nunca "admin" ni "root"), "sesión de soporte" (conexión activa), "registro de actividad" (historial).
- Todos los textos de la interfaz de cliente van en español, exactamente como están transcritos en la sección "5b. Textos de la interfaz" de la spec. No parafrasear.
- `Append` del registro de actividad **nunca** debe bloquear ni devolver error que aborte una acción del administrador. Un fallo de auditoría se registra y la acción procede.
- Ningún string de comando, ruta ni argumento puede cruzar el canal local. Fase 1 solo expone dos operaciones de lectura.
- Rutas de datos: `%ProgramData%\LAN Commander\` en Windows, `/var/lib/lan-commander/` en Linux. Escribibles solo por SYSTEM/root.
- El registro de actividad es **por equipo, no por usuario**. Las acciones de autoservicio anotan el usuario en `Actor`.

---

### Task 0: Inicializar control de versiones

Este proyecto no está bajo git. Sin repo, las tareas siguientes no pueden commitear y no hay forma de revertir un cambio fallido.

**Files:**
- Create: `.gitignore`

- [ ] **Step 1: Verificar que efectivamente no hay repo**

Run: `cd C:/Proyectos/lan-commander && git rev-parse --is-inside-work-tree`
Expected: FALLA con `fatal: not a git repository`

- [ ] **Step 2: Inicializar el repo**

```bash
cd C:/Proyectos/lan-commander && git init
```

- [ ] **Step 3: Crear `.gitignore`**

Contenido exacto de `C:/Proyectos/lan-commander/.gitignore`:

```gitignore
# Artefactos de build
control-center/build/bin/
agent/build/
installers/windows/lan-agent.exe
installers/linux/lan-agent-linux

# Dependencias
node_modules/

# Bases de datos y datos locales
*.db
*.jsonl

# Logs
*.log

# SO / editores
Thumbs.db
.DS_Store
.vscode/
.idea/
```

- [ ] **Step 4: Verificar que no se van a commitear secretos ni binarios**

Run: `cd C:/Proyectos/lan-commander && git add -A && git status --short | head -40`
Expected: listado sin `node_modules/`, sin `.exe`, sin `.db`. Si aparece alguno, corregir `.gitignore` antes de continuar.

Revisar además que ningún archivo listado contenga tokens de autenticación reales de la flota. Los instaladores solo deben tener valores por defecto vacíos.

- [ ] **Step 5: Commit inicial**

```bash
cd C:/Proyectos/lan-commander && git add -A && git commit -m "chore: initialize repository"
```

---

### Task 1: Verificar viabilidad de Fyne (puerta de riesgo)

La spec identifica esto como el riesgo principal: Fyne enlaza OpenGL/X11 vía CGO y esas bibliotecas se resuelven **al cargar el proceso**, no al usarlas. Si enlazar Fyne impide que el servicio arranque, todo el enfoque de binario único cae y hay que volver a un binario de UI separado. Descubrirlo ahora cuesta una hora; descubrirlo con la UI escrita cuesta la fase entera.

**Files:**
- Create: `agent/internal/ui/probe.go`
- Create: `agent/internal/ui/probe_test.go`
- Modify: `agent/go.mod` (añade `fyne.io/fyne/v2`)

**Interfaces:**
- Consumes: nada.
- Produces: `ui.Available() bool` — informa si el entorno gráfico está disponible. Las tareas 5 y 6 lo usan para decidir si arrancar la interfaz.

- [ ] **Step 1: Añadir la dependencia**

```bash
cd C:/Proyectos/lan-commander/agent && go get fyne.io/fyne/v2@latest
```

- [ ] **Step 2: Escribir el probe**

Contenido de `agent/internal/ui/probe.go`:

```go
// Package ui implements the client-facing interface that runs in the user's
// desktop session. It is never used by the service process.
package ui

import "os"

// Available reports whether a graphical environment is present. On Linux a
// headless server has no DISPLAY/WAYLAND_DISPLAY, and starting the interface
// there would fail; the service must keep running regardless.
func Available() bool {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return isWindows()
}
```

Contenido de `agent/internal/ui/probe_windows.go`:

```go
//go:build windows

package ui

func isWindows() bool { return true }
```

Contenido de `agent/internal/ui/probe_other.go`:

```go
//go:build !windows

package ui

func isWindows() bool { return false }
```

- [ ] **Step 3: Escribir el test que falla**

Contenido de `agent/internal/ui/probe_test.go`:

```go
package ui

import (
	"runtime"
	"testing"
)

func TestAvailableWithDisplaySet(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	if !Available() {
		t.Fatal("Available() = false with DISPLAY set, want true")
	}
}

func TestAvailableHeadless(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	got := Available()
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("Available() = %v on %s without display, want %v", got, runtime.GOOS, want)
	}
}
```

- [ ] **Step 4: Correr el test**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/ui/ -v`
Expected: PASS ambos tests.

- [ ] **Step 5: La verificación que importa — que el servicio siga compilando y arrancando**

Run: `cd C:/Proyectos/lan-commander/agent && go build ./...`
Expected: compila sin errores.

Run: `cd C:/Proyectos/lan-commander/agent && go build -o build/lan-agent-probe.exe ./cmd/lan-agent && ./build/lan-agent-probe.exe --port 19474 --discovery=false`
Expected: arranca e imprime el banner y `Listening on ws://0.0.0.0:19474`. Cortar con Ctrl+C.

Este paso es la puerta: si el binario **no arranca** tras enlazar Fyne, detener el plan y escalar. El enfoque de binario único no es viable y hay que replanificar sobre el Enfoque A (binario de UI separado) del ADR.

- [ ] **Step 6: Registrar el resultado en la spec**

Añadir al final de la sección "Riesgos" de `docs/superpowers/specs/2026-07-27-agent-client-ui-design.md`:

```markdown
**Verificación de Fyne (Tarea 1, Fase 1):** ejecutada el {fecha}. Resultado:
{arranca / no arranca} en {SO y versión}. {Si no arranca: se replanifica sobre el
Enfoque A.}
```

- [ ] **Step 7: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/ui agent/go.mod agent/go.sum docs/superpowers/specs/ && git commit -m "feat(agent): add UI availability probe and verify Fyne linkage"
```

---

### Task 2: Paquete `activitylog` — tipo Event y persistencia

**Files:**
- Create: `agent/internal/activitylog/activitylog.go`
- Test: `agent/internal/activitylog/activitylog_test.go`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `type Event struct { Timestamp time.Time; Action, Actor, Detail, Outcome string }`
  - `const OutcomeSuccess = "success"`, `OutcomeError = "error"`
  - `func Open(path string, opts ...Option) (*Log, error)`
  - `func WithMaxBytes(n int64) Option`
  - `func (l *Log) Append(e Event) error`
  - `func (l *Log) Recent(n int) ([]Event, error)`
  - `func (l *Log) Close() error`

- [ ] **Step 1: Escribir el test que falla**

Contenido de `agent/internal/activitylog/activitylog_test.go`:

```go
package activitylog

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendThenRecent(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(filepath.Join(dir, "activity.jsonl"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer l.Close()

	e := Event{
		Timestamp: time.Date(2026, 7, 27, 14, 2, 0, 0, time.UTC),
		Action:    "screenshot",
		Actor:     "192.168.1.10",
		Detail:    "pantalla capturada",
		Outcome:   OutcomeSuccess,
	}
	if err := l.Append(e); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	got, err := l.Recent(10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Recent() returned %d events, want 1", len(got))
	}
	if got[0].Action != "screenshot" || got[0].Actor != "192.168.1.10" {
		t.Errorf("Recent()[0] = %+v, want action=screenshot actor=192.168.1.10", got[0])
	}
	if !got[0].Timestamp.Equal(e.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got[0].Timestamp, e.Timestamp)
	}
}

func TestRecentReturnsNewestLast(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "activity.jsonl"))
	defer l.Close()

	for _, a := range []string{"first", "second", "third"} {
		if err := l.Append(Event{Action: a, Outcome: OutcomeSuccess}); err != nil {
			t.Fatalf("Append(%s) error = %v", a, err)
		}
	}

	got, _ := l.Recent(2)
	if len(got) != 2 {
		t.Fatalf("Recent(2) returned %d events, want 2", len(got))
	}
	if got[0].Action != "second" || got[1].Action != "third" {
		t.Errorf("Recent(2) = [%s %s], want [second third]", got[0].Action, got[1].Action)
	}
}

func TestRecentToleratesTruncatedLastLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "activity.jsonl")
	l, _ := Open(path)
	if err := l.Append(Event{Action: "complete", Outcome: OutcomeSuccess}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	l.Close()

	// Simulate a power cut mid-write: append a partial JSON line.
	appendRaw(t, path, `{"action":"trunca`)

	l2, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after truncation error = %v", err)
	}
	defer l2.Close()

	got, err := l2.Recent(10)
	if err != nil {
		t.Fatalf("Recent() after truncation error = %v", err)
	}
	if len(got) != 1 || got[0].Action != "complete" {
		t.Fatalf("Recent() = %+v, want only the complete event", got)
	}
}

func TestRotationPreservesPreviousEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "activity.jsonl")
	// Tiny threshold so a couple of events force a rotation.
	l, err := Open(path, WithMaxBytes(200))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer l.Close()

	for i := 0; i < 10; i++ {
		if err := l.Append(Event{Action: "exec_command", Detail: "comando de relleno", Outcome: OutcomeSuccess}); err != nil {
			t.Fatalf("Append(%d) error = %v", i, err)
		}
	}

	got, err := l.Recent(10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	// Rotation keeps one previous file, so all 10 must still be reachable.
	if len(got) != 10 {
		t.Errorf("Recent(10) returned %d events after rotation, want 10", len(got))
	}
}
```

Y el helper, en el mismo archivo:

```go
func appendRaw(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("cannot open %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("cannot write raw: %v", err)
	}
}
```

Añadir `"os"` a los imports del test.

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/activitylog/ -v`
Expected: FALLA con `undefined: Open`.

- [ ] **Step 3: Escribir la implementación**

Contenido de `agent/internal/activitylog/activitylog.go`:

```go
// Package activitylog records every action an administrator performs on this
// machine and makes it readable by the local user's interface. Persistence is
// the source of truth: the file survives the interface not running.
package activitylog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Outcome values for Event.Outcome.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// defaultMaxBytes is the size at which the log rotates.
const defaultMaxBytes int64 = 5 * 1024 * 1024

// Event is a single recorded action. Detail is a human-readable summary and
// must never contain file contents or command output — the log is shown to the
// machine's user, not to the administrator.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	Detail    string    `json:"detail"`
	Outcome   string    `json:"outcome"`
}

// Option configures a Log at open time.
type Option func(*Log)

// WithMaxBytes overrides the rotation threshold. Used by tests.
func WithMaxBytes(n int64) Option {
	return func(l *Log) { l.maxBytes = n }
}

// Log is an append-only activity log with a single rotated predecessor.
type Log struct {
	path     string
	maxBytes int64

	mu   sync.Mutex
	file *os.File
	size int64
}

// Open opens or creates the log at path, creating parent directories.
func Open(path string, opts ...Option) (*Log, error) {
	l := &Log{path: path, maxBytes: defaultMaxBytes}
	for _, opt := range opts {
		opt(l)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("cannot create log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open activity log: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot stat activity log: %w", err)
	}

	l.file = f
	l.size = info.Size()
	return l, nil
}

// Append persists one event. It rotates first when the event would exceed the
// size threshold.
func (l *Log) Append(e Event) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cannot encode event: %w", err)
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.size+int64(len(line)) > l.maxBytes {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}

	n, err := l.file.Write(line)
	l.size += int64(n)
	if err != nil {
		return fmt.Errorf("cannot write event: %w", err)
	}
	return nil
}

// rotateLocked renames the current file to path+".1" and starts a fresh one.
// The caller must hold l.mu.
func (l *Log) rotateLocked() error {
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("cannot close log for rotation: %w", err)
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot rotate log: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot reopen log after rotation: %w", err)
	}
	l.file = f
	l.size = 0
	return nil
}

// Recent returns up to n events, oldest first. It reads the rotated
// predecessor when the current file holds fewer than n, so a rotation does not
// visibly erase the user's history.
func (l *Log) Recent(n int) ([]Event, error) {
	if n <= 0 {
		return nil, nil
	}

	events, err := readEvents(l.path)
	if err != nil {
		return nil, err
	}

	if len(events) < n {
		older, err := readEvents(l.path + ".1")
		if err != nil {
			return nil, err
		}
		events = append(older, events...)
	}

	if len(events) > n {
		events = events[len(events)-n:]
	}
	return events, nil
}

// readEvents parses a JSONL file, skipping malformed lines. A partial final
// line is expected after an abrupt shutdown and is not an error.
func readEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read activity log: %w", err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // truncated or corrupt line: skip, do not fail
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot scan activity log: %w", err)
	}
	return events, nil
}

// Close releases the underlying file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
```

- [ ] **Step 4: Correr los tests**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/activitylog/ -v`
Expected: PASS los cuatro tests.

- [ ] **Step 5: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/activitylog && git commit -m "feat(agent): add activity log with rotation and truncation tolerance"
```

---

### Task 3: `activitylog` — difusión que nunca bloquea

Esta es la propiedad que motivó el diseño: si el proceso de UI se cuelga, `Append` debe retornar igual, porque un `Append` bloqueado propagaría el bloqueo hasta el handler del administrador.

**Files:**
- Modify: `agent/internal/activitylog/activitylog.go`
- Test: `agent/internal/activitylog/subscribe_test.go`

**Interfaces:**
- Consumes: `Log`, `Event` de la Tarea 2.
- Produces:
  - `func (l *Log) Subscribe() (<-chan Event, func())` — el segundo valor cancela la suscripción.
  - `func (l *Log) Dropped() uint64` — eventos descartados por suscriptores saturados.
  - `const subscriberBuffer = 64`

- [ ] **Step 1: Escribir el test que falla**

Contenido de `agent/internal/activitylog/subscribe_test.go`:

```go
package activitylog

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSubscribeReceivesAppendedEvents(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "activity.jsonl"))
	defer l.Close()

	ch, cancel := l.Subscribe()
	defer cancel()

	if err := l.Append(Event{Action: "exec_command", Outcome: OutcomeSuccess}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	select {
	case got := <-ch:
		if got.Action != "exec_command" {
			t.Errorf("received action = %q, want exec_command", got.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}

func TestSubscribeDeliversToAllSubscribers(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "activity.jsonl"))
	defer l.Close()

	ch1, cancel1 := l.Subscribe()
	defer cancel1()
	ch2, cancel2 := l.Subscribe()
	defer cancel2()

	l.Append(Event{Action: "screenshot", Outcome: OutcomeSuccess})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Action != "screenshot" {
				t.Errorf("subscriber %d got %q, want screenshot", i, got.Action)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

// TestAppendNeverBlocksOnStalledSubscriber is the reason this design exists: a
// wedged user interface must never be able to stall an administrator action.
func TestAppendNeverBlocksOnStalledSubscriber(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "activity.jsonl"))
	defer l.Close()

	// Subscribe and deliberately never read from the channel.
	_, cancel := l.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more than the subscriber buffer: this must not block.
		for i := 0; i < subscriberBuffer*3; i++ {
			if err := l.Append(Event{Action: "exec_command", Outcome: OutcomeSuccess}); err != nil {
				t.Errorf("Append(%d) error = %v", i, err)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Append blocked on a stalled subscriber")
	}

	if l.Dropped() == 0 {
		t.Error("Dropped() = 0, want > 0 after overflowing a stalled subscriber")
	}

	// Persistence is the source of truth: every event must still be on disk.
	got, err := l.Recent(subscriberBuffer * 3)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(got) != subscriberBuffer*3 {
		t.Errorf("Recent() returned %d events, want %d — drops must not lose persisted events",
			len(got), subscriberBuffer*3)
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "activity.jsonl"))
	defer l.Close()

	ch, cancel := l.Subscribe()
	cancel()

	l.Append(Event{Action: "screenshot", Outcome: OutcomeSuccess})

	// After cancel the channel is closed and yields no further events.
	if _, ok := <-ch; ok {
		t.Error("received an event after cancel(), want closed channel")
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/activitylog/ -run Subscribe -v`
Expected: FALLA con `l.Subscribe undefined`.

- [ ] **Step 3: Añadir la difusión a la implementación**

En `agent/internal/activitylog/activitylog.go`, añadir a los imports `"sync/atomic"`.

Añadir la constante junto a `defaultMaxBytes`:

```go
// subscriberBuffer is how many events a subscriber may fall behind before its
// events start being dropped.
const subscriberBuffer = 64
```

Añadir campos a `Log`:

```go
	subMu   sync.RWMutex
	subs    map[int]chan Event
	nextSub int
	dropped atomic.Uint64
```

En `Open`, antes del `return l, nil`, inicializar el mapa:

```go
	l.subs = make(map[int]chan Event)
```

Añadir al final de `Append`, **después** de escribir a disco y antes del `return nil`, sustituyendo el `return nil` final por:

```go
	// Persist first, broadcast second: a subscriber that cannot keep up loses
	// live notifications, never the record.
	l.broadcast(e)
	return nil
}

// broadcast delivers e to every subscriber without ever blocking. A full
// buffer means that subscriber's process is wedged; dropping is correct because
// blocking here would stall the administrator action that produced the event.
func (l *Log) broadcast(e Event) {
	l.subMu.RLock()
	defer l.subMu.RUnlock()

	for _, ch := range l.subs {
		select {
		case ch <- e:
		default:
			l.dropped.Add(1)
		}
	}
}

// Subscribe returns a channel of future events and a function that cancels the
// subscription and closes the channel.
func (l *Log) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)

	l.subMu.Lock()
	id := l.nextSub
	l.nextSub++
	l.subs[id] = ch
	l.subMu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			l.subMu.Lock()
			delete(l.subs, id)
			l.subMu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Dropped returns how many live notifications were discarded because a
// subscriber was not consuming. The interface surfaces this so the user knows
// the live view lagged while the record stayed complete.
func (l *Log) Dropped() uint64 {
	return l.dropped.Load()
}
```

Nota sobre el orden de bloqueos: `broadcast` toma `subMu` mientras `Append` mantiene `mu`. Ninguna otra ruta toma esos dos en orden inverso, así que no hay riesgo de interbloqueo. `Subscribe` y `cancel` solo toman `subMu`.

- [ ] **Step 4: Correr los tests, incluido el detector de carreras**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/activitylog/ -v`
Expected: PASS los ocho tests.

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/activitylog/ -race`
Expected: PASS sin advertencias de carrera.

- [ ] **Step 5: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/activitylog && git commit -m "feat(agent): broadcast activity events without ever blocking Append"
```

---

### Task 4: Registrar las acciones del administrador

**Files:**
- Modify: `agent/internal/server/server.go:37-61` (structs `Server` y `Client`), `:64-81` (`NewServer`), `:144-174` (`handleWebSocket`)
- Modify: `agent/internal/server/handlers.go:16-50` (`handleMessage`)
- Modify: `agent/cmd/lan-agent/main.go:79-97` (`run`)
- Create: `agent/internal/server/activity.go`
- Test: `agent/internal/server/activity_test.go`

**Interfaces:**
- Consumes: `activitylog.Log`, `activitylog.Event`, `OutcomeSuccess`/`OutcomeError` de las Tareas 2-3.
- Produces:
  - `func (s *Server) SetActivityLog(l *activitylog.Log)` — inyecta el registro; con `nil` el servidor no registra.
  - `func describeAction(msgType string) string` — texto en español para el campo `Detail`.
  - `Client.remoteAddr string` — IP del administrador, usada como `Actor`.

- [ ] **Step 1: Escribir el test que falla**

Contenido de `agent/internal/server/activity_test.go`:

```go
package server

import (
	"testing"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

func TestDescribeActionCoversEveryHandledType(t *testing.T) {
	// Every message type dispatched in handleMessage must have user-facing
	// Spanish text, or the activity log would show raw protocol names.
	handled := []string{
		protocol.MsgExecCommand,
		protocol.MsgListDir,
		protocol.MsgGetFile,
		protocol.MsgSendFile,
		protocol.MsgScreenshot,
		protocol.MsgSystemInfo,
	}

	for _, msgType := range handled {
		got := describeAction(msgType)
		if got == "" {
			t.Errorf("describeAction(%q) = empty, want Spanish description", msgType)
		}
		if got == msgType {
			t.Errorf("describeAction(%q) returned the raw type, want a description", msgType)
		}
	}
}

func TestDescribeActionUsesSpecWording(t *testing.T) {
	// Wording is fixed by the spec so notifications and the log agree.
	cases := map[string]string{
		protocol.MsgScreenshot:  "Se capturó la pantalla de este equipo",
		protocol.MsgExecCommand: "Se ejecutó un comando en este equipo",
		protocol.MsgListDir:     "Se consultaron archivos de este equipo",
		protocol.MsgGetFile:     "Se consultaron archivos de este equipo",
		protocol.MsgSendFile:    "Se copió un archivo a este equipo",
	}
	for msgType, want := range cases {
		if got := describeAction(msgType); got != want {
			t.Errorf("describeAction(%q) = %q, want %q", msgType, got, want)
		}
	}
}

func TestDescribeActionUnknownTypeIsNotEmpty(t *testing.T) {
	if got := describeAction("something_new"); got == "" {
		t.Error("describeAction() on an unknown type returned empty; the log must never show a blank row")
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/server/ -v`
Expected: FALLA con `undefined: describeAction`.

- [ ] **Step 3: Escribir el puente de registro**

Contenido de `agent/internal/server/activity.go`:

```go
package server

import (
	"time"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

// SetActivityLog attaches the local activity log. When nil, the server records
// nothing, which keeps the log optional for tests and foreground runs.
func (s *Server) SetActivityLog(l *activitylog.Log) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activity = l
}

// describeAction returns the user-facing Spanish text for a protocol message
// type. Wording is fixed by the spec so the log and the notifications match.
func describeAction(msgType string) string {
	switch msgType {
	case protocol.MsgExecCommand:
		return "Se ejecutó un comando en este equipo"
	case protocol.MsgListDir, protocol.MsgGetFile:
		return "Se consultaron archivos de este equipo"
	case protocol.MsgSendFile:
		return "Se copió un archivo a este equipo"
	case protocol.MsgScreenshot:
		return "Se capturó la pantalla de este equipo"
	case protocol.MsgSystemInfo:
		return "Se consultó el estado de este equipo"
	default:
		return "Se realizó una acción de soporte en este equipo"
	}
}

// recordActivity persists one administrator action. Failures are logged by the
// activity log itself and never propagate: a broken audit trail must not stop
// the machine from being administered.
func (c *Client) recordActivity(msgType string, outcome string) {
	c.server.mu.RLock()
	l := c.server.activity
	c.server.mu.RUnlock()

	if l == nil {
		return
	}

	_ = l.Append(activitylog.Event{
		Timestamp: time.Now(),
		Action:    msgType,
		Actor:     c.remoteAddr,
		Detail:    describeAction(msgType),
		Outcome:   outcome,
	})
}
```

- [ ] **Step 4: Añadir los campos necesarios**

En `agent/internal/server/server.go`, añadir a los imports:

```go
	"github.com/mediacode/lan-commander/agent/internal/activitylog"
```

Añadir a la struct `Server` (tras el campo `monitor`):

```go
	activity  *activitylog.Log
```

Añadir a la struct `Client` (tras el campo `id`):

```go
	remoteAddr string
```

En `handleWebSocket`, al construir el `Client`, capturar la dirección del administrador:

```go
	client := &Client{
		conn:       conn,
		server:     s,
		send:       make(chan []byte, 64),
		id:         uuid.New().String(),
		remoteAddr: r.RemoteAddr,
	}
```

- [ ] **Step 5: Registrar cada acción atendida**

En `agent/internal/server/handlers.go`, sustituir el `switch` de `handleMessage` (líneas 34-49) por:

```go
	switch msg.Type {
	case protocol.MsgExecCommand:
		c.handleExecCommand(msg)
	case protocol.MsgListDir:
		c.handleListDir(msg)
	case protocol.MsgGetFile:
		c.handleGetFile(msg)
	case protocol.MsgSendFile:
		c.handleSendFile(msg)
	case protocol.MsgScreenshot:
		c.handleScreenshot(msg)
	case protocol.MsgSystemInfo:
		c.handleSystemInfo(msg)
	default:
		c.sendError(msg.ID, fmt.Sprintf("unknown message type: %s", msg.Type))
		return
	}

	// Record after handling so the log reflects what actually ran. The user of
	// this machine must be able to see every administrator action.
	c.recordActivity(msg.Type, activitylog.OutcomeSuccess)
```

Añadir a los imports de `handlers.go`:

```go
	"github.com/mediacode/lan-commander/agent/internal/activitylog"
```

Nota: en Fase 1 el resultado se registra como `OutcomeSuccess` porque los handlers actuales no devuelven su estado a `handleMessage`. Registrar el fallo real requiere cambiar sus firmas; queda fuera de alcance y se anota como deuda al final de esta tarea.

- [ ] **Step 6: Abrir el registro al arrancar el servicio**

En `agent/cmd/lan-agent/main.go`, añadir a los imports:

```go
	"path/filepath"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
	"github.com/mediacode/lan-commander/agent/internal/paths"
```

Tras la línea `srv := server.NewServer(addr, f.tlsCert, f.tlsKey, f.authToken)`, añadir:

```go
	// The activity log is what makes administration visible to this machine's
	// user. If it cannot be opened the agent still runs: losing the log is bad,
	// losing remote management is worse.
	//
	// Declared outside the branch because Task 6 attaches the local channel to
	// the same log.
	var actLog *activitylog.Log
	if l, err := activitylog.Open(filepath.Join(paths.DataDir(), "activity.jsonl")); err != nil {
		log.Printf("[main] Cannot open activity log: %v", err)
	} else {
		actLog = l
		srv.SetActivityLog(actLog)
		defer actLog.Close()
	}
```

- [ ] **Step 7: Crear el paquete de rutas**

Contenido de `agent/internal/paths/paths.go`:

```go
// Package paths resolves the machine-wide directory where the agent stores data
// the local user may read only through the agent, never directly.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir returns the machine-wide data directory: %ProgramData%\LAN Commander
// on Windows, /var/lib/lan-commander elsewhere. It is deliberately not the
// executable's directory, which is not writable under Program Files.
func DataDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "LAN Commander")
	}
	return "/var/lib/lan-commander"
}
```

Contenido de `agent/internal/paths/paths_test.go`:

```go
package paths

import (
	"runtime"
	"strings"
	"testing"
)

func TestDataDirIsAbsolute(t *testing.T) {
	got := DataDir()
	if got == "" {
		t.Fatal("DataDir() = empty")
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(got, "LAN Commander") {
			t.Errorf("DataDir() = %q, want it to contain \"LAN Commander\"", got)
		}
		return
	}
	if !strings.HasPrefix(got, "/") {
		t.Errorf("DataDir() = %q, want an absolute path", got)
	}
}

func TestDataDirFallsBackWhenProgramDataUnset(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ProgramData only applies to Windows")
	}
	t.Setenv("ProgramData", "")
	if got := DataDir(); got == "" || strings.HasPrefix(got, string('\\')) {
		t.Errorf("DataDir() = %q with ProgramData unset, want a usable absolute fallback", got)
	}
}
```

- [ ] **Step 8: Correr toda la suite**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./... -v`
Expected: PASS en `activitylog`, `paths`, `server`, `ui`.

Run: `cd C:/Proyectos/lan-commander/agent && go build ./... && go vet ./...`
Expected: sin salida.

- [ ] **Step 9: Verificar de extremo a extremo, a mano**

Run: `cd C:/Proyectos/lan-commander/agent && go run ./cmd/lan-agent --port 19474 --discovery=false`

En otra terminal, conectar con el control-center (o cualquier cliente WebSocket) a `ws://127.0.0.1:19474/ws` y ejecutar un comando. Luego:

Run: `type "%ProgramData%\LAN Commander\activity.jsonl"`
Expected: una línea JSON con `"action":"exec_command"`, el `actor` con la IP del cliente, y `"detail":"Se ejecutó un comando en este equipo"`.

- [ ] **Step 10: Anotar la deuda conocida**

Añadir a la sección "Fases de entrega" de la spec, bajo Fase 1:

```markdown
**Deuda asumida en Fase 1:** `recordActivity` registra siempre `success` porque los
handlers de `agent/internal/server/handlers.go` no devuelven su resultado a
`handleMessage`. Registrar el fallo real exige cambiar sus firmas. Pendiente para
Fase 2, donde ya se tocan esos handlers.
```

- [ ] **Step 11: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/server agent/internal/paths agent/cmd/lan-agent/main.go docs/superpowers/specs/ && git commit -m "feat(agent): record administrator actions to the local activity log"
```

---

### Task 5: Canal local — códec y operaciones de lectura

**Files:**
- Create: `agent/internal/localapi/protocol.go`
- Create: `agent/internal/localapi/server.go`
- Test: `agent/internal/localapi/server_test.go`

**Interfaces:**
- Consumes: `activitylog.Log`, `activitylog.Event`.
- Produces:
  - `type Request struct { Op string; Limit int }`
  - `type Response struct { Op string; Events []activitylog.Event; Dropped uint64; Error string }`
  - `const OpGetActivityLog = "GetActivityLog"`, `OpSubscribeActivity = "SubscribeActivity"`
  - `func NewServer(l *activitylog.Log) *Server`
  - `func (s *Server) Serve(ctx context.Context, ln net.Listener) error`
  - `func (s *Server) handleConn(ctx context.Context, conn net.Conn)`

- [ ] **Step 1: Escribir el test que falla**

Contenido de `agent/internal/localapi/server_test.go`:

```go
package localapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
)

// newTestServer starts a localapi server on an in-process pipe pair, avoiding
// any dependency on OS-specific socket paths or privileges.
func newTestServer(t *testing.T) (*activitylog.Log, net.Conn) {
	t.Helper()

	l, err := activitylog.Open(filepath.Join(t.TempDir(), "activity.jsonl"))
	if err != nil {
		t.Fatalf("activitylog.Open() error = %v", err)
	}
	t.Cleanup(func() { l.Close() })

	srvConn, cliConn := net.Pipe()
	t.Cleanup(func() { srvConn.Close(); cliConn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := NewServer(l)
	go s.handleConn(ctx, srvConn)

	return l, cliConn
}

func TestGetActivityLogReturnsEvents(t *testing.T) {
	l, conn := newTestServer(t)

	l.Append(activitylog.Event{Action: "screenshot", Actor: "192.168.1.10", Outcome: activitylog.OutcomeSuccess})

	writeReq(t, conn, Request{Op: OpGetActivityLog, Limit: 10})
	resp := readResp(t, conn)

	if resp.Error != "" {
		t.Fatalf("response error = %q, want empty", resp.Error)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("returned %d events, want 1", len(resp.Events))
	}
	if resp.Events[0].Action != "screenshot" {
		t.Errorf("event action = %q, want screenshot", resp.Events[0].Action)
	}
}

func TestUnknownOperationIsRejected(t *testing.T) {
	_, conn := newTestServer(t)

	// The channel exposes a closed set of operations. Anything else must be
	// refused rather than guessed at.
	writeReq(t, conn, Request{Op: "RunCommand", Limit: 1})
	resp := readResp(t, conn)

	if resp.Error == "" {
		t.Fatal("unknown operation accepted, want an error")
	}
	if len(resp.Events) != 0 {
		t.Errorf("unknown operation returned %d events, want 0", len(resp.Events))
	}
}

func TestSubscribeActivityStreamsNewEvents(t *testing.T) {
	l, conn := newTestServer(t)

	writeReq(t, conn, Request{Op: OpSubscribeActivity})

	// Give the server a moment to register the subscription before appending.
	time.Sleep(100 * time.Millisecond)
	l.Append(activitylog.Event{Action: "exec_command", Outcome: activitylog.OutcomeSuccess})

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp := readResp(t, conn)

	if len(resp.Events) != 1 || resp.Events[0].Action != "exec_command" {
		t.Fatalf("streamed response = %+v, want one exec_command event", resp.Events)
	}
}

func TestOversizedRequestIsRejected(t *testing.T) {
	_, conn := newTestServer(t)

	huge := make([]byte, maxRequestBytes+1024)
	for i := range huge {
		huge[i] = 'a'
	}
	conn.Write(append(huge, '\n'))

	// The server must not hang or panic; it closes or errors on the connection.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Error("oversized request produced a normal response, want error or closed connection")
	}
}

func writeReq(t *testing.T, conn net.Conn, req Request) {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("cannot encode request: %v", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		t.Fatalf("cannot write request: %v", err)
	}
}

func readResp(t *testing.T, conn net.Conn) Response {
	t.Helper()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatalf("cannot read response: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("cannot decode response %q: %v", line, err)
	}
	return resp
}
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/localapi/ -v`
Expected: FALLA con `undefined: NewServer`.

- [ ] **Step 3: Escribir el códec**

Contenido de `agent/internal/localapi/protocol.go`:

```go
// Package localapi exposes a deliberately narrow channel from the privileged
// agent service to the unprivileged interface running in the user's session.
//
// The security property that matters: no command string, path or argument ever
// crosses this channel. Phase 1 exposes two read-only operations. Any future
// operation that accepts free text would break that property.
package localapi

import "github.com/mediacode/lan-commander/agent/internal/activitylog"

// Operations. This set is closed; unknown values are refused.
const (
	OpGetActivityLog    = "GetActivityLog"
	OpSubscribeActivity = "SubscribeActivity"
)

// maxRequestBytes caps a single request line.
const maxRequestBytes = 64 * 1024

// defaultLimit is used when a request omits Limit.
const defaultLimit = 100

// maxLimit caps how many events one request may ask for.
const maxLimit = 1000

// Request is one line of JSON from the interface.
type Request struct {
	Op    string `json:"op"`
	Limit int    `json:"limit,omitempty"`
}

// Response is one line of JSON back to the interface. Error is set when the
// request was refused; Events is then empty.
type Response struct {
	Op      string               `json:"op"`
	Events  []activitylog.Event  `json:"events,omitempty"`
	Dropped uint64               `json:"dropped,omitempty"`
	Error   string               `json:"error,omitempty"`
}
```

- [ ] **Step 4: Escribir el servidor**

Contenido de `agent/internal/localapi/server.go`:

```go
package localapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
)

// Server answers requests from interface processes. It must accept several
// clients at once: one per active user session, since fast user switching and
// RDP mean more than one desktop can be live on the same machine.
type Server struct {
	activity *activitylog.Log
}

// NewServer creates a server backed by the given activity log.
func NewServer(l *activitylog.Log) *Server {
	return &Server{activity: l}
}

// Serve accepts connections until ctx is cancelled or the listener fails.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			return fmt.Errorf("local api accept failed: %w", err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

// handleConn serves one interface process for the life of its connection.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReaderSize(conn, maxRequestBytes)
	encoder := json.NewEncoder(conn)
	var writeMu sync.Mutex

	send := func(resp Response) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return encoder.Encode(resp)
	}

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return // client closed, or the line exceeded the buffer
		}
		if len(line) > maxRequestBytes {
			return
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = send(Response{Error: "petición mal formada"})
			continue
		}

		switch req.Op {
		case OpGetActivityLog:
			s.serveGetActivityLog(req, send)
		case OpSubscribeActivity:
			// Runs until the connection or context ends.
			s.serveSubscribe(ctx, send)
			return
		default:
			_ = send(Response{Op: req.Op, Error: fmt.Sprintf("operación no permitida: %q", req.Op)})
		}
	}
}

func (s *Server) serveGetActivityLog(req Request, send func(Response) error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	events, err := s.activity.Recent(limit)
	if err != nil {
		log.Printf("[localapi] Cannot read activity log: %v", err)
		_ = send(Response{Op: req.Op, Error: "no se pudo leer el registro de actividad"})
		return
	}

	_ = send(Response{Op: req.Op, Events: events, Dropped: s.activity.Dropped()})
}

func (s *Server) serveSubscribe(ctx context.Context, send func(Response) error) {
	ch, cancel := s.activity.Subscribe()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if err := send(Response{Op: OpSubscribeActivity, Events: []activitylog.Event{e}}); err != nil {
				return // interface went away
			}
		}
	}
}
```

- [ ] **Step 5: Correr los tests**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/localapi/ -v`
Expected: PASS los cuatro tests.

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/localapi/ -race`
Expected: PASS sin advertencias.

- [ ] **Step 6: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/localapi && git commit -m "feat(agent): add local API with closed operation set"
```

---

### Task 6: Transporte del canal por plataforma

**Files:**
- Create: `agent/internal/localapi/listen_windows.go`
- Create: `agent/internal/localapi/listen_unix.go`
- Create: `agent/internal/localapi/listen_test.go`
- Modify: `agent/go.mod` (añade `github.com/Microsoft/go-winio`)
- Modify: `agent/cmd/lan-agent/main.go`

**Interfaces:**
- Consumes: `Server.Serve` de la Tarea 5.
- Produces:
  - `func Listen() (net.Listener, error)` — abre el named pipe con ACL en Windows, o el socket Unix con permisos en Linux.
  - `func Dial(ctx context.Context) (net.Conn, error)` — usado por el proceso de interfaz en la Tarea 7.
  - `const pipeName = ` + "`" + `\\.\pipe\lan-commander-agent` + "`"

- [ ] **Step 1: Añadir la dependencia**

```bash
cd C:/Proyectos/lan-commander/agent && go get github.com/Microsoft/go-winio@latest
```

- [ ] **Step 2: Escribir el test que falla**

Contenido de `agent/internal/localapi/listen_test.go`:

```go
package localapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
)

// TestListenAndDialRoundTrip exercises the real platform transport: named pipe
// on Windows, Unix socket elsewhere.
func TestListenAndDialRoundTrip(t *testing.T) {
	ln, err := Listen()
	if err != nil {
		t.Skipf("cannot open local transport in this environment: %v", err)
	}
	defer ln.Close()

	l, err := activitylog.Open(filepath.Join(t.TempDir(), "activity.jsonl"))
	if err != nil {
		t.Fatalf("activitylog.Open() error = %v", err)
	}
	defer l.Close()
	l.Append(activitylog.Event{Action: "screenshot", Outcome: activitylog.OutcomeSuccess})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go NewServer(l).Serve(ctx, ln)

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()

	conn, err := Dial(dialCtx)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	writeReq(t, conn, Request{Op: OpGetActivityLog, Limit: 10})
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp := readResp(t, conn)

	if len(resp.Events) != 1 || resp.Events[0].Action != "screenshot" {
		t.Fatalf("round trip returned %+v, want one screenshot event", resp.Events)
	}
}

// TestTwoClientsAtOnce guards the multi-session requirement: with RDP or fast
// user switching, a single-client channel would silently leave the second user
// without an interface.
func TestTwoClientsAtOnce(t *testing.T) {
	ln, err := Listen()
	if err != nil {
		t.Skipf("cannot open local transport in this environment: %v", err)
	}
	defer ln.Close()

	l, _ := activitylog.Open(filepath.Join(t.TempDir(), "activity.jsonl"))
	defer l.Close()
	l.Append(activitylog.Event{Action: "exec_command", Outcome: activitylog.OutcomeSuccess})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go NewServer(l).Serve(ctx, ln)

	for i := 0; i < 2; i++ {
		dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := Dial(dialCtx)
		if err != nil {
			dialCancel()
			t.Fatalf("client %d: Dial() error = %v", i, err)
		}
		defer conn.Close()
		defer dialCancel()

		data, _ := json.Marshal(Request{Op: OpGetActivityLog, Limit: 1})
		conn.Write(append(data, '\n'))
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		resp := readResp(t, conn)
		if len(resp.Events) != 1 {
			t.Errorf("client %d received %d events, want 1", i, len(resp.Events))
		}
	}
}
```

- [ ] **Step 3: Escribir el transporte de Windows**

Contenido de `agent/internal/localapi/listen_windows.go`:

```go
//go:build windows

package localapi

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

// pipeName is the machine-local channel between service and interface.
const pipeName = `\\.\pipe\lan-commander-agent`

// pipeSDDL grants full control to Local System and Administrators, and
// read/write to interactive users, who are the ones running the interface.
// Interactive users deliberately get no more than that: the channel's safety
// rests on its closed operation set, not on who may open it.
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)"

// Listen opens the named pipe. Multiple instances are allowed so one interface
// process per user session can connect at the same time.
func Listen() (net.Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: pipeSDDL,
		MessageMode:        false,
		InputBufferSize:    maxRequestBytes,
		OutputBufferSize:   maxRequestBytes,
	}
	ln, err := winio.ListenPipe(pipeName, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", pipeName, err)
	}
	return ln, nil
}

// Dial connects to the service's pipe from an interface process.
func Dial(ctx context.Context) (net.Conn, error) {
	conn, err := winio.DialPipeContext(ctx, pipeName)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the agent service: %w", err)
	}
	return conn, nil
}
```

- [ ] **Step 4: Escribir el transporte de Unix**

Contenido de `agent/internal/localapi/listen_unix.go`:

```go
//go:build !windows

package localapi

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// socketDir is volatile: it does not survive a reboot, so the service creates
// it on every start rather than assuming the installer left it there.
const socketDir = "/run/lan-commander"

const socketPath = socketDir + "/agent.sock"

// Listen creates the Unix socket. Mode 0666 lets any local desktop session
// connect; the channel's safety rests on its closed operation set.
func Listen() (net.Listener, error) {
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", socketDir, err)
	}

	// A stale socket from an unclean shutdown would block binding.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o666); err != nil {
		ln.Close()
		return nil, fmt.Errorf("cannot set socket permissions: %w", err)
	}
	return ln, nil
}

// Dial connects to the service's socket from an interface process.
func Dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", filepath.Clean(socketPath))
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the agent service: %w", err)
	}
	return conn, nil
}
```

- [ ] **Step 5: Correr los tests**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/localapi/ -v`
Expected: PASS. En Windows sin privilegios especiales el pipe debe abrirse sin problema; si `Listen()` falla los dos tests nuevos se saltan con un mensaje, y eso es una señal a investigar, no un éxito.

- [ ] **Step 6: Arrancar el canal desde el servicio**

En `agent/cmd/lan-agent/main.go`, tras el bloque que abre el registro de actividad, añadir:

```go
	// The local channel is what lets this machine's user see the activity log.
	// Its failure must not stop remote management.
	if actLog != nil {
		if ln, err := localapi.Listen(); err != nil {
			log.Printf("[main] Cannot open local channel: %v", err)
		} else {
			go func() {
				if err := localapi.NewServer(actLog).Serve(ctx, ln); err != nil {
					log.Printf("[main] Local channel stopped: %v", err)
				}
			}()
		}
	}
```

Añadir a los imports `"github.com/mediacode/lan-commander/agent/internal/localapi"`.

`actLog` ya está declarado con el alcance correcto por la Tarea 4, así que este
bloque solo se añade después.

El `ctx` que se pasa a `Serve` es el de `p.run(ctx)`, que el gestor de servicios
cancela al detener el agente: eso cierra el listener y termina las goroutines de
atención.

- [ ] **Step 7: Verificar que compila y que el servicio arranca**

Run: `cd C:/Proyectos/lan-commander/agent && go build ./... && go vet ./...`
Expected: sin salida.

Run: `cd C:/Proyectos/lan-commander/agent && go run ./cmd/lan-agent --port 19474 --discovery=false`
Expected: arranca sin errores de canal local. Cortar con Ctrl+C.

- [ ] **Step 8: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/localapi agent/cmd/lan-agent/main.go agent/go.mod agent/go.sum && git commit -m "feat(agent): serve the local channel over named pipe and unix socket"
```

---

### Task 7: Proceso de interfaz — bandeja, Estado y Actividad

**Files:**
- Create: `agent/internal/ui/client.go`
- Create: `agent/internal/ui/app.go`
- Create: `agent/internal/ui/text.go`
- Test: `agent/internal/ui/client_test.go`
- Test: `agent/internal/ui/text_test.go`
- Modify: `agent/cmd/lan-agent/main.go`

**Interfaces:**
- Consumes: `localapi.Dial`, `localapi.Request`, `localapi.Response`, `OpGetActivityLog`, `OpSubscribeActivity`, `activitylog.Event`, `ui.Available`.
- Produces:
  - `type Client struct` con `func NewClient() *Client`, `func (c *Client) ActivityLog(ctx context.Context, limit int) ([]activitylog.Event, error)`, `func (c *Client) Subscribe(ctx context.Context) (<-chan activitylog.Event, error)`
  - `func Run(ctx context.Context, cfg Config) error` — arranca la interfaz.
  - `type Config struct { ManagedByNotice string; AgentVersion string }`
  - `func actionText(action string) string`, `func trayTooltip(state ConnState) string`
  - `type ConnState int` con `StateDisconnected`, `StateIdle`, `StateSupportActive`

- [ ] **Step 1: Escribir los tests de texto**

Contenido de `agent/internal/ui/text_test.go`:

```go
package ui

import "testing"

func TestTrayTooltipUsesSpecWording(t *testing.T) {
	cases := map[ConnState]string{
		StateDisconnected:  "LAN Commander — sin conexión con el servicio",
		StateIdle:          "LAN Commander — activo",
		StateSupportActive: "LAN Commander — sesión de soporte en curso",
	}
	for state, want := range cases {
		if got := trayTooltip(state); got != want {
			t.Errorf("trayTooltip(%v) = %q, want %q", state, got, want)
		}
	}
}

func TestActionTextUsesSpecWording(t *testing.T) {
	cases := map[string]string{
		"screenshot":   "Se capturó la pantalla de este equipo",
		"exec_command": "Se ejecutó un comando en este equipo",
		"list_dir":     "Se consultaron archivos de este equipo",
		"get_file":     "Se consultaron archivos de este equipo",
		"send_file":    "Se copió un archivo a este equipo",
	}
	for action, want := range cases {
		if got := actionText(action); got != want {
			t.Errorf("actionText(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestActionTextNeverEmpty(t *testing.T) {
	// A blank row in the activity list would defeat the whole point.
	if got := actionText("future_action"); got == "" {
		t.Error("actionText() on an unknown action returned empty")
	}
}

func TestEmptyStateAndDegradedTextsExist(t *testing.T) {
	if emptyActivityText == "" {
		t.Error("emptyActivityText is empty")
	}
	if degradedTitle == "" || degradedBody == "" {
		t.Error("degraded mode text is incomplete")
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/ui/ -run Text -v`
Expected: FALLA con `undefined: trayTooltip`.

- [ ] **Step 3: Escribir los textos**

Contenido de `agent/internal/ui/text.go`:

```go
package ui

// Every string here is fixed by the spec's "Textos de la interfaz" section.
// Terminology is deliberate: "personal de sistemas", "sesión de soporte",
// "registro de actividad". Do not paraphrase — the log and the notifications
// have to agree word for word.

// ConnState is the interface's view of the service connection.
type ConnState int

const (
	// StateDisconnected means the local channel is unreachable.
	StateDisconnected ConnState = iota
	// StateIdle means the service is running with no administrator connected.
	StateIdle
	// StateSupportActive means an administrator is connected right now.
	StateSupportActive
)

const (
	emptyActivityText = "Sin actividad todavía. Cuando el personal de sistemas haga algo en este equipo, aparecerá aquí con la fecha y la hora."

	degradedTitle = "Sin conexión con el servicio del agente"
	degradedBody  = "El diagnóstico y las pruebas de red siguen disponibles. El registro de actividad, los scripts y las solicitudes de ayuda no lo están hasta que se restablezca la conexión. Reintentando automáticamente."

	droppedNoticeText = "Se perdieron avisos en vivo; el registro está completo."

	noticeAcknowledge = "Entendido"
	noticeDisclaimer  = "Este aviso te informa de cómo funciona el equipo; no te pide permiso."
	noticeBody        = "El personal de sistemas puede ejecutar programas, consultar archivos y capturar la pantalla de este equipo como parte del soporte técnico.\n\nCada acción queda registrada. Puedes consultar el registro completo cuando quieras desde el ícono de LAN Commander en la barra de tareas."

	notificationSource = "Soporte técnico"
	viewLogAction      = "Ver registro"
)

// trayTooltip returns the tray tooltip for a connection state.
func trayTooltip(state ConnState) string {
	switch state {
	case StateSupportActive:
		return "LAN Commander — sesión de soporte en curso"
	case StateIdle:
		return "LAN Commander — activo"
	default:
		return "LAN Commander — sin conexión con el servicio"
	}
}

// actionText returns the user-facing description of a recorded action. It must
// never return empty: a blank row in the activity list defeats the purpose.
func actionText(action string) string {
	switch action {
	case "exec_command":
		return "Se ejecutó un comando en este equipo"
	case "list_dir", "get_file":
		return "Se consultaron archivos de este equipo"
	case "send_file":
		return "Se copió un archivo a este equipo"
	case "screenshot":
		return "Se capturó la pantalla de este equipo"
	case "system_info":
		return "Se consultó el estado de este equipo"
	default:
		return "Se realizó una acción de soporte en este equipo"
	}
}
```

- [ ] **Step 4: Correr los tests de texto**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/ui/ -run Text -v`
Expected: PASS los cuatro tests.

- [ ] **Step 5: Escribir el test del cliente del canal**

Contenido de `agent/internal/ui/client_test.go`:

```go
package ui

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
	"github.com/mediacode/lan-commander/agent/internal/localapi"
)

// fakeDial serves one localapi server over an in-process pipe so the client can
// be tested without any OS transport.
func fakeDial(t *testing.T, l *activitylog.Log) dialFunc {
	t.Helper()
	return func(ctx context.Context) (net.Conn, error) {
		srvConn, cliConn := net.Pipe()
		t.Cleanup(func() { srvConn.Close(); cliConn.Close() })
		go localapi.NewServer(l).HandleConn(ctx, srvConn)
		return cliConn, nil
	}
}

func TestActivityLogReadsThroughChannel(t *testing.T) {
	l, err := activitylog.Open(t.TempDir() + "/activity.jsonl")
	if err != nil {
		t.Fatalf("activitylog.Open() error = %v", err)
	}
	defer l.Close()
	l.Append(activitylog.Event{Action: "screenshot", Outcome: activitylog.OutcomeSuccess})

	c := &Client{dial: fakeDial(t, l)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := c.ActivityLog(ctx, 10)
	if err != nil {
		t.Fatalf("ActivityLog() error = %v", err)
	}
	if len(got) != 1 || got[0].Action != "screenshot" {
		t.Fatalf("ActivityLog() = %+v, want one screenshot event", got)
	}
}

func TestActivityLogReportsDegradedWhenServiceAbsent(t *testing.T) {
	c := &Client{dial: func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("no service")
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := c.ActivityLog(ctx, 10); err == nil {
		t.Fatal("ActivityLog() succeeded with no service, want an error so the UI can show degraded mode")
	}
}
```

El test de arriba usa `localapi.NewServer(l).HandleConn(...)`, que aún no existe:
`handleConn` es privado y un archivo `export_test.go` solo aplica a los tests del
propio paquete, no a los de `ui`. Hay que exportar un envoltorio en un archivo
normal. Crear `agent/internal/localapi/handleconn.go`:

```go
package localapi

import (
	"context"
	"net"
)

// HandleConn serves a single already-accepted connection. Exported so the
// interface package can exercise its client against a real server over an
// in-process pipe, without depending on the OS transport.
func (s *Server) HandleConn(ctx context.Context, conn net.Conn) {
	s.handleConn(ctx, conn)
}
```

- [ ] **Step 6: Escribir el cliente**

Contenido de `agent/internal/ui/client.go`:

```go
package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
	"github.com/mediacode/lan-commander/agent/internal/localapi"
)

// dialFunc opens a connection to the service. Injectable so tests can run over
// an in-process pipe instead of the OS transport.
type dialFunc func(ctx context.Context) (net.Conn, error)

// Client talks to the agent service over the local channel.
type Client struct {
	dial dialFunc

	mu      sync.Mutex
	dropped uint64
}

// Dropped reports how many live notifications the service had to discard
// because this interface was not keeping up. The record stays complete, so the
// interface says so rather than hiding the gap.
func (c *Client) Dropped() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

// NewClient returns a client using the platform transport.
func NewClient() *Client {
	return &Client{dial: localapi.Dial}
}

// ActivityLog fetches the most recent recorded actions. An error means the
// service is unreachable and the interface should show degraded mode.
func (c *Client) ActivityLog(ctx context.Context, limit int) ([]activitylog.Event, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("no hay conexión con el servicio: %w", err)
	}
	defer conn.Close()

	if err := writeRequest(conn, localapi.Request{Op: localapi.OpGetActivityLog, Limit: limit}); err != nil {
		return nil, err
	}

	resp, err := readResponse(conn)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("el servicio rechazó la petición: %s", resp.Error)
	}

	c.mu.Lock()
	c.dropped = resp.Dropped
	c.mu.Unlock()

	return resp.Events, nil
}

// Subscribe streams events as they happen. The channel closes when the
// connection drops, which the caller treats as entering degraded mode.
func (c *Client) Subscribe(ctx context.Context) (<-chan activitylog.Event, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("no hay conexión con el servicio: %w", err)
	}

	if err := writeRequest(conn, localapi.Request{Op: localapi.OpSubscribeActivity}); err != nil {
		conn.Close()
		return nil, err
	}

	out := make(chan activitylog.Event, 16)
	go func() {
		defer close(out)
		defer conn.Close()

		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}
			var resp localapi.Response
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			for _, e := range resp.Events {
				select {
				case out <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func writeRequest(conn net.Conn, req localapi.Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("cannot encode request: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("cannot send request: %w", err)
	}
	return nil
}

func readResponse(conn net.Conn) (localapi.Response, error) {
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return localapi.Response{}, fmt.Errorf("cannot read response: %w", err)
	}
	var resp localapi.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return localapi.Response{}, fmt.Errorf("cannot decode response: %w", err)
	}
	return resp, nil
}
```

- [ ] **Step 7: Correr los tests del cliente**

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/ui/ -v`
Expected: PASS todos.

Run: `cd C:/Proyectos/lan-commander/agent && go test ./internal/ui/ -race`
Expected: PASS sin advertencias.

- [ ] **Step 8: Escribir la interfaz Fyne**

Contenido de `agent/internal/ui/app.go`:

```go
package ui

import (
	"context"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/mediacode/lan-commander/agent/internal/activitylog"
)

// Config carries what the interface needs from the command line.
type Config struct {
	ManagedByNotice string
	AgentVersion    string
}

// reconnectMin and reconnectMax bound the backoff when the service is absent.
const (
	reconnectMin = 1 * time.Second
	reconnectMax = 30 * time.Second
)

// Run starts the client interface. It returns when the window is closed.
func Run(ctx context.Context, cfg Config) error {
	if !Available() {
		return fmt.Errorf("no graphical environment available")
	}

	a := app.NewWithID("com.mediacode.lancommander.agent")
	w := a.NewWindow("LAN Commander")
	w.Resize(fyne.NewSize(640, 480))

	client := NewClient()

	statusLabel := widget.NewLabel(trayTooltip(StateDisconnected))
	activityList := widget.NewLabel(emptyActivityText)
	activityList.Wrapping = fyne.TextWrapWord

	tabs := container.NewAppTabs(
		container.NewTabItem("Estado", container.NewVBox(
			widget.NewLabelWithStyle("Estado del equipo", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			statusLabel,
			widget.NewLabel(fmt.Sprintf("Versión del agente: %s", cfg.AgentVersion)),
		)),
		container.NewTabItem("Actividad", container.NewVScroll(activityList)),
	)
	w.SetContent(tabs)

	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayMenu(fyne.NewMenu("LAN Commander",
			fyne.NewMenuItem("Abrir", func() { w.Show() }),
		))
	}

	// Show the managed-device notice once per user. It informs; it does not ask.
	if cfg.ManagedByNotice != "" && !noticeAlreadyShown(a) {
		showManagedNotice(a, w, cfg.ManagedByNotice)
	}

	go pollActivity(ctx, client, statusLabel, activityList)
	go notifyActivity(ctx, a, client)

	w.SetCloseIntercept(func() { w.Hide() }) // keep living in the tray
	w.ShowAndRun()
	return nil
}

// notifyActivity turns live events into desktop notifications. This is what
// makes "notify always" real: without it the user only learns what happened if
// they happen to open the window.
//
// Bursts are grouped: three or more events inside a minute become one
// notification, because a notification per action is noise, and noise gets
// ignored. The activity log keeps the detail.
func notifyActivity(ctx context.Context, a fyne.App, c *Client) {
	const (
		groupWindow    = time.Minute
		groupThreshold = 3
	)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		events, err := c.Subscribe(ctx)
		if err != nil {
			// Degraded mode is already surfaced by pollActivity; just retry.
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectMax):
			}
			continue
		}

		var burst []activitylog.Event
		flush := time.NewTimer(groupWindow)
		flush.Stop()

		drain := func() {
			if len(burst) == 0 {
				return
			}
			if len(burst) >= groupThreshold {
				a.SendNotification(fyne.NewNotification(
					notificationSource,
					fmt.Sprintf("%d acciones de soporte en este equipo · %s",
						len(burst), time.Now().Format("15:04")),
				))
			} else {
				for _, e := range burst {
					a.SendNotification(fyne.NewNotification(
						notificationSource,
						fmt.Sprintf("%s · %s", actionText(e.Action), e.Timestamp.Local().Format("15:04")),
					))
				}
			}
			burst = nil
		}

	stream:
		for {
			select {
			case <-ctx.Done():
				flush.Stop()
				return
			case e, ok := <-events:
				if !ok {
					flush.Stop()
					drain()
					break stream // connection dropped: reconnect on the outer loop
				}
				if len(burst) == 0 {
					flush.Reset(groupWindow)
				}
				burst = append(burst, e)
			case <-flush.C:
				drain()
			}
		}
	}
}

// noticeAlreadyShown records acknowledgement per user via Fyne preferences,
// which are stored in the user's own profile.
func noticeAlreadyShown(a fyne.App) bool {
	return a.Preferences().Bool("managed_notice_acknowledged")
}

func showManagedNotice(a fyne.App, parent fyne.Window, organisation string) {
	body := widget.NewLabel(noticeBody + "\n\n" + noticeDisclaimer)
	body.Wrapping = fyne.TextWrapWord

	d := widget.NewModalPopUp(container.NewVBox(
		widget.NewLabelWithStyle(
			fmt.Sprintf("Este equipo lo gestiona %s", organisation),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		body,
	), parent.Canvas())

	ack := widget.NewButton(noticeAcknowledge, func() {
		a.Preferences().SetBool("managed_notice_acknowledged", true)
		d.Hide()
	})
	d.Content.(*fyne.Container).Add(ack)
	d.Show()
}

// pollActivity keeps the activity view current, reconnecting with backoff when
// the service is unreachable. Degraded mode is visible, never silent.
func pollActivity(ctx context.Context, c *Client, status *widget.Label, list *widget.Label) {
	backoff := reconnectMin

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		events, err := c.ActivityLog(ctx, 100)
		if err != nil {
			status.SetText(degradedTitle)
			list.SetText(degradedBody)
			log.Printf("[ui] %v", err)

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < reconnectMax {
				backoff *= 2
				if backoff > reconnectMax {
					backoff = reconnectMax
				}
			}
			continue
		}

		backoff = reconnectMin
		status.SetText(trayTooltip(StateIdle))

		rendered := renderEvents(events)
		if c.Dropped() > 0 {
			rendered = droppedNoticeText + "\n\n" + rendered
		}
		list.SetText(rendered)

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// renderEvents formats the activity log for display, newest first.
func renderEvents(events []activitylog.Event) string {
	if len(events) == 0 {
		return emptyActivityText
	}

	out := ""
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		out += fmt.Sprintf("%s  ·  %s\n", e.Timestamp.Local().Format("2006-01-02 15:04"), actionText(e.Action))
	}
	return out
}
```

- [ ] **Step 9: Añadir el flag `--ui`**

En `agent/cmd/lan-agent/main.go`, añadir el campo a `agentFlags`:

```go
	uiMode           bool
	managedByNotice  string
```

Y en `parseAgentFlags`:

```go
	fs.BoolVar(&f.uiMode, "ui", false, "Run the client interface in the current desktop session")
	fs.StringVar(&f.managedByNotice, "managed-by-notice", "", "Organisation name shown in the managed-device notice")
```

En `main()`, antes de construir el servicio, desviar al modo interfaz:

```go
	// --ui runs the client interface in the user's session. It is a separate
	// process from the service: Windows session 0 isolation means a service
	// cannot draw on the user's desktop.
	if f.uiMode {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := ui.Run(ctx, ui.Config{
			ManagedByNotice: f.managedByNotice,
			AgentVersion:    Version,
		}); err != nil {
			log.Fatalf("[main] Interface failed: %v", err)
		}
		return
	}
```

Añadir a los imports `"github.com/mediacode/lan-commander/agent/internal/ui"`.

- [ ] **Step 10: Verificar todo**

Run: `cd C:/Proyectos/lan-commander/agent && go build ./... && go vet ./... && go test ./... -race`
Expected: compila, sin avisos de vet, todos los tests pasan.

- [ ] **Step 11: Verificación manual de la interfaz**

Terminal 1: `cd C:/Proyectos/lan-commander/agent && go run ./cmd/lan-agent --port 19474 --discovery=false`

Terminal 2: `cd C:/Proyectos/lan-commander/agent && go run ./cmd/lan-agent --ui --managed-by-notice "Media Code"`

Comprobar, en este orden:

1. Aparece el ícono en la bandeja del sistema.
2. El aviso de equipo gestionado se muestra una vez, incluye la línea "no te pide
   permiso", y **no vuelve a aparecer** al cerrar y reabrir la interfaz.
3. Al ejecutar un comando desde el control-center contra `127.0.0.1:19474`, llega una
   notificación de escritorio con el texto de `actionText` y la hora.
4. Al ejecutar cinco comandos seguidos en menos de un minuto, llega **una sola**
   notificación agrupada ("5 acciones de soporte en este equipo"), no cinco.
5. La pestaña Actividad lista todas las acciones, la más reciente arriba.
6. Al detener el servicio (Ctrl+C en terminal 1) la interfaz pasa a modo degradado con
   el texto de `degradedTitle` y `degradedBody`, en lugar de quedarse en blanco o
   mostrar una lista vacía engañosa.
7. Al volver a arrancar el servicio, la interfaz se recupera sola sin reiniciarla.

- [ ] **Step 12: Commit**

```bash
cd C:/Proyectos/lan-commander && git add agent/internal/ui agent/internal/localapi agent/cmd/lan-agent/main.go && git commit -m "feat(agent): add client interface with tray, activity view and managed-device notice"
```

---

### Task 8: Instaladores — arranque automático y directorio de datos

**Files:**
- Modify: `installers/windows/install-agent.ps1`
- Modify: `installers/linux/install-agent.sh`

**Interfaces:**
- Consumes: el flag `--ui` y `--managed-by-notice` de la Tarea 7.
- Produces: nada consumido por código.

- [ ] **Step 1: Windows — parámetro de aviso y arranque automático**

En `installers/windows/install-agent.ps1`, añadir al bloque `param(`:

```powershell
    [string]$ManagedByNotice = "",
```

Tras el bloque que copia el ejecutable (`Copy-Item ... -Force`), añadir:

```powershell
# Directorio de datos de la maquina: registro de actividad y scripts. Solo
# SYSTEM y administradores pueden escribir; el usuario lo lee via el agente.
$dataDir = Join-Path $env:ProgramData "LAN Commander"
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null

# Arranque automatico de la interfaz para cualquier usuario que inicie sesion.
# Es un proceso aparte del servicio: en Windows un servicio no puede dibujar
# en el escritorio del usuario (aislamiento de Sesion 0).
$runKey = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run"
$uiCommand = "`"$exeDest`" --ui"
if ($ManagedByNotice -ne "") {
    $uiCommand += " --managed-by-notice `"$ManagedByNotice`""
}
Set-ItemProperty -Path $runKey -Name "LANCommanderUI" -Value $uiCommand
Write-Host "  Interfaz de usuario registrada para arrancar al iniciar sesion" -ForegroundColor Green
```

En el bloque de `$Uninstall`, tras `Remove-NetFirewallRule ...`, añadir:

```powershell
    Remove-ItemProperty -Path "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "LANCommanderUI" -ErrorAction SilentlyContinue
```

Actualizar la cabecera de ayuda del script añadiendo tras la línea de `-AllowFrom`:

```powershell
    Con aviso de equipo gestionado (recomendado, se muestra al usuario):
        .\install-agent.ps1 -ManagedByNotice "Nombre de la organizacion"
```

- [ ] **Step 2: Linux — equivalente**

En `installers/linux/install-agent.sh`, añadir la variable junto a las demás:

```bash
MANAGED_BY_NOTICE=""
```

Y el caso de parseo:

```bash
	--managed-by-notice)
		MANAGED_BY_NOTICE="$2"
		shift 2
		;;
```

Tras `install -m 755 ...`, añadir:

```bash
# Directorio de datos de la maquina: registro de actividad y scripts.
install -d -m 755 /var/lib/lan-commander

# Arranque automatico de la interfaz en sesiones graficas. Se instala como
# autostart XDG porque el servicio corre como root y no puede dibujar en el
# escritorio del usuario.
UI_EXEC="${DEST} --ui"
if [[ -n "${MANAGED_BY_NOTICE}" ]]; then
	UI_EXEC="${UI_EXEC} --managed-by-notice \"${MANAGED_BY_NOTICE}\""
fi
install -d -m 755 /etc/xdg/autostart
cat >/etc/xdg/autostart/lan-commander-ui.desktop <<EOF
[Desktop Entry]
Type=Application
Name=LAN Commander
Comment=Interfaz de usuario del agente LAN Commander
Exec=${UI_EXEC}
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF
echo "  Interfaz de usuario registrada para arrancar al iniciar sesion"
```

En el bloque de desinstalación, tras `rm -f "${DEST}"`, añadir:

```bash
	rm -f /etc/xdg/autostart/lan-commander-ui.desktop
```

Actualizar la cabecera de ayuda tras la línea de `--allow-from`:

```bash
# Con aviso de equipo gestionado (recomendado, se muestra al usuario):
#   sudo ./install-agent.sh --managed-by-notice "Nombre de la organizacion"
```

- [ ] **Step 3: Verificar la sintaxis de los scripts**

Run: `powershell -NoProfile -Command "$null = [System.Management.Automation.Language.Parser]::ParseFile('C:/Proyectos/lan-commander/installers/windows/install-agent.ps1', [ref]$null, [ref]$errors); $errors"`
Expected: sin errores.

Run: `bash -n C:/Proyectos/lan-commander/installers/linux/install-agent.sh`
Expected: sin salida.

- [ ] **Step 4: Prueba de instalación real**

En una VM o equipo de prueba, con PowerShell como administrador:

```powershell
cd C:\Proyectos\lan-commander\installers\windows
.\install-agent.ps1 -ManagedByNotice "Media Code"
```

Comprobar: el servicio queda instalado y corriendo; se imprime el token generado; existe `%ProgramData%\LAN Commander\`; la clave `LANCommanderUI` está en el registro. Cerrar sesión y volver a entrar: la interfaz arranca sola y muestra el aviso de equipo gestionado una vez.

Después, verificar que el desinstalado limpia todo:

```powershell
.\install-agent.ps1 -Uninstall
```

Comprobar: el servicio ya no existe, la clave `LANCommanderUI` fue eliminada y la regla de firewall también.

- [ ] **Step 5: Commit**

```bash
cd C:/Proyectos/lan-commander && git add installers/ && git commit -m "feat(installers): register client interface autostart and machine data dir"
```

---

## Verificación final de la Fase 1

- [ ] `cd agent && go build ./... && go vet ./... && go test ./... -race` — todo en verde.
- [ ] El servicio arranca en una instalación limpia de **Windows Server sin experiencia de escritorio**. Es la única prueba que descarta el fallo de carga de bibliotecas de GUI descrito en los Riesgos de la spec, y no aparece en una máquina de desarrollo.
- [ ] Con el servicio detenido, la interfaz arranca y muestra modo degradado con texto explicativo, no una pantalla vacía.
- [ ] El aviso de equipo gestionado se muestra una vez por usuario y su aceptación persiste entre reinicios.
- [ ] Tras ejecutar un comando desde el control-center, aparece en la pestaña Actividad del equipo remoto en menos de 5 segundos.
- [ ] Tras ejecutar un comando, llega una notificación de escritorio; cinco comandos seguidos producen una sola notificación agrupada.
- [ ] `activity.jsonl` conserva el historial tras reiniciar el servicio.
- [ ] Con dos usuarios con sesión abierta a la vez (cambio rápido de usuario o RDP), ambos ven su interfaz funcionando y el mismo registro de actividad.
