# Modo cliente del agente: interfaz visual para PCs no administradoras

Fecha: 2026-07-27
Estado: aprobado para planificación

## Problema

Hoy solo el equipo administrador tiene interfaz gráfica (`control-center`, app Wails).
En las demás PCs corre `agent` como servicio headless: sin ventana, sin ícono de
bandeja, sin notificaciones. El usuario de un equipo gestionado no tiene forma de
saber que el agente está instalado, que un administrador se conectó, ni que se
capturó su pantalla.

Esto tiene dos consecuencias:

1. **Transparencia.** El agente ejecuta comandos como SYSTEM y captura pantalla de
   forma silenciosa e inmediata (`agent/internal/server/handlers.go`). Un usuario
   monitoreado no puede enterarse. En despliegues laborales, informar del monitoreo
   es una obligación legal en varias jurisdicciones, no una cortesía.
2. **Carga de soporte.** Todo problema trivial (disco lleno, sin red, cola de
   impresión trabada) requiere que el administrador se conecte, porque el usuario
   no tiene ninguna herramienta local.

## Objetivos

- El usuario de cualquier PC gestionada ve que el agente corre, y qué se hizo en su
  equipo y cuándo.
- El usuario resuelve por sí mismo tareas de diagnóstico y mantenimiento acotadas.
- El usuario puede solicitar ayuda al administrador desde su propio equipo.
- La superficie de ataque local no crece: un usuario sin privilegios no debe poder
  usar la nueva interfaz para obtener ejecución de código como SYSTEM.

## No objetivos

- Consentimiento previo bloqueante. Se descartó deliberadamente (ver Decisión 2).
- Control remoto en vivo (mouse/teclado) ni streaming de pantalla.
- Reemplazar el `control-center`. La UI de cliente no administra otros equipos.
- Sustituir tokens por mTLS. Es una mejora aparte, recomendada, fuera de alcance
  de esta spec.

## Decisiones y su fundamento

### Decisión 1: binario único con modo dual (`lan-agent --ui`)

La interfaz **no puede** vivir dentro del servicio: el aislamiento de Sesión 0 en
Windows impide que un servicio dibuje UI en el escritorio del usuario. Es una
restricción del sistema operativo, no de configuración. Hace falta un proceso
aparte en la sesión del usuario.

Se evaluaron tres opciones:

| Opción | Artefactos | Aislamiento de fallos | Desajuste de versiones |
|---|---|---|---|
| Binario separado `lan-agent-ui.exe` | 2 | sí | posible |
| **Binario único, modo dual** | **1** | **sí** | **imposible** |
| App Wails independiente | 2 (+90 MB/PC) | sí | posible |

`lan-agent --ui` es un proceso distinto aunque comparta binario, así que ofrece el
mismo aislamiento en runtime que un ejecutable aparte: si la UI se cae, el servicio
sigue intacto. Con un solo artefacto que firmar, distribuir y versionar, el
desajuste de versiones entre servicio e interfaz pasa a ser estructuralmente
imposible — la causa más común de incidencias difíciles de diagnosticar en flotas.
El costo es tamaño de binario (unos pocos MB de dependencias de GUI que el servicio
nunca invoca), irrelevante frente a ese beneficio.

La app Wails se descarta por desproporción: ~90 MB por PC con WebView2 para mostrar
una bandeja con avisos.

### Decisión 2: notificar siempre, no bloquear nunca

Cuando el administrador ejecuta una acción, se ejecuta de inmediato y el usuario
recibe un aviso a posteriori, registrado en su historial local.

La alternativa (pedir aprobación con timeout) da más control al usuario pero deja al
administrador esperando a un equipo que puede estar sin nadie delante, que es el
caso normal en mantenimiento fuera de horario. Es el modelo que siguen los RMM
comerciales.

**Consecuencia asumida:** esto cubre transparencia y auditoría, no consentimiento
previo. El usuario se entera después, no antes. Para cumplir el deber de informar de
antemano se añade un aviso en el primer inicio de sesión (ver Componente 5), no un
diálogo en tiempo real.

### Decisión 3: el autoservicio corre en el contexto del usuario

El camino aparentemente más simple sería que la UI se conectara al WebSocket que el
agente ya expone en `127.0.0.1:9474`, reusando el protocolo existente sin escribir
nada nuevo.

