# serve-go ⚡

Servidor estático ultra-rápido en Go — la alternativa directa a `npx serve .` sin Node: un solo binario, cero dependencias, arranque instantáneo.

**Benchmark real (Pi 5, ARM64): [v1.1.0] serve-go 23.066 rps vs npx serve 659 rps — 33x más rápido.**

```
npx serve:   2.000 req, 50 conc.  →  3.033 ms  |  659 rps  |  avg 1,52 ms
serve-go:    2.000 req, 50 conc.  →    115 ms  | 17.280 rps |  avg 0,06 ms   (frío)
serve-go:    2.000 req, 50 conc.  →     86 ms  | 23.066 rps |  avg 0,04 ms   (cache caliente)
serve-go:   10.000 req, 100 conc. →    458 ms  | 21.833 rps |  0 errores
```

## Por qué es más rápido

- **Cache LRU en RAM** acotada por bytes (64 MB default): el hot path sirve del heap, sin `syscall` por request.
- **ETag + 304**: clientes que repiten requests no re-descargan nada.
- **Content-types por tabla propia**: cero llamadas al sistema preguntando el mime.
- **Binario nativo**: sin runtime de Node (~50-80 MB) ni resolución de módulos (~2-4 s de arranque en frío con npx).

## Instalación

### go install (Linux / macOS / Windows, si tenés Go)

Con Go 1.22+ en el PATH, un solo comando instala el binario en `$GOPATH/bin` (normalmente `~/go/bin` — asegurate de que esté en tu `PATH`):

```bash
go install github.com/xErik444x/serve@latest
```

```powershell
go install github.com/xErik444x/serve@latest   # genera serve.exe en %USERPROFILE%\go\bin
```

> Requiere una release taggeada (`v1.2.0` o posterior) — `@latest` resuelve la última tag `vX.Y.Z` publicada en [Releases](../../releases).

### Windows (recomendado sin Go: release precompilado)

1. Bajá `serve-windows-amd64.exe` de la última [release](../../releases).
2. Renombralo a `serve.exe` y ponelo en una carpeta del `PATH`:

```powershell
mkdir $HOME\bin -Force
move .\serve-windows-amd64.exe $HOME\bin\serve.exe

# agregar al PATH (permanente, para la sesión de usuario):
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";$HOME\bin", "User")
# cerrá y reabrí la terminal
```

3. Listo — ya se llama `serve` directo:

```powershell
cd C:\misitio
serve              # sirve . en http://localhost:3000
serve -p 8080 -o   # puerto 8080 + abre el navegador
serve -d .\dist    # sirve ./dist en vez de .
serve -L           # accesible desde la LAN
```

### Windows (compilarlo vos mismo)

