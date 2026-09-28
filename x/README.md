# x — OpenTofu extensions

`x` is a set of OpenTofu extensions: a stable library facade over OpenTofu's
internal packages, for programs that consume OpenTofu as a Go library rather
than as a CLI. It re-exports selected internal types and wraps selected
internal functionality behind a small, deliberate API surface, so that library
consumers do not couple themselves to internal package layouts.

## Packages

| Package | Purpose |
| --- | --- |
| `x/addrs` | The address vocabulary: resources, modules, instance keys, resource modes, reference subjects (including `caller`), targets, providers and provider configurations |
| `x/backend` (+ `x/backend/init`) | State-storage backends and the backend registry |
| `x/builtinproviders` | Built-in providers (`terraform_data`) served as go-plugin provider servers, and an action-only built-in provider served from a host's own definitions |
| `x/cliconfig` | Host credentials and service discovery, as the CLI assembles them |
| `x/configs` | Configuration loading and parsing, module installation, schema and block types, variables files, reference extraction |
| `x/encryption` | State encryption configuration |
| `x/jsonplan` | The canonical `tofu show -json` plan marshaller and its document types |
| `x/jsonstate` | The canonical `tofu show -json` state marshaller and its document types |
| `x/lang` | HCL evaluation through a caller-implemented `Data`: expressions, blocks (with `dynamic` expansion), references, value marks, repetition meta-argument checks |
| `x/objchange` | Proposed-new and provider-less planned values, schema-driven sensitivity marks, the legacy-SDK refresh repair |
| `x/plans` | Plan types, change actions and reasons |
| `x/providers` | Provider installation (registry + filesystem mirrors) |
| `x/refactoring` | `moved` and `removed` statements |
| `x/state` | State types, state managers, `check_results` |
| `x/statefile` | State file reading/writing |
| `x/tofu` | Aggregate provider-schema sets for the JSON marshallers |

## Design rules

- Packages under `x/` expose **type aliases** and **thin wrappers** — no
  behavioral forks of the internals they cover. A wrapper may restate a
  sequence the CLI performs when a library host has to do the same thing and
  the CLI's own copy is unreachable; its doc comment names the CLI function it
  mirrors.
- The API works in terms of exported types (`cty.Value`, `hcl.Body`,
  `configschema.Block` via the `x/configs` aliases), so callers never need to
  import an `internal/` path.
- **One home per identifier.** A name is exported from exactly one package;
  every address type lives in `x/addrs`, and other packages' signatures use it.
- **Surface follows use.** An identifier is added when a consumer needs it,
  when the next piece of planned consumer work needs it, or to complete a
  sub-API that is already nearly mirrored (an enum missing one member, a block
  vocabulary missing one type). Surface no consumer uses is removed at the next
  consolidation of the patch series; members of a kept enum, and types that
  appear in a kept signature, stay.
- **No new dependencies.** Every external import of `x/` is already in this
  module's graph, so the directory stays purely additive.

## Downstream patches to existing packages

`x/` needs a handful of changes to OpenTofu's own packages. They are the only
part of this fork an upstream rebase can conflict with, and each is kept to
additions wherever the change allows it.

| Patch | Files | Why | Used by |
| --- | --- | --- | --- |
| `go.mod`: golang.org/x/* catch up with the go1.27 toolchain | `go.mod`, `go.sum` | x/net v0.54.0's http2 does not build under go1.27 | the module itself |
| configs: Terraform 1.14 action blocks and action_trigger | `internal/configs/{action,module,parser_config,resource}.go` | Parse `action` blocks, `lifecycle.action_trigger` (events, actions, on_failure, condition; refused on a data or ephemeral resource, in Terraform's words), and an address-targeted top-level `action_trigger` | `x/configs` |
| addrs, lang: the "caller" object | `internal/addrs/{caller,parse_ref}.go`, `internal/lang/{eval,scope}.go` | `caller` inside an action configuration evaluated for an action_trigger | `x/addrs`, `x/lang` |
| plans: action invocations and deferred changes, in memory only | `internal/plans/{action_invocation,changes,changes_src}.go` | Carriers for planned invocations and a change's deferral reason; neither is serialized to a plan file | `x/plans`, `x/jsonplan` |
| jsonplan: deferred_changes and action_invocations | `internal/command/jsonplan/*` | Render those carriers into the plan document | `x/jsonplan` |
| configload: read configuration through a provided filesystem | `internal/configs/configload/{loader,module_mgr}.go` | `Config.FS` / `Config.FSCanInstall` | `x/configs` |
| refactoring: the move-statement graph check as its own entry point | `internal/refactoring/move_validate.go` | Report a cyclic `moved` graph where `ApplyMoves` silently declines | `x/refactoring` |
| states/remote, cloud: a state manager reports its snapshot's version | `internal/states/remote/state.go`, `internal/cloud/state.go` | `SnapshotMeta.TerraformVersion` for the remote-backend family | `x/state` |
| addrs: "turf" implies its built-in provider | `internal/addrs/provider.go` | A `turf_*` type implies `terraform.io/builtin/turf` with no `required_providers` entry, as `terraform_data` implies the `terraform` one | `x/configs`, `x/builtinproviders` |

On the `main` line two of these are absorbed upstream and dropped: the
`go.mod` bump (upstream is ahead) and `ValidateMoveStatementGraph` (added
upstream with the same signature).

## Maintaining the series

The fork carries one commit per concern, in a fixed order: the `go.mod` bump,
then the patches above, then one commit per `x/` package group in import order
(`addrs` → `configs` → `lang` → `plans`/`objchange` → `state` → `tofu`/`jsonplan`/`jsonstate`
→ `refactoring` → `backend`/`cliconfig`/`providers` → `builtinproviders`). Every
commit builds and vets on its own.

Two lines carry the same series under the same subjects: `fork/<release>` off
the latest patch release of a release branch, and `fork/main` off upstream
`main`. `git range-diff fork/<release>...fork/main` is therefore exactly the
list of main-only adaptations.

- **Rebasing.** Rebase the series onto the new base and resolve each conflict
  or compile break **in the commit that owns it** — amend it, or commit a
  `fixup!` for it and `rebase --autosquash` — never as a trailing commit. Drop a
  patch upstream has absorbed, and record that here.
- **Changing the facade.** Land the change as a `fixup!` of the owning `x/`
  commit, with its test, on both lines.
- **Tags.** A consumable point on a release line is tagged
  `v<upstream-version>-turf.<N>` (Go semver: a release version starts a
  prerelease with a dash), with `N` restarting at 1 when the base moves.
  Consumers pin the tag.
- **Commit text** says "downstream", so the patches read cleanly if proposed
  upstream.
- **Tests.** The fork's own CI does not run on `fork/*` branches. Run
  `go test ./x/...` plus every patched package listed above.

## License

MPL-2.0, per this repository's `LICENSE`. Files under `x/` are original
additions except where a file's header notes adapted code.
