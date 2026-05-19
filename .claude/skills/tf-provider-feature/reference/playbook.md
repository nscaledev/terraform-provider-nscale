# Nscale Terraform provider playbook

> Opinionated team consensus on how to build resources and data sources in this provider. Synthesised from external research and the failure modes we've actually hit. Pick the recommendations here unless you have a specific reason to deviate; if you deviate, leave a comment in code explaining why.

## Executive summary — five things that matter most

1. **Never marshal Terraform values straight into upstream openapi-generated structs when zero values are semantically meaningful.** Go's `encoding/json` drops bare `bool false`, empty strings, and `0` under `omitempty`. openapi-generator-go cannot mark a primitive non-pointer field as both required and JSON-omit-empty-safe (see [openapi-generator#8841](https://github.com/OpenAPITools/openapi-generator/issues/8841)). Wrap upstream models in a provider-side DTO that uses `*bool`/`*string` for any field whose absent-vs-zero distinction matters. This is the root cause of the "Provider produced inconsistent result after apply" class of bug we ran into on `allow_create_bucket`.
2. **Treat `Sensitive: true` as redaction in CLI output only, never as state protection.** State is plaintext JSON. Write-once secrets like `access_key.secret` need: `Sensitive: true` + stash-on-Read + import warning + documented "Handling the secret" prose. See §2.3.
3. **Use `retry.StateChangeConf` from `terraform-plugin-sdk/v2/helper/retry` as the shared waiter primitive for new async resources.** HashiCorp staff have explicitly confirmed it's framework-compatible. Our current `nscale.CreateStateWatcher` works; new resources should use `StateChangeConf` so we converge on AWS-style `Pending`/`Target`/`Refresh`/`Timeout`/`Delay`/`MinTimeout` semantics with explicit error states. See §2.2.
4. **Explicit timeouts on every long-running operation; framework has no implicit default.** Per HashiCorp Plugin Development forum: "The Terraform Plugin Framework does not have a default timeout that is automatically set by the SDK like in the SDKv2." Use `timeouts.Block` with resource-specific defaults documented in prose. See §2.2 for the default table.
5. **Avoid `Optional+Computed` unless the API has a stable default it will return exactly on Read.** Otherwise either `Required`, `Computed`, or split into `requested_*`/`effective_*`. `Optional+Computed` on a field the API may rewrite produces "planned value vs actual value" surprises that look like drift and aren't.

---

## 1. Schema design

### 1.1 Attribute kinds

Pick by source of truth:

| Source of value | Setting |
| --- | --- |
| User must provide; resource cannot exist without it | `Required: true` |
| User may provide; null is meaningful (absent) | `Optional: true` |
| API computes it, user can never set it (IDs, ARNs, timestamps) | `Computed: true` |
| User may provide; if absent, API picks a stable default we want to read back | `Optional: true, Computed: true` |

The trap is the fourth row. Use `Optional+Computed` only when the API has a deterministic default and `Read` returns it exactly. If the API may rewrite a practitioner-supplied value (e.g. normalising case, applying a quota), use `Required` plus validators, or split the schema into `requested_*` and `effective_*`. Do **not** rely on `Optional+Computed` to mask server-side normalisation.

From the framework docs: "Computed only: any practitioner configuration causes the framework to automatically raise an error." That's the correct behaviour — don't try to make server-owned IDs overridable.

### 1.2 Plan modifiers

Three you will use; one you will write yourself.

```go
// internal/services/storage/object_storage_endpoint_resource.go
"id": schema.StringAttribute{
    MarkdownDescription: "The endpoint ID assigned by Nscale.",
    Computed:            true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
},
"region": schema.StringAttribute{
    MarkdownDescription: "Region the endpoint lives in. Changing this forces replacement.",
    Required:            true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
},
```

`UseStateForUnknown()` copies a known prior state value into the planned value. Apply it to every Computed-only attribute on every resource — IDs, timestamps, ARNs, project_id, region (where region is computed from a parent). Without it the plan output is full of spurious `(known after apply)` lines.

`RequiresReplace()` for immutable fields. Prefer `RequiresReplaceIfConfigured()` if the field is Optional and you don't want unconfigured drift to trigger replacement.

**Do not** use `UseStateForUnknown` to paper over upstream serialisation bugs (like the `omitempty` bool problem). That's a lie and Terraform will catch you. Fix the converter — see §1.6.

### 1.3 Nested objects, lists, sets