**Se rechaza por escalada de privilegios local.** El agente corre como SYSTEM y
ejecuta comandos arbitrarios sin whitelist (`agent/internal/executor/executor.go`).
Un usuario sin privilegios que obtuviera el token de ese WebSocket tendría SYSTEM en
su propia máquina. En un aula, cualquier alumno podría hacerse administrador.

En su lugar:

- Diagnóstico, mantenimiento del perfil propio y tests de red se ejecutan **dentro
  del proceso de UI**, con los privilegios del usuario. No requieren SYSTEM, así que
  no se le piden a SYSTEM.
- El canal hacia el servicio se limita a lo que genuinamente necesita privilegios o
  acceso a estado que el usuario no puede leer.
- **Ningún string de comando cruza el canal local.** Solo identificadores de un
  conjunto cerrado (ver Componente 2). Aunque alguien suplantara el canal, no
  obtiene ejecución arbitraria.

### Decisión 4: los scripts de autoservicio viven en el agente

El motor de scripts actual está en el lado administrador
(`control-center/backend/scripting/engine.go`); el agente no tiene paquete de
scripting. Si la UI leyera los scripts de allí, el autoservicio dejaría de funcionar
justo cuando el administrador está desconectado, que es cuando el usuario más lo
necesita.

Por eso el administrador **despliega** scripts al agente, que los almacena
localmente. La UI los ejecuta sin depender de conectividad con el administrador.

## Arquitectura

```
┌─ Sesión 0 (SYSTEM) ────────────────┐      ┌─ Sesión del usuario ───────────┐
│  lan-agent (servicio)              │      │  lan-agent --ui                │
│   ├─ WebSocket :9474  ← admin      │      │   ├─ Bandeja + ventana         │
│   ├─ activitylog (JSONL)           │◄────►│   ├─ Diagnóstico   (local)     │
│   ├─ scriptstore (ProgramData)     │ IPC  │   ├─ Mantenimiento (local)     │
│   └─ localapi (pipe/socket, ACL)   │      │   └─ Tests de red  (local)     │
└────────────────────────────────────┘      └────────────────────────────────┘
```

Arranque de la UI:

- Windows: `HKLM\Software\Microsoft\Windows\CurrentVersion\Run`, entrada
  `LANCommanderUI` con valor `"<InstallDir>\lan-agent.exe" --ui`. Aplica a todo
  usuario que inicie sesión.
- Linux: `/etc/xdg/autostart/lan-commander-ui.desktop`.

Ambos los añade el instalador existente (`installers/windows/install-agent.ps1`,
`installers/linux/install-agent.sh`) y los elimina en el desinstalado.

## Componentes

### 1. `agent/internal/activitylog`

**Qué hace.** Registra toda acción atendida por el agente y la difunde a las UIs
suscritas.

**Interfaz.** `Append(Event) error`, `Recent(n int) ([]Event, error)`,
`Subscribe() (<-chan Event, func())`.

**Dependencias.** Solo biblioteca estándar.

**Almacenamiento.** JSONL con append en
`%ProgramData%\LAN Commander\activity.jsonl` (Linux: `/var/lib/lan-commander/`),
rotado al superar 5 MB conservando un archivo previo. Se eligió JSONL sobre SQLite
porque el patrón es append-only con lectura de la cola: menos dependencias, y un
archivo truncado por corte de energía pierde una línea en lugar de corromper una
base.

**Orden de operaciones.** Persistir primero, difundir después. Si el usuario no
había iniciado sesión, al entrar ve lo ocurrido durante su ausencia — que es
exactamente el valor de la transparencia.

**La difusión nunca bloquea.** Cada suscriptor recibe un canal con buffer de 64
eventos y `Append` escribe con `select` y rama `default`: si el buffer está lleno
porque el proceso de UI está colgado, el evento se descarta para ese suscriptor y se
incrementa un contador de descartes. Un `Append` que bloqueara propagaría el bloqueo
hasta el handler del administrador, de modo que una UI de usuario colgada podría
frenar la administración del equipo — inaceptable. La persistencia es la fuente de
verdad; el stream es solo para inmediatez, y la vista de Actividad siempre relee del
archivo al abrirse. Cuando hubo descartes, la UI muestra "Se perdieron avisos en
vivo; el registro está completo".

