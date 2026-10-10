# serve-go ⚡

Servidor estático ultrarrápido en Go — alternativa directa a `npx serve .`, sin Node, sin npm install, un solo binario.

**Benchmark real (Pi 5, ARM64): serve-go 23,066 rps vs npx serve 659 rps — 33x más rápido.**

## Por qué

| | npx serve | serve-go |
|---|---|---|
| Latencia en frío (arranque + npx resolve) | ~2-4 s | ~10 ms |
| RPS (2000 req, c=50) | ~659 | ~23,000 |
| Dependencias | Node + npm + 30 paquetes | ninguna (binario único) |
| Memoria del proceso | ~50-80 MB (Node runtime) | ~8 MB |
| ETag/304 automático | ❌ (usa Last-Modified) | ✅ |

## Instalación

### Windows
1. Bajá `serve.exe` de [Releases](../../releases) (o compilalo: `go build -o serve.exe .`)
2. Ponaló en una carpeta que esté en tu `PATH`, por ejemplo:
   ```powershell
   mkdir $HOME\bin
   copiar serve.exe $HOME\bin\serve.exe
   # agregar al PATH (permanente):
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";$HOME\bin", "User")
   # cerrar y reabrir la terminal
   ```
3. Listo:
   ```powershell
   cd C:\mi\proyecto
   serve
   # http://localhost:3000
   ```

### Linux / macOS
```bash
go install github.com/xErik444x/serve-go@latest
# o clonar y compilar:
git clone https://github.com/xErik444x/serve-go && cd serve-go
make build && sudo cp serve /usr/local/bin/
```

## Uso

```
serve [flags]
```

| Flag | Descripción |
|------|-------------|
| `-p 3000` | puerto (default: 3000, si está ocupado busca el siguiente libre) |
| `-d ./dist` | directorio a servir (default: `.`) |
| `-o` | abrir el navegador al arrancar |
| `-cors` | habilita CORS `*` (útil para frontends que consumen APIs) |
| `-L` | escuchar en `0.0.0.0` (accesible desde la LAN) |
| `-b` | listing de directorios sin `index.html` |
| `-no-cache` | deshabilitar la cache LRU (dev) |
| `-cache 64` | tope de la cache en MB (default 64) |
| `-q` | sin logs por request |
| `-no-menu` | sin menú interactivo (CI / scripts) |
| `-v` | versión |

### Menú interactivo

Al arrancar, el CLI queda vivo con un menú en la terminal:

```
opciones:
  [o] abrir en el navegador
  [s] ver stats (hit rate de la cache)
  [l] listar archivos servidos
  [q] salir
```

### Endpoints internos

- `GET /__serve/health` → `ok` (para healthchecks)
- `GET /__serve/stats` → JSON con hits/misses/bytes/cacheLen

## Benchmark

Duplicá el test con el binario `loadtest` incluido:

```bash
# terminal 1: serve-go
./serve -p 4510 -no-menu -q -d /tmp/loadtest-site

# terminal 2: npx serve
npx serve -l 4511 /tmp/loadtest-site

# terminal 3: medir
go run ./cmd/loadtest -n 2000 -c 50 http://127.0.0.1:4510/page1.html   # serve-go
go run ./cmd/loadtest -n 2000 -c 50 http://127.0.0.1:4511/page1.html   # npx serve
```

Resultados en Pi 5 (ARM64, Go 1.26, Node 22):

```
npx serve:  requests: 2000 | ok: 2000 | 3033 ms | rps: 659  | avg: 1.52 ms
serve-go:   requests: 2000 | ok: 2000 |  115 ms | rps: 17280 | avg: 0.06 ms  (cold)
serve-go:   requests: 2000 | ok: 2000 |   86 ms | rps: 23066 | avg: 0.04 ms  (warm)
serve-go:   requests: 10000 | ok: 10000 | 458 ms | rps: 21833 (stress c=100, 0 fails)
```

## Cómo está hecho

- **Cache LRU en memoria** con tope por bytes (64 MB default): el hot path sirve directamente del heap, sin `syscall` por request. `container/list` + `map` — cero deps.
- **ETag débil** (sha1 truncado) + `If-None-Match` → 304 para clientes que repiten requests.
- **Path traversal seguro**: `filepath.Rel` contra el root — `../etc/passwd` y encodings `%2e%2e` devuelven 400.
- **Content-types por extensión** (tabla propia, sin `mime.TypeByExtension` ni syscalls).
- **Archivos gigantes**: si el archivo excede la cache, se streamea directo con `io.Copy` (buffer acotado).
- **Puerto auto**: pide un puerto efímero si el 3000 está ocupado.
- **Open browser**: `rundll32` (Windows), `open` (macOS), `xdg-open` (Linux) — sin deps.

## Tests

```bash
go test -v ./...   # 15 tests: serving, content-types, ETag 304, LRU evict, traversal, CORS, HEAD, stats, menú
```
