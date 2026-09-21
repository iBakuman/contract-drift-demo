# Two ways a generated API contract fails you

[中文](README.zh-CN.md)

A runnable, five-minute demo of a problem that shows up in every
contract-first stack: an OpenAPI document, a generated server, a generated
client, and a bug that neither generator can see.

It is deliberately tiny — one endpoint, one response object, one optional
array. Everything here compiles. The type checker reports nothing. The client
still crashes.

The interesting part is that there are **two different failures** that look the
same from a distance, and they need **two different defences**. Confusing them
is how teams end up with one guard that they think covers both.

## Run it

Needs Go 1.24+, Node 20+, and pnpm. Generated code is checked in, so you do not
need the generators to run the demo.

```bash
cd frontend && pnpm install && cd ..

make demo       # start the server, call it three times, show what breaks
make check      # the server-side defence: validate real response bytes
make typecheck  # the client-side view: tsc --strict has nothing to say
```

`make demo` prints:

```
── scenario: ok ─── server builds the slice with make(...)
   raw response: {"generatedAt":"…","items":[{"id":"w-1","label":"left hinge"},…]}
   list  → rendered 2 widget(s): w-1, w-2
   title → LEFT HINGE, RIGHT HINGE

── scenario: nil-slice ─── server sends a null the contract forbids
   raw response: {"generatedAt":"…","items":null}
   list  → 💥 Cannot read properties of null (reading 'map')

── scenario: omit-optional ─── server legally leaves an optional field out
   raw response: {"generatedAt":"…","items":[{"id":"w-1"},{"id":"w-2"}]}
   list  → rendered 2 widget(s): w-1, w-2
   title → 💥 Cannot read properties of undefined (reading 'toUpperCase')
   label → w-1=undefined, w-2=undefined
```

## The contract

The whole thing is [`openapi.yaml`](openapi.yaml). Two fields matter:

```yaml
WidgetList:
  properties:
    items:          # optional, an array, and NOT nullable
      type: array
      items: { $ref: '#/components/schemas/Widget' }

Widget:
  required: [id]
  properties:
    label:          # optional — a server may leave it out
      type: string
```

Read that carefully, because both failures are hiding in the word *optional*,
used in two different senses.

For `items`, optional means: **the key may be absent, or its value is an
array.** `null` is not on that list. The contract never said this field could
be null, so a client is entitled to assume it never is.

For `label`, optional means the same thing — and here the server really does
exercise it. That is allowed. The client is the one that has to cope.

---

## Failure mode A — the server sends something the contract forbids

### What happens

`oapi-codegen` reads `items: optional array` and produces:

```go
type WidgetList struct {
    GeneratedAt string    `json:"generatedAt"`
    Items       *[]Widget `json:"items,omitempty"`
}
```

Optional becomes a pointer. That is the only tool Go's encoder offers for
"this key might not be there", and `omitempty` is how the pointer gets
dropped.

Now look at the two ways [`main.go`](backend/main.go) fills that field. Both
are ordinary Go. Both pass review.

```go
// wellBehaved
items := make([]api.Widget, 0, len(rows))
for _, row := range rows { items = append(items, …) }
return api.WidgetList{Items: &items}          // empty result → []

// nilSlice
items := byGroup["archived"]                  // no such key → nil slice
return api.WidgetList{Items: &items}          // empty result → null
```

The second one ships `"items": null`.

Here is the mechanism, and it is worth saying slowly because it is the whole
point: **`omitempty` asks the pointer, not the slice.** The pointer is real —
it points at a variable that exists — so the key stays. The encoder then asks
the slice what it looks like as JSON, and a nil slice is `null`.

`omitempty` cannot see through the pointer, and every `[]T` in Go can be nil.
So **every array field in this API has the same capability**, whatever the
generator settings. There is no code-generation option that takes it away.

### Why the client can't see it coming

`orval` reads the same line of the same contract and produces:

```ts
export interface WidgetList {
  generatedAt: string
  items?: Widget[]      // absent, or an array. Never null.
}
```

That is a *correct* reading. The contract really does say that. So the client
writes the idiomatic thing for an optional array:

```ts
function renderList({ items = [] }: WidgetList) {
  const ids = items.map((w) => w.id)     // items: Widget[]  ← tsc is certain
}
```

And a destructuring default **only fires on `undefined`**. `null` passes
straight through it, keeps the declared type `Widget[]`, and reaches `.map()`.

### The shape of it

Nobody is wrong. The Go type is a faithful generation of the contract. The TS
type is a faithful generation of the same contract. The client code is the
recommended way to handle an optional array. The only broken link is that the
server **emitted a value its own contract forbids**, and nothing in the system
was checking.

This is not a code-review problem. The demo makes the point by putting both
constructions in one file: `wellBehaved` never had this bug, `nilSlice` does,
and the difference is a `make` call several lines away from the field.

