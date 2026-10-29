# monitors.bind — syntax version 1

This is the reference for the file format. The [manual](../manual.md)
explains what the records do; this page says exactly how a file is read
and how Unagikuma writes it back. Version 1 is stable: every file that
release 1.x accepts will be read the same way by later 1.x releases, and
additions only use names that version 1 reserves or rejects today.

Every `bind` example on this page is a complete file and is checked by the
test suite (`go test ./internal/config/ -run TestSpecExamples`).

## Lexical structure

A file is UTF-8 text, read line by line. `\r\n` line ends are accepted and
kept when the file is rewritten.

- **Whitespace.** Spaces and tabs separate tokens. How many is free; the
  formatter lines columns up, the parser does not care.
- **Comments.** A `;` that starts a token begins a comment that runs to the
  end of the line. A `;` inside a token is an ordinary character, so
  `anchors=lan:a;wan:b` is one value.
- **Quoted strings.** `"..."` may hold spaces, `;`, `(`, `)` and `=`.
  Inside quotes `\"` is a quote and `\\` a backslash; any other backslash
  is kept as is. A quoted section can sit inside a token: `title="Main
  site"` is the option `title` with the value `Main site`.
- **Parentheses.** `(` and `)` are tokens of their own. Everything between
  a `(` and its matching `)` belongs to one statement, across lines; this
  is how long records and the SOA are spread over several lines.
- **Statements.** Outside parentheses each non-empty line is one
  statement. A statement whose first token starts with `$` is a
  directive; every other statement is a record.

```ebnf
file        = { line } ;
line        = [ statement ] [ comment ] newline ;
statement   = directive | record ;
comment     = ";" { any character } ;

directive   = "$" NAME { token } ;
record      = name [ interval ] "IN" TYPE [ target ] { option } ;

name        = "@" | letter-or-digit { letter | digit | "_" | "." | "-" } ;   (* at most 63 characters *)
interval    = digit { letter | digit | "." } ;                                (* 30s, 1m, 1h30m, 500ms *)
TYPE        = letter { letter } ;                                             (* case-insensitive *)
target      = token ;                                                         (* any token that is not an option *)
option      = key "=" value ;
key         = letter { letter | digit | "." | "_" | "-" } ;                   (* case-insensitive, at most 64 characters *)
value       = { character | quoted } ;
token       = { character | quoted } | "(" | ")" ;
quoted      = '"' { character | '\"' | '\\' } '"' ;
```

### Targets and options

The first token after the type is the **target** unless it is an option.
A token is an option only when it is not entirely quoted and the text
before its first `=` is a valid key. So:

| Token                          | Read as                              |
|--------------------------------|--------------------------------------|
| `https://h/api?token=abc`      | target (`https://h/api?token` is not a key) |
| `"https://h/api?token=abc"`    | target (quoted)                      |
| `url=https://h/api?a=b`        | option `url`, value `https://h/api?a=b` |
| `"title=x"`                    | target (quoted)                      |
| `Title=x`                      | option `title` (keys are lower-cased) |

Every token after the target must be an option. An option may appear only
once per record. Values are taken literally except for references:
`${secret:NAME}` is replaced by a stored secret and `${env:NAME}` by an
environment variable the server was allowed to read.

## The SOA record

The SOA record is the first statement of the file; only comments and
`$VERSION` may come before it. `$QUORUM` and `$DIALECT` follow it, before
the first other record.

```
@ IN SOA <master-name> <admin-mailbox> ( <serial> <interval> <retry> <timeout> <attempts> )
```

- **serial** — 1 to 4294967295. A changed file is applied only when its
  serial is higher than the running one; `YYYYMMDDnn` is the usual form.
- **interval**, **retry**, **timeout** — durations (`500ms`, `15s`, `1m`,
  `1h30m`, `1d`): the default check interval, the interval while a problem
  is being confirmed, and the default timeout of one check.
- **attempts** — 1 to 100: how many failed checks in a row make a problem
  HARD and notified.
- **admin-mailbox** — written the BIND way (`admin.example.com.` is
  `admin@example.com`) or as a plain address.

```bind
$VERSION 1
; serial  interval  retry  timeout  attempts
@ IN SOA unagikuma. admin.example.com. (
        2026102901 ; serial
        1m         ; interval
        15s        ; retry
        5s         ; timeout
        3 )        ; attempts

www IN HTTP https://www.example.com/
```

## Directives

| Directive | Arguments | Where |
|-----------|-----------|-------|
| `$VERSION 1` | the syntax version; a file without it is version 1 | once, usually the first line |
| `$QUORUM v` | `majority`, `all`, `N` or `N%`: how many nodes must see a failure | after the SOA, before other records, once |
| `$DIALECT v` | `auto`, `huawei`, `cisco`, `asa`, `xr`: vendor meaning of ambiguous UDP keywords | after the SOA, before other records, once |
| `$SET key=value …` | tunables; each key once per file | anywhere |
| `$DEFAULT key=value …` | options added to the monitors after it, until the next `$DEFAULT`; an option on the record wins | anywhere |
| `$SCOPE spec [policy=NAME]` | nodes (`bj,sh`) or `region:NAME` that check the monitors after it, and their default policy; `$SCOPE *` returns to every node | anywhere |
| `$TAG key[:value] color=#rrggbb [description=…]` | how a tag is shown | anywhere |

