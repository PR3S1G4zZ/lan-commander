---
title: Mejoras de lógica y experiencia del Control Center
date: 2026-10-02
status: implemented
tags:
  - control-center
  - ux
  - backend
---

# Mejoras de lógica y experiencia del Control Center

Este informe registra los cambios de interfaz y backend realizados tras revisar el estado del Control Center. Se contrastaron con [[2026-08-22-lan-commander-completion-design]] y [[2026-08-22-lan-commander-completion]]. El proyecto no tiene vault; no se creó uno.

## Interfaz

- El dashboard representa todos los discos reportados, indica explícitamente cuando no hay datos de discos y muestra los campos de red disponibles (IP, MAC y hostname). No infiere tráfico ni velocidad que el agente no proporcione.
- Multi-Exec elimina selecciones de agentes desconectados y vuelve a filtrar destinos antes de enviar comandos.
- Las sesiones pueden reconectarse por ID usando sus opciones guardadas. El token no se incorpora al modelo de sesión del frontend; la reconexión permanece en el backend.
- El formulario de sesión permite guardar configuración opcional TLS, CA y nombre del servidor; las sesiones TLS se identifican visualmente.
- El explorador muestra un estado accesible de subida/descarga en curso, sin porcentaje ficticio, porque el binding no informa progreso por bytes.
- La navegación, selección de agente, selección múltiple y métricas exponen estados accesibles además de los estilos visuales.

## Backend

- `ReconnectSession` carga una sesión por ID, restablece conexión con sus opciones de autenticación/TLS, actualiza `last_connected`, registra auditoría y devuelve solo el ID del agente.
- `GetSessions` elimina el token de la respuesta entregada al renderer.
- Una conexión existente al mismo host/puerto solo se reutiliza si token y opciones TLS coinciden; de lo contrario se devuelve un error explícito.
- Los nombres de scripts se validan como nombres de archivo individuales; se rechazan rutas, separadores y traversal. La carga ignora symlinks y el guardado usa archivo temporal y rename.
- Operaciones de listado, sistema, captura, Wake-on-LAN y scripts auditan el resultado final, evitando el registro prematuro de éxito.
- El descubrimiento mDNS se inicia sin esperar a que terminen las reconexiones guardadas.
- Las transferencias comparten un contexto cancelable del ciclo de vida de App y el shutdown espera su finalización antes de cerrar dependencias.

## Correcciones posteriores a la revisión

- `GetAgents` también devolvía `AgentInfo.AuthToken` al renderer; ahora lo vacía (`redactAgentTokens`), igual que `GetSessions`.
- `GetAgents` ya no toma `App.mu`: connect y reconnect lo mantienen durante todo el dial y el handshake, y el sondeo de 2 s de la interfaz se congelaba mientras un host inalcanzable agotaba su timeout.
- `Upload` envía `cancel_file` también cuando su contexto se cancela (por ejemplo al cerrar la app), para no dejar un `.lan-commander-upload-*.part` huérfano en el agente.

## Límites

- El binding de transferencia no comunica bytes completados, por lo que la UI solo expresa que una transferencia está activa.
- Una petición WebSocket ya iniciada no acepta cancelación por contexto. El cierre detiene el trabajo entre peticiones y espera hasta que la petición pendiente termine o expire su timeout.
- Las opciones TLS se conservan por sesión; la política para una conexión activa con opciones diferentes es rechazar, no sustituir la conexión existente.
- Las pruebas de interfaz son de helpers de estado; no se añadió un harness de pruebas de componentes Svelte.

## Verificación

Verificado en Windows durante esta tarea:

- Frontend: `npm test`, `npm run check` y `npm run build`.
- Control Center: `go test ./... -count=1` y `go vet ./...`.
- `git diff --check`.

El test con `-race` no quedó verificado: requiere CGO habilitado y compilador C disponible en el entorno.
