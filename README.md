# cpa-plugin-combos

A [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) plugin that exposes
named model fallback chains ("combos") as ordinary models.

A combo is an ordered list of models. A request for `combo/<name>` is routed to
the plugin, which tries each target in order and returns the first success. A
combo is reachable by name from any client that speaks the OpenAI, Claude,
Gemini, or Responses protocol, and is advertised in `GET /v1/models` as
`combo/<name>`.

## Data model

```json
{
  "version": 1,
  "combos": [
    {
      "name": "smart",
      "description": "codex then glm",
      "targets": [
        { "provider": "codex", "model": "gpt-5.6-terra" },
        { "provider": "openai-compatible", "model": "glm-5" }
      ]
    }
  ]
}
```

Order is priority. `provider` is optional; when omitted the host resolves the
model by name. When present it is matched against the providers the host
advertises, so `glm` resolves to `openai-compatible-glm`.

## Management

The plugin registers a `Combos` menu in CPA Manager Plus and serves its own UI
at `/v0/resource/plugins/combos/combos`. CRUD is available at
`/v0/management/combos/api` (GET, POST, PUT, DELETE).

A new combo is callable as soon as it is created. It joins the advertised model
list on the next CPA configuration change or restart.

## Build

```sh
make            # builds dist/combos.so and verifies the exported C ABI symbol
```

Cross-compile for arm64 with a cgo toolchain:

```sh
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
  CC=aarch64-linux-gnu-gcc \
  go build -trimpath -buildmode=c-shared -o combos-linux-arm64.so .
```

## Releases

`.github/workflows/release.yml` builds on every push and pull request, and
publishes release assets when a `v*` tag is pushed. Every artifact is checked
for the exported `cliproxy_plugin_init` symbol before it ships.

```sh
git tag v0.1.0
git push origin v0.1.0
```

Assets:

```text
combos_<version>_linux_amd64.tar.gz
combos_<version>_linux_arm64.tar.gz
checksums.txt
```

## Install

Copy the `.so` into `<plugins-dir>/<os>/<arch>/` and enable it:

```yaml
plugins:
  enabled: true
  dir: "/CLIProxyAPI/plugins"
  configs:
    combos:
      enabled: true
      priority: 50
```

Set `COMBOS_TRACE=1` in the host environment for dispatch-level logging.
