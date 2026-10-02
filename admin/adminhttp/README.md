# HTTP admin binding

`adminhttp.NewHandler(Config)` serves the independent admin contract from
caller-specific `admin.Registry` declarations. It returns an `http.Handler`:
the host mounts it on its own mux, listener and middleware. It starts no server,
goroutine, poller or timer. The root envelope packages remain independent of
admin; this is the module's only HTTP-importing package.

All three host callbacks are required, with no default allow policy:

- `Authorize(request, resource)` authenticates and authorizes every requested
  manifest/group/observation/operation, before lookup or backend access. The
  host supplies actor and selected app context through its existing request
  middleware. Unknown resources still pass through authorization first.
- `Resolve(request)` supplies an immutable, caller-filtered registry for that
  selected context. It performs metadata selection, not probes or mutations.
  Omit groups/resources/capabilities the caller cannot see; caller-visible
  declaration changes need a new revision. Each resource call still authorizes.
- `ProtectCommand(request)` enforces host CSRF policy for every POST, including
  validate. A token-only host may explicitly supply a no-op after determining
  that its credential policy makes that appropriate. The kit chooses no policy.

`BasePath` is the canonical deployment prefix used by the declaration builder,
not a path to strip. `/sysop` serves `/sysop/admin/manifest` and the corresponding
declared endpoints. Mount the handler on that subtree without StripPrefix.
Discovery verifies that returned declarations match the mounted prefix; mismatch
is 503. Encoded paths, traversal, queries and noncanonical paths are rejected,
not normalized or redirected. Only declared GET read/observation and POST
validate/update/reset commands are available. Wrong methods are malformed
requests (400). Unsupported operations on known groups return 501; unknown
resources return 404 after authorization. No lifecycle endpoint exists.

All responses are `application/json` and `Cache-Control: private, no-store`.
Discovery is metadata-only: no backend reads, probes or writes. Observation
callbacks receive request context and must be cheap/cached, sanitized and
read-only. Returned data is checked before emission. Unhealthy is an ordinary
completed 200 health observation; unavailable callbacks/invalid data return 503.
A missing scalar sample is explicit null, never a synthetic zero.

## Commands and preconditions

Bodies must be UTF-8 application/json (optional UTF-8 charset). The default
body limit is 64 KiB; hosts can set `MaxBodyBytes`. Oversized/malformed bodies,
unknown top-level keys, duplicate JSON members (including escaped equivalents),
null containers/scalars, nested non-scalars, invalid types, duplicates in unset
or overlapping set/unset are rejected as 400. Isolated Unicode surrogate
escapes are rejected instead of being silently replaced in secrets or strings.
Empty text, false and zero remain typed values. No sentinel or coercion.

Validate/update require exactly `{revision, set, unset}`; reset requires exactly
`{revision, keys}` and translates to set:{} / unset:keys. Both containers are
required even when empty. Authorization and command protection precede body
decoding; malformed JSON/command structure precedes revision checking. Declaration
revision is checked before group keys/types and current effective permissions,
then ETag is checked. A decoded command with a stale declaration revision returns
409 even if it names an unknown group key; with a current revision that key
returns 400. Both stale revisions and stale values return 409, not 412.
Permission failures return 403.

Update/reset require exactly one concrete strong read ETag in If-Match:
missing/empty is 428, wildcard/multiple/malformed is 400, weak/nonmatching is 412.
No wildcard overwrite, optimistic save or automatic retry. Validate needs a
matching declaration revision but no If-Match. It returns HTTP 200
`{valid, errors}` for a completed valid or invalid check; mutations failing
complete schema/semantic validation return 422. Refetch/reconcile after
409/412 rather than overwriting drafts or repeating a mutation automatically.

## Transaction boundary

The mutation handler enters `GroupBackend.WithTransaction` ONCE. The host must
invoke its callback exactly once synchronously; replay is rejected. Inside the
host's serializable/CAS boundary the handler:

1. Reads current desired configuration, authoritative revision, permissions and
   opaque version; checks the command, revision and If-Match.
2. Calls Resolve for a complete private candidate (including retained secrets
   and fallbacks); applies complete profile and host semantic validation.
3. Calls Stage to prepare the whole group and resulting version, without
   committing or applying/restarting anything.
4. Verifies staged desired values match the validated candidate, checks the
   core's staged response/restart projection, and serializes the redacted
   response before allowing commit. Callback error or cancellation rolls back.
5. Returns that consistent resulting snapshot and ETag only after the host
   reports successful atomic commit. Commit failure gets no staged body/ETag.

Read/Current views must be detached and consistent. Resolve/Preview candidates
carry the BASE opaque version, not a speculative new committed version;
Stage supplies the actual resulting version. Validate uses Read and
side-effect-free Preview; if Preview's base changed, it returns 503 rather
than claim completed validation based on inconsistent permissions. That check
is advisory, never a permit to skip mutation revalidation. The host must
invalidate versions for changes in every enabled layer, source, permission and
apply metadata, including writes outside this handler. Unsupported atomic
stores must not advertise mutation capabilities. A lib mutex does not make
DB/keychain writes atomic. No production adapter or storage is supplied here.

The resulting snapshot's strong ETag is used on success; a true no-op retains
it. Only this command's changed desired values/conservative need contribute to
restart_required/apply_targets. Outstanding pending work stays in per-field
apply_state. True + empty targets, guessed/omitted declared targets and invalid
host projections fail before commit. A no-op after an earlier pending save
returns false/[] while retaining pending_restart. Storage, live application,
restart verification and audit persistence remain entirely host-owned.

Private candidates are never JSON responses. Secret records expose presence
and safe metadata only. Omitted set keys keep secrets; explicit empty text
replaces them subject to constraints; unset only removes overrides and must
leave a valid complete configuration. Defaults never initialize or persist.

Errors use `{error:{code,message,errors?}}`. Typed `admin.Failure` and semantic
validation messages are host-authored, trusted SANITIZED text: never include
supplied secrets or raw storage/transport errors. Untyped/unknown errors map to
fixed 503 messages. The handler does not log requests, candidates or errors.

## Verification and adoption

Recorder-based HTTP tests exercise real request routing/decoding and a simulated
transactional adapter with staged and durable stores. They cover 409/412/428
precedence, redaction/empty-secret/keep behavior, validation-preserving reset,
full fallback/semantic checks, grouped rollback on stage/commit/bad projection,
no automatic restart, no-op pending state, concurrent writes with one winner
and no retries, explicit auth/command protection, filtered discovery, strict
JSON, prefix safety and unavailable observations. Targeted race tests repeat
20 times; the repo gates cover the entire module.

Tests bind no listener and read no production credentials/storage. They prove
helper behavior under the documented adapter guarantee, not atomicity of any
app's DB/keychain. Nanite adoption is a separate follow-up; Tachyon is untouched.
There is no module tag or release in this change.