Pick by ordering semantics. The decision is permanent; getting it wrong forces a schema version bump.

- **`ListNestedAttribute`** when order matters or duplicates are possible (e.g. firewall rules with priority).
- **`SetNestedAttribute`** when the API treats the collection as unordered and the framework should not show diffs for reordering. **Caveat: write-only / write-once attributes cannot live inside sets.** If a nested object contains a secret, use list, not set.
- **`SingleNestedAttribute`** for fixed sub-objects (e.g. `network { vpc_id, subnet_id }`). Prefer this over `MapNestedAttribute` for fixed-shape configuration.
- **`MapNestedAttribute`** when objects are naturally keyed by a stable string (labels, named backends).

Avoid the older `schema.Block` form on new resources. Per HashiCorp's framework guidance, nested *attributes* are the convention.

Beware: defaults inside set nested attributes are broken in the framework ([terraform-plugin-framework#783](https://github.com/hashicorp/terraform-plugin-framework/issues/783)). If you want a default inside a set, restructure to a list.

### 1.4 Sensitive attributes

`Sensitive: true` redacts CLI/HCP Terraform output. It does **not** encrypt or omit the value from state. From the HashiCorp best-practices page on sensitive state: "This will prevent the field's values from showing up in CLI output and in HCP Terraform. It will not encrypt or obscure the value in the state, however."

For write-once secrets (the canonical `access_key.secret` case): `Sensitive: true` is necessary but insufficient. The full pattern is in §2.3. Do **not** adopt Terraform 1.11+ `WriteOnly` arguments for secrets yet — they aren't persisted to state at all, which breaks the stash-on-Read recovery. Revisit when we set a `>= 1.11` floor.

### 1.5 Default values

There are three places defaults can live. Pick by ownership:

1. **Server-side default** (API picks if null): model as `Optional: true, Computed: true`, leave `Default` unset, read the value back. This is correct for almost every Nscale field — our backend owns the semantics.
2. **Framework `Default`** (e.g. `booldefault.StaticBool(false)`): use only when the *provider* owns the default and the API has no opinion. Required: `Computed: true`. Example: an `auto_start` flag we invent that doesn't map to an API field.
3. **Plan modifiers**: don't. Defaults belong in `Default`, not in modifiers.

```go
"public_access": schema.BoolAttribute{
    MarkdownDescription: "Whether the bucket is publicly accessible. Defaults to `false`.",
    Optional:            true,
    Computed:            true,
    Default:             booldefault.StaticBool(false),
},
```

### 1.6 The `omitempty` bool failure mode (failure mode #1)

**The bug.** openapi-generator emits

```go
type CreateBucketRequest struct {
    Versioned bool `json:"versioned,omitempty"`
}
```

`encoding/json` omits any field whose value is the zero value when `omitempty` is set, which for `bool` means `false`. A user setting `versioned = false` plans `false`, we POST `{}`, the server defaults `versioned` to whatever it likes, we read back something different, and the framework throws "Provider produced inconsistent result after apply: was cty.False, but now cty.True" (or vice versa).

**Detection.** Two signals:

1. Acceptance test that explicitly sets a Required bool to its zero value fails on apply with "Provider produced inconsistent result". Add one of these per Required bool — see [testing.md](testing.md) §4.3.
2. Grep the generated client for `bool \`json:"[^"]+,omitempty"\`` after each codegen run. Any match is suspect for fields that aren't truly tri-state.

**Fix priority.**

1. **Patch upstream** if the API team owns the spec. The OpenAPI fix is to add `nullable: false` and remove the `omitempty`, or change to a pointer-with-explicit-encoding. File the issue and reference openapi-generator [#8841](https://github.com/OpenAPITools/openapi-generator/issues/8841).
2. **Wrap at the service boundary** for everything else. The `<name>_model.go` file already separates the Terraform model from the API DTO. Add a provider-owned request DTO that uses `*bool`:

   ```go
   // internal/services/storage/bucket_model.go
   type bucketAPIRequest struct {
       Name      string `json:"name"`
       Versioned *bool  `json:"versioned,omitempty"`
   }

   func (b bucketAPIRequest) MarshalJSON() ([]byte, error) {
       // If Versioned is non-nil, emit it explicitly so false is never lost.
       if b.Versioned == nil {
           type alias bucketAPIRequest
           return json.Marshal((alias)(b))
       }
       type explicit struct {
           Name      string `json:"name"`
           Versioned bool   `json:"versioned"`
       }
       return json.Marshal(explicit{Name: b.Name, Versioned: *b.Versioned})
   }
   ```

3. **Never** mask the symptom with `UseStateForUnknown` on a Required bool.

Pair the fix with a unit test in `<name>_model_test.go` covering both `true` and `false`, and an acceptance test step that explicitly sets `false` then runs `PlanOnly` to catch the regression.

---

## 2. State and lifecycle

### 2.1 Drift detection

Drift is anything where the API's view of an attribute differs from state. Real drift is a feature; users want to know. Spurious diffs are bugs.

Spurious diffs in this provider have three sources:

- **Server returns a field we haven't put in the model** → add the field to the model as `Computed: true` and read it.
- **Server returns a normalised version of what we sent** (lowercased region, trimmed whitespace) → normalise on the *write* side before storing in state. Do not suppress with a custom comparator; users will hate the surprise.
- **The `omitempty` bool problem** — see §1.6.

**Don't** write a custom plan modifier that suppresses a diff because two values "look equivalent." That's the SDKv2 `DiffSuppressFunc` antipattern; the framework deliberately doesn't have a direct replacement. If two strings should be considered equivalent, normalise on write.

The provider's `Read` rule:

- Normalise API responses into the Terraform model before storing state.
- Preserve unrecoverable values (write-once secrets) from prior state when the API cannot return them.
- Remove the resource from state when the remote object is gone (`resp.State.RemoveResource(ctx)`).
- Never write a value that changes the meaning of a practitioner-configured argument unless the schema marks it computed-only or you are in a documented replacement flow.

### 2.2 Async create with state watchers (failure mode #5)

Our current `nscale.CreateStateWatcher` works but lacks a Pending/Target separation — a transient `failed` looks the same as a permanent one.

**For new async resources, prefer `retry.StateChangeConf`** from `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`. HashiCorp explicitly confirms it's framework-compatible: "The helper functions detailed below are compatible with Terraform Plugin Framework based resources... While these functions currently reside in the legacy Terraform Plugin SDK repository, they are not directly tied to functionality exclusive to this library." AWS's own framework resources use it.

Target shape — drop a `<name>_wait.go` next to the resource:

```go
// internal/services/compute/instance_wait.go
func statusInstance(ctx context.Context, c *nscale.Client, id string) retry.StateRefreshFunc {
    return func() (any, string, error) {
        out, err := c.Compute.GetInstance(ctx, id)
        if errors.Is(err, nscale.ErrNotFound) {
            return nil, "", nil
        }
        if err != nil {
            return nil, "", err
        }
        return out, string(out.Status), nil
    }
}

func waitInstanceProvisioned(ctx context.Context, c *nscale.Client, id string, timeout time.Duration) (*nscale.Instance, error) {
    sc := &retry.StateChangeConf{
        Pending:    []string{"pending", "provisioning", "starting"},
        Target:     []string{"provisioned", "healthy"},
        Refresh:    statusInstance(ctx, c, id),
        Timeout:    timeout,
        Delay:      5 * time.Second,
        MinTimeout: 3 * time.Second,
    }
    raw, err := sc.WaitForStateContext(ctx)
    if v, ok := raw.(*nscale.Instance); ok {
        return v, err
    }
    return nil, err
}
```

Conventions:

- One `status<Resource>` and one `wait<Resource><Verb>` per resource per terminal state (`waitInstanceProvisioned`, `waitInstanceDeleted`).
- `Pending` lists every state we expect to transit through. Anything not in `Pending` and not in `Target` is a hard failure — exactly what we want for an unexpected `failed`.
- `Delay` is the initial wait before the first poll. `MinTimeout` is the floor between polls. 5s / 3s is a reasonable default for our control plane.

**Timeouts.** Framework has no implicit default. Use `terraform-plugin-framework-timeouts`:

```go
// In Schema()
Blocks: map[string]schema.Block{
    "timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true, Update: true}),
},

// In Create()
createTimeout, diags := plan.Timeouts.Create(ctx, 30*time.Minute) // resource-specific default
resp.Diagnostics.Append(diags...)
if resp.Diagnostics.HasError() { return }
ctx, cancel := context.WithTimeout(ctx, createTimeout)
defer cancel()
```

Default-timeout table by resource class (set per resource, document in the prose):

| Class | Create | Update | Delete |
| --- | --- | --- | --- |
| Region / config (no async) | 1m | 1m | 1m |
| Storage endpoint / access key | 5m | 5m | 5m |
| Compute instance (small) | 15m | 10m | 15m |
| Compute instance (GPU / large) | 30m | 15m | 30m |

Users override with `timeouts { create = "45m" }`. Expose `timeouts` on every resource that has a `wait*` call — no exceptions.

### 2.3 Write-once secrets (failure mode #2)

`secret` on the access-key resource is returned by Create only. The current pattern — stash on Read, re-attach to state — is correct. Make it convention.

**Schema:**

```go
"secret": schema.StringAttribute{
    MarkdownDescription: "The secret access key. Returned only on creation; cannot be read back. " +
        "If you lose this value, you must replace the resource to obtain a new one.",
    Computed:  true,
    Sensitive: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
},
```

**Create flow:**

1. Save the secret to state immediately after Create returns, *before* any state-watcher Wait, so a watcher failure does not strand the secret.
2. After the watcher settles, re-fetch via Read and re-attach the preserved secret to the final state.

**Read flow:**

```go
func (r *accessKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state accessKeyModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() { return }

    // Preserve the write-once secret across Read; the API will not return it.
    priorSecret := state.Secret

    apiObj, err := r.client.Storage.GetAccessKey(ctx, state.ID.ValueString())
    if errors.Is(err, nscale.ErrNotFound) {
        resp.State.RemoveResource(ctx)
        return
    }
    if err != nil {
        resp.Diagnostics.AddError("Reading access key", err.Error())
        return
    }

    state = NewAccessKeyModel(apiObj)
    state.Secret = priorSecret // re-attach

    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

**Import flow:** the secret will be unrecoverable. Warn, don't error:

```go
func (r *accessKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    parts := strings.SplitN(req.ID, "/", 2)
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        resp.Diagnostics.AddError(
            "Invalid import ID",
            fmt.Sprintf("Expected `<endpoint_id>/<access_key_id>`, got %q", req.ID),
        )
        return
    }
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("endpoint_id"), parts[0])...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("secret"), types.StringNull())...)

    resp.Diagnostics.AddWarning(
        "Imported secret unavailable",
        "The Nscale API does not return `secret` after creation. Terraform state will have an empty `secret`. "+
            "To restore it, destroy and recreate the resource.",
    )
}
```

Acceptance tests must use `ImportStateVerifyIgnore: []string{"secret"}`. The matching prose docs must contain a "Handling the secret" section covering safe extraction patterns (`terraform output -raw … | pbcopy`, restricted-perm files, secret-manager piping) and the state-at-rest warning. See `website/docs/r/object_storage_access_key.html.markdown` for the canonical shape.

### 2.4 Import: passthrough vs composite

Three patterns, one rule each.

- **Single ID:** `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`. Default. Use it.
- **Composite (`<endpoint_id>/<access_key_id>`):** parse manually; validate every part. Always use one delimiter (`/`) and one precise validation message.
- **Multi-attribute reconstruction:** avoid. If Read needs more than one attribute and they're not derivable from the composite ID, you have an API design problem, not a Terraform problem.

Document the format in `website/docs/r/<name>.html.markdown` under an `## Import` heading. Show the exact format string in a code block.