Requisitos: [Go 1.22+](https://go.dev/dl/).

```powershell
git clone https://github.com/xErik444x/serve.git
cd serve-go
go build -o serve.exe .
# opcional: copiarlo al PATH
copy serve.exe $HOME\bin\
```

### Linux / macOS

```bash
git clone https://github.com/xErik444x/serve.git && cd serve-go
make build            # o: go build -o serve .
sudo cp serve /usr/local/bin/
```

## Uso

```
serve [flags]
```

| Flag | Default | Descripción |
|------|---------|-------------|
| `-p` | `3000` | puerto; si está ocupado busca el siguiente libre automáticamente |
| `-d` | `.` | directorio a servir |
| `-o` | off | abre el navegador al arrancar |
| `-cors` | off | CORS `*` (para frontends que consumen APIs en otro origen) |
| `-L` | off | escuchar en `0.0.0.0` (accesible desde la LAN; default solo localhost) |
| `-cache` | `64` | tope de la cache LRU en MB |
| `-no-cache` | off | deshabilita la cache (dev) |
| `-no-browse` | off | deshabilita el listing de dirs (404 en dirs sin index.html) |
| `-no-color` | off | sin colores ANSI (para logs/scripts) |
| `-q` | off | sin log por request |
| `-no-menu` | off | arranca sin el menú interactivo (CI / scripts / background) |
| `-v` | — | versión |

### Dashboard y menú (TUI)

Al arrancar, el CLI levanta una **TUI a pantalla completa** (Bubble Tea + Lipgloss,
alt-screen) con el dashboard en vivo: hit rate con barra, contadores, uptime y las
últimas requests en tiempo real. Se refresca solo cada segundo, sin flicker.

```
╭─────────────────────────────────────────────────────────╮
│   ⚡ serve-go — dashboard                                │
│                                                         │
│  cache hit rate   [██████████████████░░░░░░] 62.5%      │
│                                                         │
│  requests         14000                                  │
│  hits             8750                                   │
│  misses           5250                                   │
│  bytes enviados   12.4 MB                                │
│  cache (len)      38 files                               │
│  cache (mem)      9.2 MB                                 │
│  root             /home/erik/misitio                     │
│  uptime           3m20s                                  │
│                                                         │
│  logs recientes                                          │
│  › GET /index.html 200 42µs                              │
│  › GET /app.css 200 18µs                                 │
│                                                         │
│  ⣾ sirviendo · [Enter] menú · [q] salir                  │
╰─────────────────────────────────────────────────────────╯
```

- **[Enter]** abre el menú de acciones: `[o]` abrir navegador, `[l]` listar archivos, `[c]` limpiar cache.
- **↑/↓** mueven el cursor, **Enter** ejecuta, **Esc** vuelve al dashboard.
- Los atajos (`o`, `l`, `c`) también funcionan directo desde el dashboard.
- **q** (o **Ctrl+C**) sale y cierra el server. **`-no-menu`** corre sin TUI (CI / scripts).

### Por qué Bubble Tea

Los logs de request del server salen por un **canal** y los renderiza la TUI, en vez
de escribir a stdout desde las goroutines del server. Así no hay dos escritores
compitiendo por la pantalla (que en la versión anterior desincronizaba el panel al
abrir una imagen / cachear un archivo). El alt-screen de Bubble Tea además elimina el
flicker del redibujo por cursor relativo.

### Panel de stats

El panel se refresca cada segundo con los contadores reales (requests, hits,
misses, bytes, cache). Hit rate con barra ascii y colores: verde ≥80%,
amarillo 50-80%, rojo <50%.

### Listing de directorios

Por defecto, igual que `npx serve`: si un directorio no tiene `index.html`, se lista su contenido con una página HTML dark (directorios resaltados, link `../`). Para desactivarlo: `-no-browse`.

### Endpoints internos

| Endpoint | Descripción |
|----------|-------------|
| `GET /__serve/health` | healthcheck (para monitores/otro servicio) |
| `GET /__serve/stats` | métricas en JSON: hits, misses, bytes, cacheLen, root |

## Comportamiento

- **index.html**: si un directorio lo tiene, se sirve automáticamente (`/sub/` → `/sub/index.html`).
- **304 Not Modified**: con `ETag` + `If-None-Match` (soporta `*` y listas).
- **Archivos gigantes**: si un archivo supera el tope de cache, se streamea del disco con `io.Copy` (el `Content-Length` sale del `Stat`).
- **Path traversal seguro**: `../`, `%2e%2e` y null bytes no salen del root (`400`).
- **Puerto auto**: `-p 3000` con el puerto ocupado pasa al siguiente libre.
- **Ya no se puede**: servir `.go`, `.env`, `.key`, `.pem`, `.sqlite`, `.db` (siembre 404 con `ShouldCache=false`).

## Benchmark (reproducible)

```bash
# terminal 1
./serve -p 4510 -no-menu -q -d /tmp/loadtest-site
# terminal 2
npx -y serve -l 4511 /tmp/loadtest-site
# terminal 3
go run ./cmd/loadtest -n 2000 -c 50 http://127.0.0.1:4510/page1.html
go run ./cmd/loadtest -n 2000 -c 50 http://127.0.0.1:4511/page1.html
```

`cmd/loadtest` es un cliente de carga sin dependencias: `-n` requests, `-c` concurrencia.

## Releases automáticos

`.github/workflows/release.yml` — al pushear un tag `v*`:

1. Corre los tests.
2. Cross-compila para `windows/amd64`, `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.
3. Crea el release con los 5 binarios + release notes automáticas.

```bash
git tag v1.2.0 && git push origin v1.2.0   # → release publicado solo
```

También sirve desde el botón **Run workflow** en GitHub → Actions (`workflow_dispatch`, sube los binarios como artifact).

## Tests

```bash
go test ./...        # 29 tests
go vet ./...
```

Cubren: serving + MIME types (+ mayúsculas), ETag/304, LRU evict y orden, path traversal, CORS, HEAD, 404/405, health/stats, listing de dirs (default y `-no-browse`), stream de archivos mayores a la cache, concurrencia (200 requests paralelos + stats sin pérdidas), panel de stats (formato, colores por hit rate, `-no-color`) y TUI Bubble Tea (render del dashboard, `q`/Ctrl+C salen, Enter abre menú, Esc vuelve, navegación ↑/↓, ejecución por atajo, acumulado+recorte de logs, error del server).

## Licencia

MIT
