# Missing Apple web session exits with the authentication code

Changed in 5.9.0. Some `asc web` commands need a signed-in Apple Account
session. When there is none and the command cannot sign in, it now fails with
authentication exit code `3` and does not print its usage page. This applies
to every web command except `asc web auth login`. It cannot sign in when:

- nothing is cached and no account is selected, or
- the selected account has no usable cached session, and `ASC_WEB_PASSWORD`,
  a saved password, and an interactive terminal are all unavailable.

Before 5.9.0, this failure exited with usage code `2` and printed the full
usage page. The new output names the next step:

```text
Error: no Apple web session is cached
Hint: asc web commands need a signed-in Apple Account session, and signing in needs an interactive terminal for the password and two-factor code. Run 'asc web auth login --apple-id EMAIL' in a terminal, or load a session exported elsewhere with 'asc web auth import --file FILE'. Unattended sign-in needs ASC_WEB_PASSWORD and ASC_WEB_2FA_CODE_COMMAND, plus --apple-id or ASC_WEB_APPLE_ID.
```

Where the public App Store Connect API can answer instead, the hint also names
that command. For example, `asc web review show` points to
`asc review status --app APP_ID`.

`asc web auth login` keeps exit code `2` for a missing `--apple-id` or password
source, and an ambiguous session cache still exits with `2`.

## Migration

Scripts that treated exit code `2` from a web command as "no web session"
should check for `3`, or check the session before running web commands:

```bash
asc web auth status --output json   # {"authenticated":false,...} and exit 0 when there is no session
```