### 2.5 Update vs ForceNew; reject Update entirely when appropriate

If the API has no PATCH/PUT, don't write an Update that lies. Mark every mutable field `RequiresReplace()` and return an error from `Update` (it should be unreachable given the plan modifiers, but defence in depth):

```go
func (r *accessKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    resp.Diagnostics.AddError(
        "Update not supported",
        "Object storage access keys are immutable. All attribute changes force resource replacement.",
    )
}
```

This is what `access_key_resource.go` already does. Make it the default for resources without an update API.

### 2.6 Delete idempotency

Always treat 404 on Delete as success. Same for Read — call `resp.State.RemoveResource(ctx)` and return without error:

```go
func (r *bucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state bucketModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() { return }

    err := r.client.Storage.DeleteBucket(ctx, state.ID.ValueString())
    if err != nil && !errors.Is(err, nscale.ErrNotFound) {
        resp.Diagnostics.AddError("Deleting bucket", err.Error())
        return
    }
    // Framework auto-removes from state on Delete success.
}
```

From the framework `tfsdk` docs: "RemoveResource removes the entire resource from state. If a Resource type Delete method is completed without error, this is automatically called on the DeleteResourceResponse.State." That's why we don't call it manually in Delete. We do in Read.

---

## 3. Error handling and UX