**Concurrencia.** `Append` se llama desde varias goroutines (un administrador puede
tener varias conexiones, y puede haber varios administradores). Serializa con un
mutex sobre el `io.Writer`, y expone la lista de suscriptores bajo su propio
`RWMutex` para no retener el de escritura durante la difusión.

**Múltiples sesiones de usuario.** En equipos con cambio rápido de usuario, RDP o
varios escritorios, existirá un proceso de UI por sesión, todos suscritos a la vez.
El registro es **por equipo, no por usuario**: las acciones del administrador afectan
a la máquina, y ocultar a un usuario lo que se hizo mientras otro estaba conectado
vaciaría de sentido la transparencia. Las acciones de autoservicio, en cambio, sí son
por usuario (limpiar temporales toca el perfil de quien la invoca) y así se registran,
anotando el usuario en el campo `actor`.

**Campos de `Event`.** `timestamp`, `action` (mismo vocabulario que el audit log del
administrador: `exec_command`, `screenshot`, `list_dir`, `get_file`, `send_file`,
`system_info`), `actor` (IP de origen del administrador), `detail` (resumen legible,
sin volcar contenido de archivos ni salida de comandos), `outcome`
(`success`/`error`).

**Punto de integración.** El `switch` de `handleMessage`
(`agent/internal/server/handlers.go`) emite un evento por cada caso atendido, tras
resolverlo, con su resultado real.

### 2. `agent/internal/localapi`

**Qué hace.** Expone al proceso de UI, en la misma máquina, la superficie mínima que
requiere privilegios o estado no legible por el usuario.

**Transporte.** Windows: named pipe `\\.\pipe\lan-commander-agent` con ACL que
concede lectura/escritura a `INTERACTIVE` y control total a `SYSTEM`. Linux: socket
Unix en `/run/lan-commander/agent.sock`, propietario `root`, grupo del usuario
interactivo, modo `0660`.

**Codificación.** JSON delimitado por líneas, una petición por línea, una respuesta
por línea; el stream de actividad usa la misma conexión tras `SubscribeActivity`.

**Operaciones (conjunto cerrado y completo).**

| Operación | Parámetros | Devuelve | Por qué necesita al servicio |
|---|---|---|---|
| `SubscribeActivity` | — | stream de `Event` | Solo el servicio observa al administrador |
| `GetActivityLog` | `limit` | `[]Event` | El historial vive en ProgramData, no legible por el usuario |
| `ListApprovedScripts` | — | `[]ScriptSummary` | El almacén está en ProgramData |
| `RunApprovedScript` | `script_id` | `ExecResult` | Puede requerir elevación |
| `RunMaintenanceAction` | `action_id` | `ExecResult` | Acciones que requieren privilegios |
| `SendHelpRequest` | `message`, `urgency` | `ack` | El servicio tiene el socket hacia el administrador |

**Propiedad de seguridad central.** `script_id` y `action_id` son identificadores
validados contra conjuntos conocidos por el servicio. Ningún parámetro alimenta la
ejecución de código: no pasa texto de comando, ruta arbitraria ni argumentos. Un
atacante con acceso al canal solo puede invocar lo que el administrador ya autorizó.

El único campo de texto libre es `message` en `SendHelpRequest`, y es inerte: se
almacena y se reenvía al administrador, nunca se interpola en un comando, una ruta
ni una consulta. Se trata como dato no confiable en todo su recorrido — el
`control-center` debe mostrarlo escapado, ya que lo escribe un usuario cuyo equipo
podría estar comprometido.

**Límites.** Peticiones con timeout de 30 s; tamaño máximo de petición 64 KB;
`SendHelpRequest` limitado a una cada 60 s por sesión, para que la UI no pueda
inundar al administrador.

**Concurrencia.** Debe aceptar varios clientes a la vez, uno por sesión de usuario
activa: en Windows el pipe se crea con `PIPE_UNLIMITED_INSTANCES` y una goroutine de
atención por conexión. Un solo cliente sería un error de diseño en equipos con cambio
rápido de usuario o RDP, donde el segundo usuario en iniciar sesión se quedaría sin
interfaz sin explicación posible.

