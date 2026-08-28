# x — OpenTofu extensions

`x` is a set of OpenTofu extensions: a stable library facade over OpenTofu's
internal packages, for programs that consume OpenTofu as a Go library rather
than as a CLI. It re-exports selected internal types and wraps selected
internal functionality behind a small, deliberate API surface, so that library
consumers do not couple themselves to internal package layouts.

## Packages

| Package | Purpose |
| --- | --- |
| `x/addrs` | Resource addressing: addresses, references, instance keys |
| `x/backend` (+ `x/backend/init`) | State-storage backends and the backend registry |
| `x/configs` | Configuration parsing, module loading, schema types, body decoding, reference extraction |
| `x/encryption` | State encryption configuration |
| `x/jsonplan` | The canonical `tofu show -json` plan marshaller |
| `x/lang` | HCL expression evaluation: scopes, evaluation, validation, value marks, repetition meta-argument checks |
| `x/objchange` | ProposedNew computation and schema-driven sensitivity projections |
| `x/plans` | Plan types, change actions, and rendering helpers |
| `x/providers` | Provider installation (registry + filesystem mirrors) |
| `x/state` | State types and helpers |
| `x/statefile` | State file reading/writing |
| `x/tofu` | Aggregate provider-schema sets for the JSON marshallers |

## Design rules

- Packages under `x/` expose **type aliases** and **thin wrappers** — no
  behavioral forks of the internals they cover.
- The API works in terms of exported types (`cty.Value`, `hcl.Body`,
  `configschema.Block` via the `x/configs` aliases), so callers never need to
  import an `internal/` path.
- New surface is added only when a library consumer needs it, and stays as
  close to the shape of the underlying internal API as practical.

## License

MPL-2.0, per this repository's `LICENSE`. Files under `x/` are original
additions except where a file's header notes adapted code.
