# Admin contract core

`admin` builds the approved admin manifest v1 from a single app declaration.
It is independent of the envelope type catalog and `Envelope.V`, and the
contract version is independent of the module's release version. This package
imports no HTTP code. The optional [admin/adminhttp](adminhttp/README.md)
binding supplies a host-mounted handler; the core alone serves no endpoints.

Declare `Definition{App, Revision, BasePath, Groups, Health, Stats}`, then call
`New`. A group's ordered `Fields` derive its schema properties, required list
and field flags together; capability booleans derive exactly the corresponding
GET read / POST validate / POST update / POST reset endpoint declarations.
Group and resource arrays retain declaration order. JSON object keys are
serialized deterministically in lexical order. `BasePath` is a deployment
prefix: `/sysop` produces `/sysop/admin/settings/<group>`. Empty prefix uses
`/admin`. No unsafe URL, query, fragment, escaping or traversal is accepted.
`Registry.Manifest()` returns detached metadata and never invokes a backend or
observation callback. Unsupported settings/health/stats/series/diagnostics
arrays are explicitly empty. Series and diagnostics are not implemented.

Only `scope: {kind: "app", id: app.id}` is supported. Other contexts are
rejected, not relabeled. This helper creates no operator role or authorization
policy. The host must filter discovery and authorize each call and subject.
Cookie-backed mutations need the host's CSRF protection. There is no implicit
public or unauthenticated admin surface.

Use `String`, `Boolean`, `Integer` and `Number` to construct `Scalar`s. They
preserve empty text, false and zero; null and the zero `Scalar` are invalid.
Numbers must be finite; integers must be within ±9007199254740991. Decimal JSON
integrality is checked before floating-point rounding, so
`1.0000000000000001` cannot become an integer setting. `Scalar.Value()` is for
trusted host code only. Schemas are flat scalars with required, enum, numeric
bounds, code-point string lengths, patterns and annotation defaults. A default
never initializes or persists a missing value. Secrets must be strings and
cannot have defaults or enums (no examples annotation is exposed). Noneditable
fields require a reason; restart fields require an explicit target, and other
fields cannot have a target. Do not put secret material in any label/annotation.

## Private candidates and public snapshots

The host supplies `State` with a record for every field. `ResolvedValue.Value`
contains desired configuration privately, including retained secrets. Present
records require a typed value and a safe source kind/label from an enabled
layer. Absent records omit value/source; missing required values remain
representable with validation errors for recovery. Effective permission cannot
exceed the declaration; read-only records require a safe reason. `apply_state`
is always active, pending_restart or unknown. A saved row does not prove that
it is running: use unknown unless application has been verified.

Private state/value JSON marshaling fails, and ordinary/Go formatting redacts
those wrappers. Never log private candidates, individual Scalars or command
bodies. Call `Group.Project(ctx, state)` to obtain a public `Snapshot`: secret
records expose only `secret_present` plus presence/source/editability/override/
apply metadata, never a value/hash/length/prefix/mask. Missing nonsecrets omit
value; missing values omit source. Source labels are sanitized descriptions,
not private paths, credential locators or values. The host owns their safety.
Returned projections are detached from the privately resolved state.

`Validate` checks the complete configuration including kept secrets;
`ValidateCandidate` additionally calls the group's optional, side-effect-free
`SemanticValidation` callback with a detached private view. Apps must supply
this callback for any semantic/cross-field rules their settings require.
Errors are safe `{path, code, message}` records. Paths name a declared field or
are empty for a group error; errors never include rejected values. A raw
callback failure becomes a fixed unavailable error. The host authors safe
semantic messages, and must not echo transport/storage errors or secrets.

## Host adapter and commit boundary

`GroupBackend.Read` returns a consistent view; `Preview` resolves a command
without persistence or external writes. Resolution, precedence, fallbacks and
semantic meaning belong to the host. `CheckChanges` verifies both command
containers, typed keys, duplicates/intersection and current permissions. It
cannot validate a fallback; the host must resolve the full candidate and run
`ValidateCandidate`. An explicit empty secret string is a replacement; omitted
keys mean keep, unset removes only the named override. Unset never erases other
layers; required values must still have a valid fallback. No mask sentinel.