### 3.1 Diagnostics text

Per HashiCorp's diagnostics docs: "Good summaries are general — they don't contain specific details about values — and concise. Good details are specific — they tell the practitioner exactly what they need to fix and how."

Convention for our REST-client errors:

```go
// internal/clients/diag.go
func APIErrorDiag(action string, err error) (summary, detail string) {
    var apiErr *nscale.APIError
    if errors.As(err, &apiErr) {
        return fmt.Sprintf("Nscale API error %s", action),
            fmt.Sprintf("HTTP %d: %s\nTrace ID: %s\n\n%s",
                apiErr.StatusCode, apiErr.Code, apiErr.TraceID, apiErr.Message)
    }
    return fmt.Sprintf("Error %s", action), err.Error()
}

// Usage:
if err != nil {
    summary, detail := clients.APIErrorDiag("creating object storage endpoint", err)
    resp.Diagnostics.AddError(summary, detail)
    return
}
```

Include status code, the API error code (e.g. `ENDPOINT_LIMIT_EXCEEDED`), the trace ID for support, and the message. **Do not** dump the full response body — that's noise and a potential information leak.

### 3.2 Retryable vs fatal

Retries belong in two places only:

1. **Inside the openapi client transport** for transient HTTP errors (5xx, network blips, 429 with `Retry-After`). Use a `RoundTripper` with exponential backoff, capped at 3 attempts.
2. **Inside `wait*` functions** for transient resource states. `StateChangeConf` handles "retry while in Pending" — don't double-wrap.