`$INCLUDE` is recognised and rejected (`unsupported`). `$TIMEZONE` and
`$HOLIDAYS` are reserved (`needs_newer`). Any other directive is an error
(`unknown_directive`).

```bind
$VERSION 1
@ IN SOA unagikuma. admin.example.com. ( 2026102901 1m 15s 5s 3 )
$QUORUM majority

$SET notify.repeat=30m
$TAG site:bj color=#2f80ed description="Beijing office"

bj       IN NODE "Beijing office" region=north
sh       IN NODE "Shanghai office" region=north
ops      IN CHANNEL feishu url=${secret:ops_webhook}
default  IN POLICY chain=ops
north    IN POLICY chain=ops repeat=1h

$DEFAULT tags=site:bj
gw       IN PING 10.1.0.1
$SCOPE region:north policy=north
core-sw  IN PING 10.1.0.2 title="Core switch"
$SCOPE *
$DEFAULT
www  30s IN HTTP https://www.example.com/health?full=1 keyword=ok
```

## Records

Names are unique across all record types: a channel and a monitor cannot
share one. Only monitors have an interval; on other records it is ignored
with a warning (`interval_ignored`). Types are written in
upper case by convention and read in any case.

| Type | Kind | Target |
|------|------|--------|
| `HTTP`, `TCP`, `PING`, `UDP`, `DNS`, `NTP`, `SNMP`, `SYSLOG`, `PUSH`, `EXEC`, `REDIS`, `POSTGRES`, `MYSQL`, `GRPC`, `DOCKER`, `DOMAIN` | monitor | what is checked (URL, `host:port`, address, script, domain name); `PUSH` has none |
| `NODE` | probe node | display title |
| `CHANNEL` | where notifications go | the channel kind (`feishu`, `email`, `group`, …) |
| `POLICY` | which channels, how often | none |
| `PERIOD` | weekly time ranges | the ranges, quoted |
| `MAINT` | maintenance window | title |
| `STATUSPAGE` | public status page | title |

Options common to all monitors and never sent to the check itself:
`title`, `description`, `tags`, `group`, `device`, `handler`, `was`. The
options of each type are listed in the manual and by `unagikuma check`,
which names an unknown option and the line it is on.

`TEAM`, `SCHEDULE`, `ROTA`, `ROUTE`, `TEMPLATE`, `CHANGE` and `PERSON` are
reserved record types: a file using them is rejected with `needs_newer`, so
it fails clearly on a release that does not know them yet.

### Renaming

The name is how the file refers to a monitor, not what keeps its history.
Renaming in the web UI or API carries the history over. When renaming by
hand, write the old name once with `was=`; it can be removed after the
file has been applied.

```bind
@ IN SOA unagikuma. admin.example.com. ( 2026102902 1m 15s 5s 3 )

pg-primary IN TCP 10.0.0.5:5432 was=pg title="Postgres"
```

## How the file is written back

Saving from the web UI or API changes the text, not just the meaning:

- Only the lines of the records that changed are rewritten. Comments,
  blank lines, directives and the order of records are kept.
- A changed record keeps its option order, its quoting and its column
  alignment. A changed value is replaced in place; a removed option
  disappears with the space before it; a new option is added at the end
  of the record.
- A new record goes after the last record of the same `group=`, otherwise
  after the last record of its kind in its `$SCOPE` section; a first
  monitor goes before the maintenance windows and status pages. It is
  lined up with the record above it.
- The serial is raised: to today's `YYYYMMDD01` (UTC) or, if that is not
  higher, by one.
- Values are quoted only when they must be: when they are empty or hold a
  space, a tab, `;`, `(`, `)`, `"`, `\` or `=`.

`unagikuma fmt monitors.bind` rewrites a file in the canonical layout —
single-line records lined up in columns per block, option names in lower
case, quotes only where needed — and never changes its meaning;
`unagikuma fmt --check` exits with status 1 if a file is not formatted.
Records spread over several lines and everything that is not a record are
left as written.

## Diagnostics

`unagikuma check monitors.bind` prints `file:line:col: severity: message
[code]`; `--json` prints a list of `{file, line, col, severity, code,
message}`. The codes are part of version 1 and keep their meaning:

| Code | Meaning |
|------|---------|
| `syntax` | the statement does not have the shape above |
| `unterminated_string`, `unbalanced_paren`, `invalid_utf8` | lexical errors |
| `soa_first`, `missing_soa`, `duplicate_soa` | SOA placement |
| `invalid_serial`, `invalid_duration`, `invalid_value` | a value that cannot be read |
| `invalid_name`, `duplicate_name`, `duplicate_option` | names and options |
| `unknown_type`, `unknown_directive`, `unknown_option` | names this release does not know |
| `needs_newer`, `unsupported_version` | reserved for, or written for, a later release |
| `duplicate_directive`, `directive_order`, `duplicate_tunable`, `invalid_tunable` | directives |

A file with any error is never applied; the last good version keeps
running. Warnings (for example a timeout not shorter than the interval) do
not stop a file from being applied.
