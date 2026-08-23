---
name: arandu-doctor
description: Read and act on aru doctor, the architecture checker of an Arandu (Go) application. Use when a doctor finding is reported, when a build passes but something feels unenforced, or before declaring any Arandu change finished — it is one of the gates. Also use when the request mentions "lint", "static analysis", "architecture check", "why is this failing", or a rule name such as grant-not-received, tenant-from-request, view-data-is-a-map or raw-output-is-not-a-component. Covers what each finding means, why it is never suppressed, and what the checker cannot see.
license: MIT
---

# Reading what the doctor says

`aru doctor` is this framework's architecture rules run as static analysis over
the parsed tree. Without it, mandatory architecture is documentation nobody
reads.

Every finding carries a file, a line, the rule name, what is wrong, and a **Why**
that says what breaks. The Why is the field that matters: a finding that only
says what is forbidden gets suppressed, and one that says what breaks gets
fixed.

```sh
aru doctor            # every finding
aru doctor --strict   # warnings fail too
```

## The procedure

**1. Read the Why, not the rule name.** The rule name tells you which check
fired. The Why tells you what a user of the application would experience. Fix
the second one.

**2. Fix the cause at the line it names.** Every finding points at real code.

**3. Never suppress.** There is no ignore comment and no allow-list, deliberately.
A finding you cannot fix is a design question, not a lint to silence.

**4. Run it again until it is clean, then run the other gates.**

## The findings you will actually meet

**`grant-not-received`** — a repository method takes no `security.Grant`. Every
caller gets the row, whoever asked. Add the Grant as the parameter before the id
and take it from the Policy.

**`tenant-from-request`** — the tenant is read from a path segment, a body, a
query or a header. A tenant that arrives with the request is a tenant the caller
chose. Read it with `data.Tenant(g)`.

**`repository-without-policy`** — a repository is reachable with no Policy
deciding. Write the Policy; the generator writes one that denies everything, and
you open it action by action.

**`resource-not-reauthorized`** — a handler authorized the action and then read a
row without authorizing the row. The first call answers "may this caller look at
all"; the second answers "may this caller look at *this*". Skipping the second
means any user of the same tenant sees the row.

**`view-data-is-a-map`** — a view was handed a map. A typo in a key is then a
blank space on a page that answered 200. Declare a struct that embeds
`view.Page`.

**`raw-output-is-not-a-component`** — `{!! x !!}` was given a value rather than a
call. The raw form escapes nothing, so a value that ever comes from a person is
stored cross-site scripting. Write `{{ x }}`, which escapes, or return it from a
component function.

**`policy-never-opened`** — a policy denies every action. On a new module this is
correct and expected; it is a warning so that a fresh project is not red on day
zero.

**`retired-module`** — an import names a module that no longer exists. The line
says what replaced it.

The last three checks run only under `--profile=performance`. What they report
is correct code on the conventional profile, and each says so in its own first
lines.

## What it cannot see, and why that matters

It reads the parsed tree, not the running program.

- SQL built from a variable, or held in a package constant, is not inspected. A
  clean report means no unscoped statement was **found**, not that none exists.
- Partition keys are not checked, because nothing in the code declares one.
- A build tag is invisible to it, so a file excluded from the compiler is still
  read.

Trust it as evidence, never as proof.
