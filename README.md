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

### Windows (recomendado: release precompilado)

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
git clone https://github.com/xErik444x/serve-go.git
cd serve-go
go build -o serve.exe .
# opcional: copiarlo al PATH
copy serve.exe $HOME\bin\
```

### Linux / macOS

```bash
git clone https://github.com/xErik444x/serve-go.git && cd serve-go
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

### Menú interactivo

Al arrancar, el CLI queda vivo con un menú:

```
  serve-go 1.1.0
  ────────────────────────────────────
  sirviendo   /home/erik/misitio
  archivos    60 (880.0 KB)
  url         http://localhost:3000
  cache       64 MB (LRU)
  ────────────────────────────────────

opciones:
  [o] abrir en el navegador
  [s] ver stats (panel con hit rate)
  [l] listar archivos servidos
  [q] salir
```

### Panel de stats ([s])

Hit rate de la cache con barra ascii y colores: verde ≥80%, amarillo 50-80%, rojo <50%.

```
  ╭──────────────────────────────╮
  │   ⚡ serve-go — stats         │
  ╰──────────────────────────────╯

  cache hit rate  [███████████████████████░] 99.9%

  requests        14000
  hits            13997
  misses          3
  bytes enviados  107.2 MB
  cache (len)     1 files
  cache (mem)     7.8 KB
  root            /home/erik/misitio
```

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

Cubren: serving + MIME types (+ mayúsculas), ETag/304, LRU evict y orden, path traversal, CORS, HEAD, 404/405, health/stats, listing de dirs (default y `-no-browse`), stream de archivos mayores a la cache, concurrencia (200 requests paralelos + stats sin pérdidas), panel de stats (formato, colores por hit rate, `-no-color`).

## Licencia

MIT