Don't sprinkle ad-hoc retry loops in CRUD methods. If you find yourself writing `for i := 0; i < 3; i++` in a resource file, push it down to the client.

Retry transport errors, timeouts, 408, 429, 5xx. Respect `Retry-After`. Optionally retry documented eventual-consistency cases (a 404 immediately after create). **Do not** retry authentication failures, authorisation failures, or validation/business-rule failures like a clean 422.

Tie retries to CRUD timeouts, not unbounded loop counts. A retry that can outlive the user's configured `create` timeout is a bug.

### 3.3 Validation: when to validate where (failure mode #3)

Three layers, used in this order:

1. **Schema validators** (`stringvalidator.LengthBetween`, `int64validator.OneOf`, regex). For anything the user can know without an API call. From the framework validation page: "Prefer plan-time validation over apply-time validation."
2. **`ConfigValidators` / `ValidateConfig`** for cross-attribute rules ("if `mode = external`, `endpoint_url` is required").
3. **API errors** for global constraints we can't see from the config alone (e.g. "one object-storage endpoint per project per region"). Surface the 422 unmodified — see §3.1.

```go
// internal/services/storage/object_storage_endpoint_resource.go
func (r *objectStorageEndpointResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
    return []resource.ConfigValidator{
        resourcevalidator.Conflicting(
            path.MatchRoot("public_url"),
            path.MatchRoot("private_url"),
        ),
    }
}
```

**Don't** add provider-side caching of "one endpoint per project per region" — you'd be reimplementing the API's job, and any race condition will leave you wrong. Push that to the server and surface its 422. Document the constraint in the resource prose so users aren't surprised.

Provider-side validators are appropriate when:

- The rule is purely structural (regex, length, enum).
- The rule depends only on the config (cross-attribute consistency).
- A bad value would cost a non-trivial amount of money or time to find out at apply time.

### 3.4 `AddError` vs `AddWarning`

- **Error** if the operation cannot proceed or the state is now suspect. Halts execution.
- **Warning** if the user can ignore the message and still get a working result.

Use warnings sparingly. The two right uses in this codebase:

- Import warnings when an attribute can't be recovered (the `secret` case).
- Deprecation warnings when an attribute or resource is being removed in the next major.

Don't use warnings for "the API took longer than expected" or "we retried." Use `tflog.Info` / `tflog.Warn` (structured logs); they're visible with `TF_LOG=INFO` and don't pollute the CLI.

### 3.5 Multi-URL service routing (failure mode #4)

Three base URLs (`region.*`, `compute.*`, `storage.*`). Convention:

- **Env vars are the source of truth**: `NSCALE_REGION_SERVICE_API_ENDPOINT`, `NSCALE_COMPUTE_SERVICE_API_ENDPOINT`, `NSCALE_STORAGE_SERVICE_API_ENDPOINT`, plus auth (`NSCALE_SERVICE_TOKEN`, `NSCALE_ORGANIZATION_ID`, `NSCALE_PROJECT_ID`).
- **Provider attributes override env vars** and exist mainly for testing and dual-stack setups: `region_service_api_endpoint` etc.
- **Defaults are baked into the provider** for production endpoints. Users set nothing in the common case.