---

## Failure mode B — the server is correct and the client assumed too much

### What happens

`omitOptional` leaves `label` out. That is fully within the contract — `label`
is optional and the server declined to populate it.

The client, though, was written against the responses it saw during
development, where `label` was always there:

```ts
const titles = items.map((w) => w.label!.toUpperCase())
//                                     ↑ the non-null assertion
```

`tsc --strict` would have caught `w.label.toUpperCase()`. The `!` is how the
complaint gets silenced, and it is silenced for an honest-sounding reason:
"the server always sends it." The contract never promised that.

The quieter version of the same mistake does not even crash:

```ts
`${w.id}=${w.label}`     // → "w-1=undefined", rendered to the user
```

### The shape of it

Both sides are within the contract. The server exercised a freedom the
contract gave it, and the client had stopped treating that freedom as real.
There is no violation to detect here — which is exactly why the defence for A
does not help.

---

## Side by side

|  | **A — illegal null** | **B — unhandled optional** |
|---|---|---|
| Who broke the contract | the server | nobody |
| What the response looks like | `"items": null` | `"label"` key absent |
| Is the response legal? | **no** | **yes** |
| What the client sees | `null` where the type says `Widget[]` | `undefined` where the code assumed `string` |
| Detectable by validating the response? | **yes** | **no** — there is nothing to detect |
| Where the defence belongs | **server side** | **client side** |
| The defence | validate emitted bytes against the spec in tests | treat every `?` as real: no `!`, no casts, default at the boundary |

---

## Defence for A: check the bytes you actually sent

Types describe intent. The defence has to look at output.

[`backend/contractcheck`](backend/contractcheck/contractcheck.go) is an
`http.RoundTripper` that validates every response against the same OpenAPI
document the types came from — the generator embeds the spec, so there is no
second copy to keep in sync. Slide it under the `http.Client` your integration
tests already use and every call they already make gets checked, with no test
rewritten and no assertion changed:

```go
checker, _ := contractcheck.New(doc, nil)
client := &http.Client{Transport: checker}
```

It **reports rather than interrupts**: a violation is a finding about the
response, not a reason to fail the request. The test still runs its own
assertions, and a failure shows you both.

`make check` runs it against all three scenarios:

```
scenario ok             → no violations
scenario nil-slice      → response body doesn't match schema #/components/schemas/WidgetList:
                          Error at "/items": Value is not nullable
scenario omit-optional  → no violations
```

That third line is not a gap in the implementation. It is the correct answer.
Mode B produced a legal response.

### What this defence can't do

It only sees responses your tests actually made. An endpoint with no
integration coverage, or one that is covered but never hit its empty-list
branch, is invisible to it.

That is still a real change: it moves "can this reach production?" from
*depends on everyone remembering the `make` call, forever* to *depends on
whether a test hit that path* — which is something you can check, and fix.
It is not immunity.

## Defence for B: treat optional as optional

Nothing on the server can help here, because the server did nothing wrong. The
defence is client-side discipline:

- **Ban the escape hatches.** `@typescript-eslint/no-non-null-assertion` plus
  a rule against `as` casts on response data. Every `!` in code that touches a
  response is a place where someone overruled the contract from memory.
- **Default at the boundary, not at the use site.** Normalise the response
  once, where it arrives, instead of at each of the twelve places that read it.
- **Test against the contract's freedoms, not the server's habits.** Write the
  fixture where the optional field is missing. The server is allowed to start
  doing that tomorrow, without telling you.

## A trap worth naming

You may have noticed that `items ?? []` would have survived mode A, because
`??` catches `null` as well as `undefined`.

It would have — **by accident**. The author wrote `??` to handle *absent*, and
it happened to also handle a value the contract said could never occur. The
destructuring default `{ items = [] }` expresses the same intent and does not
survive, and neither does `if (items) …` if you reach for `.length` first, and
neither does any code that trusted the type.

Client-side null-coalescing is not a defence against mode A. It is a coin
flip on which idiom the author happened to pick. The defence against a server
sending illegal values is to stop the server sending them.

## Files

```
openapi.yaml                            the whole contract, ~40 lines
backend/main.go                         three handlers: correct, mode A, mode B
backend/api/types.gen.go                oapi-codegen output (checked in)
backend/contractcheck/contractcheck.go  the response validator
backend/contract_test.go                proves it catches A and not B
frontend/src/generated/widgets.ts       orval output (checked in)
frontend/src/consume.ts                 client code; passes tsc --strict
```

Generated with `oapi-codegen v2.8.0` and `orval v7.21.0`. `make generate`
re-runs both, if you edit the contract.

---

Both failures in this demo are reconstructions of things that have reached
production in real systems. The mechanism is the interesting part, not the
incident.
