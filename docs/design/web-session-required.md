# Missing web session is an authentication failure

## Problem

`asc web` commands need a signed-in Apple Account session. Without one and
without a terminal, they failed as usage errors (exit `2`) and printed the
command's whole usage page.

- With nothing cached, the error was `--apple-id is required when no cached web
  session is available; run 'asc web auth login --apple-id EMAIL'`.
- With an account selected but no usable session, it was `password is required:
  run in a terminal for an interactive prompt or set ASC_WEB_PASSWORD`.

In non-interactive environments such as coding-agent sandboxes and CI, the
first message sends callers to `asc web auth login`. That command then stops on
the second message, and callers retry other web commands. Agents in 68 projects
hit this error 136 times. When stderr is merged into stdout, the usage page also
breaks JSON parsing: `DESCRIPTION` was the most common first line of
`2>&1 --output json` output.

## Decision

- Every web command except `asc web auth login` returns
  `shared.MissingWebSessionError` (matching `shared.ErrMissingWebSession`)
  when it has no usable session and cannot sign in. This covers:
  - the empty-cache case;
  - the selected-account case with no password from `ASC_WEB_PASSWORD`, the
    saved-password store, or a terminal;
  - `web apps create` without a terminal.
- The error maps to `ExitAuth` (`3`), like missing App Store Connect API
  credentials.
  - It is not a usage error, so no usage page is printed.
  - The root renderer prints the message and a `Hint:` line. The hint says
    that sign-in needs a terminal and names three ways forward:
    `asc web auth login`, `asc web auth import`, and the unattended sign-in
    variables.
  - Telemetry records it as a validation-stage `auth_error`.
- Where the public App Store Connect API answers the same question, the hint
  names that command:
  - `web review list` points to `asc review submissions-list`.
  - `web review show` and `web review threads` point to `asc review status`.
  - The other web commands have no public API equivalent and add nothing.
- No new telemetry diagnostic code is added. The collector validates codes
  against a fixed list, so a new code would drop events. Like missing App
  Store Connect credentials, the error carries no diagnostic. App Group
  commands keep attaching `authentication_rejected` to session failures.
- `asc web auth login` exists to create the session, so it keeps its usage
  errors (exit `2`) for a missing account or password. An ambiguous cache stays
  a usage error everywhere, because the fix is to pass `--apple-id`.

## Compatibility

The failure stays non-zero, but its exit code changes from `2` to `3`. The
command docs promised `2` for the empty-cache case. Exit codes cannot carry a
warning period, so the change ships in 5.9.0 with:

- updated command docs;
- the CI exit-code list;
- a release note (`docs/release-notes-web-session-required.md`) with migration
  guidance: check for `3`, or probe with `asc web auth status`, which returns
  `"authenticated":false` and exits `0`.

## Tests

- `internal/cli/cmdtest/web_session_required_test.go` runs the root command and
  asserts the exit code, the exact stderr, and the absence of the usage page.
  It covers:
  - privacy pull, agreements status, and review list, show, and threads;
  - the named-account case;
  - `web apps create`;
  - that `web auth login` keeps both usage errors.
- Unit tests cover:
  - the resolver for both cases and the sign-in context;
  - the public API alternative;
  - the exit-code mapping and telemetry classification;
  - the `errfmt` hint;
  - the `web auth capabilities` pass-through;
  - the App Group diagnostic.