Precedence in `provider.Configure`:

```go
regionURL := os.Getenv("NSCALE_REGION_SERVICE_API_ENDPOINT")
if !cfg.RegionServiceAPIEndpoint.IsNull() {
    regionURL = cfg.RegionServiceAPIEndpoint.ValueString()
}
if regionURL == "" {
    regionURL = DefaultNscaleRegionServiceAPIEndpoint
}
```

Why env vars first then config override: the most common pain is CI overriding the prod URL for staging. Env var is the cheapest knob.

Document all the env vars in `website/docs/index.html.markdown`. List them in the same order as the provider attributes.

If one URL is overridden, do not infer the others from it. Validate every supplied URL as absolute and normalise trailing slashes once in `Configure`.

---

## 4. Testing

See [testing.md](testing.md) for the detailed strategy (layers, the existing acceptance pattern, what to write for a new resource, sweepers, and CI splits). The short version:

- **Schema and helper unit tests** on every new resource. `ValidateImplementation` + table tests for converters. Include explicit bool-zero-value round-trip tests for every Required bool.
- **Acceptance tests** with at minimum: basic create, `PlanOnly: true` step (catches the inconsistent-result class), update path (or `_disappears` if immutable), import with `ImportStateVerify: true`.
- **`ImportStateVerifyIgnore`** only for legitimately unrecoverable fields (write-once secrets, timeouts). Add a comment naming why.
- **Sweepers** with a `tf-acc-` name prefix, when the API has a list endpoint. Object storage endpoints currently don't — accept manual cleanup there.

---

## 5. Documentation

### 5.1 Where prose lives, where schema lives

Keep the legacy `website/docs/{r,d}/<name>.html.markdown` layout. `tfplugindocs generate` writes to `docs/`, which we discard, copying only the schema block into the legacy file. This is deliberate.

`tfplugindocs` cannot render:

- **Plan modifier descriptions** ([terraform-plugin-docs#549](https://github.com/hashicorp/terraform-plugin-docs/issues/549)).
- **Validator descriptions** for framework providers ([terraform-plugin-docs#243](https://github.com/hashicorp/terraform-plugin-docs/issues/243)).
- **Default values, plan modifiers, validators in the JSON schema at all** ([terraform#35646](https://github.com/hashicorp/terraform/issues/35646)).

This is why the prose is hand-authored. Don't switch to the Registry `docs/` layout to "fix" it; the data isn't in the schema JSON.

**`MarkdownDescription` is the source of truth** for the per-attribute one-liner. It feeds the schema block, the Language Server hover, and is the only attribute documentation a user sees in their editor. Convention: write `MarkdownDescription`, leave `Description` unset (`tfplugindocs` always prefers `MarkdownDescription` if both are set, so this is safe).

### 5.2 What goes in `MarkdownDescription` vs the prose file

| Content | Place |
| --- | --- |
| One-line attribute purpose | `MarkdownDescription` |
| Allowed values, format constraints | `MarkdownDescription` (mirror the validator) |
| "Forces replacement", "computed by API" | `MarkdownDescription` (until `tfplugindocs` renders plan modifiers, do it ourselves) |
| Worked example | `website/docs/.../<name>.html.markdown` |
| Async/timeouts semantics | `website/docs/.../<name>.html.markdown` (Timeouts section) |
| Import format and recoverable attributes | `website/docs/.../<name>.html.markdown` (Import section) |
| API constraints not in schema | `website/docs/.../<name>.html.markdown` (Notes section) |
| Handling the secret (write-once fields) | `website/docs/.../<name>.html.markdown` (dedicated section) |

### 5.3 The doc template

Every `website/docs/r/<name>.html.markdown` follows this order:

```markdown
---
subcategory: "Storage"
page_title: "Nscale: nscale_object_storage_endpoint"
description: |-
  Manages an object storage endpoint in an Nscale project.
---

# Resource: nscale_object_storage_endpoint

Manages an object storage endpoint in an Nscale project.

~> **Note** Only one object storage endpoint can exist per project per region. Attempting to create a second will fail with HTTP 422.

## Example Usage

```hcl
resource "nscale_object_storage_endpoint" "main" {
  project_id = nscale_project.main.id
  region     = "eu-west-1"
}
```

<!-- schema block copied from tfplugindocs output -->

## Async behaviour

This resource provisions asynchronously. Terraform polls until the endpoint reaches `provisioned` status, up to the configured `create` timeout (default 5m).

## Timeouts

The `timeouts` block supports:

* `create` - (Default `5m`)
* `delete` - (Default `5m`)

## Import

Object storage endpoints can be imported using the endpoint ID:

```sh
terraform import nscale_object_storage_endpoint.main edp-abc123
```
```

For resources with composite IDs **and** write-once attributes, include both:

```markdown
## Import

Access keys can be imported using `<endpoint_id>/<access_key_id>`:

```sh
terraform import nscale_object_storage_access_key.main edp-abc123/ak-xyz789
```

~> **The `secret` attribute cannot be imported.** The Nscale API does not return secrets after creation. After import, Terraform will emit a warning and the `secret` attribute will be empty in state. To restore it, destroy and recreate.
```

### 5.4 Worked examples

`examples/<service>/main.tf` is the example the docs link to. Maintain it as the same simple "basic" configuration the acceptance test uses. Richer scenario stacks belong in subdirectories. CI should `terraform init && terraform plan` the example to catch silent drift between docs and code.

Convention: every new user-settable field must appear in either an example or an acceptance test. Steal this rule from the Google provider.

### 5.5 Document the operational behaviour, not just the schema

Each resource page must have these sections in this order if they apply:

1. `## Argument Reference` / `## Attribute Reference` (schema block from `tfplugindocs`)
2. `## Async behaviour` — what we poll for, what timeout applies
3. `## Timeouts` — block syntax, defaults
4. `## Import` — format, unrecoverable attributes
5. `## Handling the secret` — only for write-once secret resources; covers safe extraction (`terraform output -raw … | pbcopy`, `umask` files, secret-manager piping) and state-at-rest warning
6. `## Notes` — API constraints not in schema (the 422 cases)

---

## 6. Release and versioning

### 6.1 SemVer

Pre-1.0: every minor is allowed to break (and we should document each break clearly). Post-1.0: SemVer strictly.

Breaking changes for this framework specifically include:

- Removing an attribute, or renaming it.
- Changing an attribute's type.
- Changing Required ↔ Optional in either direction.
- Adding a new Required attribute without a default.
- Changing a default value of an Optional+Computed field.
- Adding `RequiresReplace` to an existing attribute (forces every existing resource to recreate on next plan).
- Changing schema version without an upgrader.
- Changing import ID format.

Not breaking:

- Adding Optional or Computed attributes.
- Tightening validation on a field that was previously accepting invalid values (this is a bug fix; document it).
- Adding new resources or data sources.

### 6.2 Deprecation

Use `DeprecationMessage` on the attribute. The framework will auto-warn when the field is set. Convention for the message: name the replacement, name the removal version.

```go
"legacy_field": schema.StringAttribute{
    Optional:           true,
    DeprecationMessage: "Use `new_field` instead. `legacy_field` will be removed in v2.0.",
},
"new_field": schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ExactlyOneOf(path.MatchRoot("legacy_field")),
    },
},
```

Deprecate in a minor release; remove in the next major. Don't silently rename attributes in place.

### 6.3 Schema versioning and state upgraders

When you must break state shape, bump `schema.Version` and implement `UpgradeState`:

```go
func (r *bucketResource) Schema(...) {
    resp.Schema = schema.Schema{
        Version: 1,
        // ...
    }
}

func (r *bucketResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            PriorSchema: &schema.Schema{ /* v0 schema */ },
            StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
                // migrate v0 → v1
            },
        },
    }
}
```

The framework expects each upgrader to convert the prior version directly to the current version; it does not chain through intermediate versions automatically. Increment by 1 per upgrade. Test every upgrader.

### 6.4 Changelog

`CHANGELOG.md`, sections: `### Breaking`, `### Features`, `### Bug fixes`, `### Documentation`. User-facing only — internal refactors don't appear. Reference the user's mental model: "the `versioned` attribute now persists `false` correctly" not "fixed `bucketAPIRequest.MarshalJSON`."

Every release note for a resource should answer three questions in one line: what changed, whether it is breaking, and whether the user needs to act.

---

## 7. Code organisation

Our current shape is fine for a small framework provider that ships weekly:

- service-scoped packages under `internal/services/<service>`;
- adjacent `*_resource.go`, `*_data_source.go`, `*_model.go`, and tests;
- a single registration point in `internal/provider/provider.go`.

Do **not** copy the large-provider package sprawl. The patterns worth stealing from AWS/Google/AzureRM are the named ones (`waitFooCreated`, `statusFoo`, shared diagnostics helpers, sweeper-name prefix discipline), not their directory trees.

Target shape to converge toward:

```text
internal/
  provider/
    provider.go
  services/
    common/
      diag.go          # API error -> diagnostics
      waiter.go        # shared retry/StateChangeConf helpers if needed
      importid.go      # strict composite import parsing
    region/
      region_resource.go
      region_data_source.go
      region_model.go
      region_resource_test.go
    compute/
      instance_resource.go
      instance_data_source.go
      instance_model.go
      instance_wait.go         # status* + wait* per resource
      instance_resource_test.go
    storage/
      endpoint_resource.go
      access_key_resource.go
      endpoint_data_source.go
      access_key_model.go
      access_key_resource_test.go
      sweep_test.go            # per-service sweeper (when API has list)
```

Add `internal/services/common/` only when there are concrete shared helpers. Don't pre-create empty packages.

The AzureRM convention adds `client/`, `helpers/`, `migration/`, `validate/` subpackages — adopt those only when we hit ~3+ items each. `client/` is worth adding to a service when the client wiring grows beyond a few lines.

**Don't adopt:**

- AWS's annotation-based registration (`@FrameworkResource`). Magic, codegen-dependent, not worth it at our scale.
- AzureRM's `magic-modules`-generated approach. Whole separate ecosystem.
- A `flex/` package for expand/flatten. Inline the conversion in `<name>_model.go`; we don't have shared shapes yet.

---

## Anti-patterns

1. **Don't use `Sensitive: true` and assume secrets are protected.** State is plaintext. Document, warn on import, and adopt the stash-on-Read pattern (§1.4, §2.3).
2. **Don't use `Optional+Computed` for every field "to be safe".** It silently masks server-side normalisation bugs and lets users override fields they shouldn't.
3. **Don't write a custom plan modifier to suppress a "false vs true" diff.** Fix the upstream `omitempty` in the converter (§1.6).
4. **Don't add provider-side caching of API constraints.** The API is the source of truth; surface its errors (§3.3).
5. **Don't write a Read method that returns an error for 404.** Call `resp.State.RemoveResource(ctx)` and return cleanly.
6. **Don't put resources inside `set` if any nested attribute is sensitive or write-once.** Sets and write-only are incompatible.
7. **Don't add `ImportStateVerifyIgnore` without a comment naming why.** Default position: every imported attribute must round-trip.
8. **Don't write Update methods that no-op when there are real changes.** Either implement Update or `RequiresReplace` + reject (§2.5).
9. **Don't switch to the Registry `docs/` layout to "fix" the doc generator.** The data isn't in the schema JSON; the layout switch doesn't help.
10. **Don't skip the `PlanOnly` step in acceptance tests.** It is the single most effective bug catch we have for the inconsistent-result class.
11. **Don't pass Terraform data straight into lossy generated openapi structs.** Wrap at the service boundary (§1.6).
12. **Don't apply `UseStateForUnknown` to transient status fields.** It only fits values expected to remain stable when unconfigured.
13. **Don't embed custom polling loops in each resource.** Use shared `StateChangeConf` (§2.2).
14. **Don't retry all 4xx responses.** Validation and auth failures are not transient and should fail fast.
15. **Don't change defaults or replacement behaviour in a patch release.** Configuration and state compatibility are the SemVer contract.

---

## Appendix: failure modes → playbook sections

| Failure mode | Sections |
| --- | --- |
| 1. `omitempty` bool round-trip → "inconsistent result after apply" | §1.6 (model boundary), [testing.md](testing.md) §4.3 (false round-trip acceptance step), Anti-patterns #3, #11 |
| 2. Write-once secrets (`secret` returned only on Create) | §1.4 (Sensitive), §2.3 (stash-on-Read pattern), §2.4 (import warning), [testing.md](testing.md) §4.2 (`ImportStateVerifyIgnore`), §5.2/5.5 (docs convention) |
| 3. API constraints not expressible in schema | §3.1 (surfacing the 422), §3.3 (validation layering), §5.5 (Notes section), Anti-pattern #4 |
| 4. Multi-URL service routing | §3.5 (env-first precedence) |
| 5. Async/long-running operations | §2.2 (StateChangeConf migration, per-resource waiters, default timeout table), §5.5 (Async behaviour + Timeouts sections), Anti-pattern #13 |
