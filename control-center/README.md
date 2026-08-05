# Control Center

`control-center` es la aplicacion de escritorio del administrador. Usa Wails para empaquetar el frontend Svelte dentro de un ejecutable nativo; en produccion no abre una pagina visible ni requiere que el usuario navegue a `localhost`.

## Desarrollo

Desde esta carpeta:

```powershell
npm.cmd --prefix frontend install
npm.cmd --prefix frontend run check
npm.cmd --prefix frontend run build
go test ./...
```

Para ejecutar la aplicacion durante el desarrollo necesitas el CLI de Wails:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
wails dev
```

El servidor Vite/localhost que pueda aparecer durante `wails dev` es una herramienta exclusiva de desarrollo. El ejecutable distribuible se compila con `scripts/build-all.ps1` y se entrega en `build/bin/lan-commander.exe`.