**Ciclo de vida.** El servidor del canal arranca y para con el servicio. En Linux el
directorio `/run/lan-commander` es volátil: lo crea el propio servicio al arrancar
(no se puede asumir que exista tras un reinicio), con `0755` y propietario `root`.

### 3. `agent/internal/scriptstore`

**Qué hace.** Almacena los scripts que el administrador despliega y expone los
marcados como visibles para el usuario.

**Interfaz.** `Put(Script) error`, `Get(id string) (Script, error)`,
`ListUserVisible() ([]ScriptSummary, error)`, `Delete(id string) error`.

**Almacenamiento.** `%ProgramData%\LAN Commander\scripts\<id>.json`, escribible solo
por SYSTEM y administradores. Cada `Script` lleva `id`, `name` (legible),
`description`, `content`, `shell`, `user_visible` (bool),
`requires_elevation` (bool).

**Ejecución.** Con `requires_elevation` falso, la UI podría ejecutarlo en contexto de
usuario; para mantener un solo camino de código y un solo punto de auditoría, **todo
script aprobado se ejecuta siempre por el servicio** vía `RunApprovedScript`, y toda
ejecución queda en el `activitylog` con `actor: "local-user"`. La distinción de
elevación determina el contexto en que el servicio lanza el proceso, no quién lo
lanza.

**Extensión de protocolo.** Nuevo mensaje administrador→agente `MsgDeployScript`
(`deploy_script`) con el `Script` completo, y `MsgDeleteScript` (`delete_script`) con
el `id`. Requieren autenticación como cualquier otro mensaje. En el lado
administrador, `scripting.Engine` gana los campos `user_visible` y
`requires_elevation`, y el `ScriptEditor` un par de casillas y un botón de despliegue
por agente.

**Limpieza asociada.** El constante `MsgScriptRun` del protocolo del agente
(`agent/internal/protocol/types.go`) está declarado pero no tiene caso en
`handleMessage`: es código muerto. El administrador ejecuta scripts descomponiéndolos
en `exec_command` línea por línea. Se elimina para que no se confunda con
`deploy_script`.

### 4. `agent/internal/selfservice`

**Qué hace.** Implementa las acciones que el usuario ejecuta sobre su propio equipo.

**Se divide en dos subpaquetes, y la separación es de seguridad, no de orden.**

- `selfservice/userctx` — solo lo importa el proceso de UI. Nunca lo alcanza el
  canal local.
- `selfservice/elevated` — solo lo importa el servicio, invocado por
  `RunMaintenanceAction`.

El motivo: varias de estas acciones **resuelven rutas según el usuario que las
ejecuta**. `%TEMP%` apunta al perfil de quien invoca, así que "limpiar temporales"
llamada por el servicio borraría el temporal de SYSTEM, no el del usuario — silenciosa
y equivocadamente. Si ambos grupos vivieran en un mismo paquete, nada impediría que
una futura operación del canal invocara por descuido una función pensada para
contexto de usuario. La frontera de paquetes convierte ese error en un fallo de
compilación en lugar de un incidente.

Regla asociada: ninguna función de `userctx` puede ser alcanzable desde `localapi`.
Se verifica con un test que comprueba que `localapi` no importa `userctx`, ni
directa ni transitivamente.

**Acciones de contexto de usuario** (sin pasar por el canal local):

- Diagnóstico: CPU, RAM, discos, red, hostname, uptime. Reutiliza
  `agent/internal/system/monitor.go`, que ya lo recoge; funciona sin privilegios.
- Limpieza de temporales del propio perfil (`%TEMP%` del usuario).
- Vaciado de la papelera del propio usuario.
- Tests de conectividad: ping a la puerta de enlace, resolución DNS, alcance de
  internet y de un host interno configurable.
- Reinicio del equipo: los usuarios de estación de trabajo tienen
  `SeShutdownPrivilege` por defecto. Se intenta en contexto de usuario y, si falla
  por privilegios, se informa con claridad en vez de escalar en silencio.

**Acciones elevadas** (conjunto cerrado, vía `RunMaintenanceAction`):

- `restart_print_spooler`
- `flush_dns_cache`

Cada una es un comando fijo compilado en el binario, sin parámetros. Añadir una
acción es un cambio de código revisable, no configuración.

### 5. `agent/internal/ui`

**Qué hace.** Bandeja del sistema y ventana del cliente.