`WithTransaction` must supply `GroupTransaction.Current`, `Resolve` and `Stage`
under one host-controlled serializable transaction/CAS boundary. Resolve has
no writes. Stage only prepares/stages persistence and returns `Staged` with
resulting state, changed keys and host-supplied restart metadata. Validate the
complete candidate, stage, then call `Group.UpdateResult(ctx, before, staged)`
BEFORE returning success from the transaction callback. Callback errors must
roll back; success commits the whole group and its opaque version atomically.
The staged view must match what is committed. Stage cannot call live apply or
restart. The optional adminhttp binding supplies HTTP orchestration/precondition checks.

All other app writers, enabled env/file/default layers and effective permission
changes must participate in invalidation/concurrency control. A lib mutex
cannot make a database and keychain atomic. Do not advertise update/reset unless
the adapter guarantees atomic durability across all touched stores. Validation
and discovery cannot write anything. Transactions are per group, not across
wizard groups. No retry, listener, goroutine, secret store or lifecycle operation
is supplied by this package.

`State.Version` is a host-issued opaque generation, never a hash of a secret or
secret-bearing snapshot. `State.ETag()` quotes it as a strong tag. Every desired
value, source, override, permission or apply metadata change invalidates it.
`State.Revision` identifies the declaration version, not the values. A command
response uses the resulting snapshot's ETag: a truly unchanged snapshot keeps
its ETag. The host must change the declaration revision when caller-visible
schema, capabilities, endpoints or declarations change.

`UpdateResult` checks declared/actual changed keys, resulting version and the
host's restart projection. `restart_required` and `apply_targets` describe only
THIS command's changed desired values and any conservative need caused by that
change. Targets are never guessed. True with empty targets, duplicate/invalid
IDs, or omitted affected declared targets is rejected before commit. Changed
restart fields must be pending_restart. A no-op after an earlier pending save
returns changed_keys [], restart_required false and apply_targets [], while its
snapshot retains pending_restart. Outstanding lifecycle state is not folded
into command metadata. The host separately verifies application/lifecycle;
no restart or application callback is exposed.

## Patterns

The portable subset allows printable ASCII literals, character classes/ranges
(including negated classes), capturing groups, alternation, start/end anchors,
and greedy `?`, `*`, `+`, `{n}`, `{n,}`, `{n,m}` quantifiers. Explicit repetition
bounds are limited by Go's compiler to 1000. Literal metacharacters may be
backslash-escaped: `\ . ^ $ | ? * + ( ) [ ] { }`. Use `[.]` or `\.` for a
literal dot. Class elements use printable ASCII; literal brackets are escaped.

Definition construction rejects wildcard dot (different line terminators),
lookaround, backreferences, inline flags/noncapturing groups, lazy quantifiers,
shorthand classes (`\d`, `\w`, `\s`), POSIX/Unicode/engine-specific classes or
escapes, non-ASCII patterns, invalid nesting and unsupported syntax. Examples:
`^https?://` and `^[a-zA-Z0-9._-]{1,20}$` are supported; `.*`, `(?i)x` and
`\w+` are not. String values remain Unicode, and lengths count code points.

Equivalence with kit-settings' JavaScript `RegExp(pattern, 'u')` is documented,
not machine-verified. This PR has only Go tests; it adds no cross-repo pattern
corpus or import-boundary gate. A shared engine check needs a separate decision.

## Observation and error surfaces

Health/stat declarations pair metadata and a context-aware read callback once.
Callbacks must be read-only, cheap or cached and authorized before invocation.
`HealthObservation.Validate` accepts unhealthy/degraded/unknown as completed
observations; unavailable fetches remain failures. It rejects unknown statuses,
missing check containers and duplicate check IDs. `StatObservation.Validate`
requires a finite scalar or explicit null sample, valid units, and ratio 0–1 /
percent 0–100 bounds. Times must be UTC RFC3339. No synthetic zero/healthy data,
probe fan-out, polling timer, repair or storage is provided.

`ErrorResponse` wraps `Failure` as `{error:{code,message,errors?}}`.
`Failure.StatusCode()` covers 400/401/403/404/409/412/422/428/501/503 without
importing HTTP. The optional adminhttp binding enforces authorization, well-formed
command, revision, then ETag in that order. Validate completes with HTTP 200 for
valid or invalid; mutation validation failure is 422. Unknown host failures
must map to fixed sanitized 503 messages, not their raw error text.

Nanite adoption and Tachyon config-ops are outside this core PR. No app code,
module tag or release is included.