**Dependencias.** `fyne.io/fyne/v2`, que cubre bandeja del sistema
(`desktop.App.SetSystemTrayMenu`), ventana y notificaciones nativas con una sola
dependencia y sin WebView. Se prefiere sobre combinar `getlantern/systray` con otra
biblioteca de ventanas porque evita dos dependencias de GUI que hay que mantener
coordinadas para el mismo proceso.

**Estados del ícono.** Gris (sin conexión con el servicio), verde (servicio activo,
sin administrador conectado), ámbar (administrador conectado ahora).

**Vistas.**

1. *Estado*: datos del equipo, estado del agente, quién está conectado.
2. *Actividad*: historial de lo que se hizo en el equipo, con fecha y actor.
3. *Herramientas*: diagnóstico, mantenimiento, tests de red, scripts aprobados.
4. *Pedir ayuda*: formulario con mensaje y urgencia.

**Aviso de primer inicio.** En el primer arranque por usuario se muestra una vez un
aviso de equipo gestionado, con texto configurable por el administrador en la
instalación (`-ManagedByNotice`). Cubre el deber de informar de antemano que la
Decisión 2 deja sin resolver. Se registra su aceptación en el `activitylog`.

### 5b. Textos de la interfaz

El texto es la parte funcional de esta interfaz, no decoración: la diferencia entre
informar y aparentar que se pide permiso está en las palabras. Se especifican aquí
para que no se improvisen en la implementación.

**Terminología fija.** "personal de sistemas" para el administrador (no "admin" ni
"root"), "sesión de soporte" para una conexión activa, "registro de actividad" para
el historial. El mismo término en todas las vistas y notificaciones.

**Aviso de equipo gestionado (primer inicio de sesión de cada usuario).**

> **Este equipo lo gestiona {Organización}**
>
> El personal de sistemas puede ejecutar programas, consultar archivos y capturar la
> pantalla de este equipo como parte del soporte técnico.
>
> Cada acción queda registrada. Puedes consultar el registro completo cuando quieras
> desde el ícono de LAN Commander en la barra de tareas.
>
> Este aviso te informa de cómo funciona el equipo; no te pide permiso.
>
> `[Entendido]`

La última línea es deliberada. Sin ella, un aviso con un único botón se lee como un
consentimiento, y el usuario concluiría que puede negarse. Decir qué *no* es el aviso
evita esa lectura falsa. `{Organización}` viene de `--managed-by-notice`.

**Notificaciones de actividad.** Origen: "Soporte técnico". Un hecho por
notificación, en voz pasiva impersonal para no sugerir vigilancia dirigida, con la
hora y una acción para abrir el registro.

| Acción | Texto |
|---|---|
| `screenshot` | Se capturó la pantalla de este equipo · {hora} |
| `exec_command` | Se ejecutó un comando en este equipo · {hora} |
| `list_dir`, `get_file` | Se consultaron archivos de este equipo · {hora} |
| `send_file` | Se copió un archivo a este equipo · {hora} |
| script aprobado | Se ejecutó «{nombre}» · {hora} |

Acción secundaria: `Ver registro`. Si llegan tres o más eventos en un minuto se
agrupan en una sola notificación: "{n} acciones de soporte en este equipo · {hora}".
Notificar cada una sería ruido, y el ruido se ignora — el registro conserva el
detalle.

**Estados de la bandeja (tooltip).**

- Gris: `LAN Commander — sin conexión con el servicio`
- Verde: `LAN Commander — activo`
- Ámbar: `LAN Commander — sesión de soporte en curso`

**Estado vacío del registro.**

> Sin actividad todavía. Cuando el personal de sistemas haga algo en este equipo,
> aparecerá aquí con la fecha y la hora.

**Modo degradado.**

> **Sin conexión con el servicio del agente**
>
> El diagnóstico y las pruebas de red siguen disponibles. El registro de actividad,
> los scripts y las solicitudes de ayuda no lo están hasta que se restablezca la
> conexión. Reintentando automáticamente.

Nombra lo que sí funciona antes de lo que no: el usuario abrió la app para hacer
algo, y buena parte sigue siendo posible.

**Solicitudes de ayuda.**

- Enviada: `Solicitud enviada al personal de sistemas.`
- En cola: `Solicitud guardada. No hay nadie de sistemas conectado en este momento;
  se enviará automáticamente en cuanto se restablezca la conexión.`
- Límite de frecuencia: `Espera un momento antes de enviar otra solicitud.`

**Confirmación de reinicio.** Titular con la consecuencia, botón con el verbo:

> **¿Reiniciar este equipo?** Se cerrarán todos los programas abiertos. Guarda tu
> trabajo antes de continuar.
>
> `[Reiniciar ahora]` `[Cancelar]`

**Resultados de mantenimiento.** Cuantificar cuando se pueda: `Se liberaron
{tamaño} de espacio.` Si falla por privilegios, decir el motivo real y la salida:
`No se pudo reiniciar el equipo: tu cuenta no tiene permiso para hacerlo. Pide ayuda
al personal de sistemas.` Nunca "Error inesperado".

**Idioma.** Esta interfaz va en español, como los instaladores. El `control-center`
está hoy en inglés; la inconsistencia es aceptable porque son públicos distintos
(usuario final frente a administrador), pero conviene decidirlo a conciencia y no
por inercia. Si más adelante se internacionaliza, los textos de arriba son los
únicos que ve el usuario final y son el conjunto a extraer primero.

### 6. Solicitudes de ayuda hacia el administrador

El protocolo ya admite mensajes agente→administrador no solicitados: `MsgAgentInfo` y
`MsgSystemUpdate` se envían sin que nadie los pida. Por tanto el añadido es pequeño:

- Nuevo `MsgHelpRequest` (`help_request`) con `hostname`, `user`, `message`,
  `urgency`, `timestamp`.
- El agente lo difunde a todos los administradores autenticados en ese momento.
- Si no hay ninguno, lo encola en
  `%ProgramData%\LAN Commander\pending-help.jsonl` (máximo 20, descartando el más
  viejo) y la UI informa al usuario de que quedó en espera.
- **La cola se vacía cuando un administrador se autentica**, no "al reconectar": el
  agente es un servidor y nunca inicia conexiones: es el `control-center` quien se
  conecta a él. El punto de entrega es por tanto el final de `handleAuth`
  (`agent/internal/server/handlers.go`), tras marcar al cliente como autenticado.
- Una solicitud entregada se elimina de la cola solo después de escribirla con éxito
  en el socket. Ante duda se reenvía: un duplicado en la bandeja del administrador es
  preferible a una petición de ayuda perdida.
- En el `control-center`, `onAgentMessage` (`app.go`) lo convierte en entrada de
  auditoría y notificación en la UI del administrador.

## Manejo de errores

- **Servicio caído o canal ausente:** la UI arranca en modo degradado. Diagnóstico y
  tests de red siguen disponibles (son locales); historial, scripts y ayuda se
  muestran deshabilitados con la razón visible, no en silencio. Reconexión con
  backoff exponencial de 1 s a 30 s.
- **UI caída:** el servicio no se ve afectado. El `activitylog` sigue persistiendo,
  así que al relanzarse la UI no hay hueco en el historial.
- **Escritura del log fallida:** se registra en el log del servicio y la acción del
  administrador **procede igualmente**. Un fallo de auditoría local no debe impedir
  la administración del equipo, pero sí debe ser visible.
- **Suplantación del canal:** mitigada por diseño, no por detección — sin strings de
  comando en el protocolo, el peor caso es invocar acciones ya autorizadas.

## Estrategia de pruebas

Testeable sin GUI, que es la mayor parte:

- `activitylog`: append y lectura de la cola, rotación en el umbral, recuperación
  ante última línea truncada, difusión a múltiples suscriptores, orden
  persistir-antes-de-difundir. **Y el caso que motivó el diseño: con un suscriptor
  que no consume, `Append` retorna sin bloquearse, cuenta el descarte, y el evento
  sigue presente en el archivo.**
- `localapi`: codificación de cada operación, rechazo de `script_id`/`action_id`
  desconocidos, límite de tamaño, timeout, límite de frecuencia de
  `SendHelpRequest`, y varios clientes concurrentes atendidos a la vez. Con socket en
  directorio temporal, sin privilegios.
- Frontera de privilegios: test que falla si `localapi` importa
  `selfservice/userctx`, directa o transitivamente.
- Cola de solicitudes de ayuda: entrega al autenticarse un administrador (no antes),
  descarte del más viejo al llegar a 20, y que una solicitud no se borre de la cola si
  falla la escritura al socket.
- `scriptstore`: put/get/list/delete, que `ListUserVisible` excluya los no visibles,
  rechazo de `id` con separadores de ruta.
- `selfservice`: tests de red contra servidores locales de prueba; las acciones
  destructivas (limpieza, papelera) contra directorios temporales.
- Extensiones de protocolo: ida y vuelta de serialización de `MsgHelpRequest`,
  `MsgDeployScript`, `MsgDeleteScript`.
Queda como verificación manual la bandeja, los avisos nativos y el aviso de primer
inicio.

Verificación no automatizable pero obligatoria antes de desplegar: **el servicio
arranca en una instalación limpia de Windows Server sin experiencia de escritorio**
(ver Riesgos). Es la única forma de descartar el fallo de carga de bibliotecas de GUI,
que no aparece en una máquina de desarrollo.

## Cambios en los instaladores

- Registrar y desregistrar el arranque automático de la UI (clave `Run` / autostart).
- Crear `%ProgramData%\LAN Commander\` con permisos de escritura solo para SYSTEM y
  administradores.
- Nuevo parámetro `-ManagedByNotice` / `--managed-by-notice` con el texto del aviso
  de equipo gestionado.

## Fases de entrega

El alcance abarca seis componentes, demasiado para un solo ciclo. Se entrega en tres
fases, cada una útil por sí sola:

**Fase 1 — Transparencia.** `activitylog`, `localapi` (solo `SubscribeActivity` y
`GetActivityLog`), `ui` con las vistas de Estado y Actividad, aviso de primer inicio,
cambios de instalador para el arranque automático. Al cerrarla, el hueco de
transparencia queda resuelto, que es el problema con consecuencias legales.

**Fase 2 — Autoservicio.** `selfservice` completo, `RunMaintenanceAction`, vista de
Herramientas. Al cerrarla, baja la carga de soporte.

**Fase 3 — Scripts y ayuda.** `scriptstore`, `RunApprovedScript`,
`ListApprovedScripts`, `MsgDeployScript`/`MsgDeleteScript`, `MsgHelpRequest` con su
cola, cambios en `ScriptEditor` del administrador, vista de Pedir ayuda.

La verificación de viabilidad de compilación de Fyne (ver Riesgos) va al inicio de la
Fase 1: si obliga a cambiar al Enfoque A, conviene descubrirlo antes de escribir la
UI, no después.

## Riesgos

- **Falsa sensación de consentimiento.** La UI podría hacer creer al usuario que
  puede impedir acciones, cuando solo las observa. El texto debe ser explícito:
  informa, no autoriza.
- **Dependencias de GUI en el binario del servicio: el riesgo principal.** Fyne
  enlaza contra OpenGL y X11 vía CGO. Esas bibliotecas se resuelven **al cargar el
  proceso**, no al usarlas: un binario que las enlace no arranca en una máquina que
  no las tenga. En un servidor Linux headless eso significa que **el servicio del
  agente no arrancaría**, convirtiendo una mejora de UI en una regresión que deja el
  equipo sin monitoreo. La inicialización perezosa no protege de esto, porque el
  fallo ocurre antes de ejecutar `main`.

  Mitigación, por plataforma:

  - **Windows** (destino principal): las DLL de OpenGL están siempre presentes. El
    binario único es seguro. Se verifica arrancando el servicio en una instalación
    limpia de Windows Server sin experiencia de escritorio.
  - **Linux**: la UI queda tras la etiqueta de compilación `ui`. Se publican dos
    artefactos —`lan-agent` (headless, sin Fyne, para servidores) y
    `lan-agent-desktop` (con `-tags ui`)— y el instalador elige según detecte
    entorno gráfico. Se acepta el versionado doble solo en Linux, donde el problema
    es real, en lugar de imponerlo en toda la flota.

  Esta verificación es la primera tarea de la Fase 1: si en Windows apareciera el
  mismo problema, todo el enfoque de binario único cae y hay que volver al Enfoque A,
  y conviene saberlo antes de escribir la interfaz.
- **Superficie nueva en una máquina hostil.** El canal local es alcanzable por
  cualquier proceso del usuario. La mitigación es el conjunto cerrado de
  operaciones; hay que sostenerla en el tiempo — cualquier futura operación que
  acepte texto libre rompe la propiedad.

**Verificación de Fyne (Tarea 1, Fase 1) — INVÁLIDA, corregida el 2026-07-27.**
La nota original de esta sección afirmaba "arranca en Windows 11 Enterprise
10.0.26100", pero esa verificación no probó nada: `go get fyne.io/fyne/v2`
había añadido la dependencia a `go.mod`, pero ningún archivo Go del proyecto
la importaba. Confirmado con `grep -rn "fyne" --include="*.go" .` (sin
resultados), `go list -deps ./cmd/lan-agent | grep -c fyne` (0), y
`go mod why fyne.io/fyne/v2` ("main module does not need package
fyne.io/fyne/v2"). El linker nunca incluyó Fyne en el binario que se arrancó;
el "arranca" registrado no descartaba el riesgo de esta sección, solo
confirmaba que un binario sin Fyne arranca.

Corrección intentada el 2026-07-27, en Windows 11 Enterprise 10.0.26100: para
forzar el enlace real se creó `agent/internal/ui/app.go` con una función
`Run` que construye una `fyne.App` de verdad (`app.NewWithID(...)`) y una
`fyne.Window`, y se modificó `agent/cmd/lan-agent/main.go` para importar
`internal/ui` y despachar a `ui.Run` bajo el flag `--ui`, reproduciendo la
topología final en la que el binario del servicio y el de la interfaz son el
mismo ejecutable. Con esos cambios, `go list -deps ./cmd/lan-agent | grep -c
fyne` pasó a devolver 95 y `go mod why fyne.io/fyne/v2` mostró la cadena de
import real (`.../internal/ui` → `fyne.io/fyne/v2`), confirmando que el
enlace real por fin estaba en juego.

Sin embargo, la verificación **no pudo completarse en esta máquina**: el
driver de escritorio de Fyne en Windows (`glfw` + OpenGL, paquete
`github.com/go-gl/gl`) requiere CGO, y este entorno de compilación no tiene
un compilador de C instalado ni acceso de administrador para instalarlo.
`go build ./...` con `CGO_ENABLED=0` (el valor por defecto detectado en esta
máquina) falla con "build constraints exclude all Go files" para el paquete
`go-gl/gl`; forzando `CGO_ENABLED=1` falla de inmediato con
`cgo: C compiler "gcc" not found: exec: "gcc": executable file not found in
%PATH%`; y `choco install mingw -y` para instalar el compilador falla con
"Acceso denegado" por falta de privilegios de administrador. No se intentó
ningún otro workaround (drivers alternativos, compiladores portátiles,
etc.), conforme a la instrucción de no improvisar alternativas cuando la
puerta de riesgo no se puede cruzar.

**Resultado: la puerta de riesgo sigue sin verificarse empíricamente.** No es
un "no arranca" (nunca llegó a compilar el binario real), pero tampoco es un
"arranca" válido — es un bloqueo de herramientas de compilación en esta
máquina de desarrollo, distinto del riesgo arquitectónico que la tarea
buscaba descartar. Los cambios de código de esta corrección (`internal/ui/app.go`,
los flags `--ui`/`--managed-by-notice` en `main.go`, y las dependencias
transitivas añadidas a `go.mod`/`go.sum`) se revirtieron para no dejar en el
árbol un estado a medio verificar. Antes de retomar la Fase 1 sobre la
premisa de un binario único, esta verificación debe repetirse en una máquina
Windows con un compilador de C (MinGW-w64/TDM-GCC) instalado y disponible en
`PATH`.

## Trabajo futuro relacionado (fuera de alcance)

**Identidad por equipo con mTLS en lugar de tokens.** Con tokens obligatorios, el
`control-center` guarda un token por PC en texto plano en `lan-commander.db`: copiar
ese archivo compromete la flota entera. Con mTLS, cada agente genera su par de
claves al instalarse y el administrador firma su certificado; se obtiene cifrado en
tránsito (hoy ausente), ningún secreto compartido que robar, revocación por equipo, e
identidad estable para el audit log sin depender de IP ni hostname. El momento
natural es una reinstalación de flota; hacerlo con pocos equipos es una tarde, con
cincuenta es un proyecto de migración.
