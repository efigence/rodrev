# rodrev

Puppet fleet orchestration over MQTT. `rvd` runs on every node, `rv` is the CLI. Both talk
through [go-zerosvc](https://github.com/zerosvc/go-zerosvc); a local checkout usually lives in
`~/src/my/go-zerosvc`, and library-level fixes belong there rather than being worked around here.

## Layout

| path | what lives there |
|---|---|
| `cmd/rv`, `cmd/rvd` | binaries. `cmd/rv/cobra.go` holds every flag; `cmd/rv/commands/*` are thin wrappers |
| `cmd/rv/clinit` | client startup: config files, logger, MQ connection. `Init` connects and panics on failure, `InitOffline` does neither |
| `common` | `Runtime` (node + transport + config), MQ node construction, node metadata, URL handling |
| `client` | client side RPC: `Session` (many calls over one reply subscription), discovery, `NodeWatcher` |
| `daemon` | rvd wiring, plus the retained-heartbeat cleanup |
| `plugin/puppet` | the puppet module: facts, classes, query harness, command handlers |
| `plugin/fence`, `plugin/ipset`, `downtime`, `hvminfo`, `web` | the other modules |
| `query` | the zygomys query engine. `query/repl` is the `rv query` REPL as a library |
| `t-data` | one node's real facts/classes/last run summary, the fixture everything tests against |

## Protocol

Topics are **relative to the event root** (`mq_prefix` minus its trailing slash); the node
prefixes them on send, so pass `puppet`, not `rv/puppet`.

- **Events**: `<sig length byte><signature><CBOR body>`. A first byte of `0x00` means unsigned,
  which is what rodrev sends. Bodies are CBOR, so `json.RawMessage` does not work for delayed
  decoding - `PuppetCmdRecv.Parameters` is a `cbor.RawMessage`.
- **Request/reply**: the client puts a relative path in `ReplyTo` plus a `correlation-id` header
  and subscribes `<path>/#`, so one channel serves many concurrent calls. `Runtime.Reply`
  propagates the correlation id; without it every answer looks the same.
- **Presence**: a retained JSON `NodeInfo` on `discovery/<node name>/<node uuid>`. It carries only
  name/uuid/ts/services, so everything else a client needs is the `Data` of the node's own
  `rodrev` service entry (`common.RodrevService`: fqdn, version, features, heartbeat interval).
  Anything on that tree without a `rodrev` entry is not a fleet node - the CLI announces itself
  there too. The MQTT will clears a node's presence when it disconnects.
- **Commands**: `status`, `run`, `fact`, `query`, `facts`, `classes`. `query` answers from every
  node (matched/not matched/failed) so counts are exact; the `answer_always` envelope flag does
  the same for any other command. Daemons advertise the list in `features`, and the client picks
  the exact path only when the whole fleet supports it.
- **Every daemon subscribes `puppet/#`**, so a request addressed to one node is delivered to all
  of them. `addressedToMe`/`unicastToMe` in `plugin/puppet/server.go` is what keeps one node's
  fact dump from being answered by the entire fleet. Treat it as load bearing.

## MQ traps

Every one of these cost a production outage or a crash loop. They are all invisible in a
short-lived CLI run and only bite something long-lived.

- **Always drain a subscription channel.** The transport hands messages over from its own reader,
  so a consumer that stops reading stalls the whole client - pings included - until the broker
  drops the connection. Every later subscribe then fails with `not currently connected`.
- **Retained messages arrive only when a subscription is made**, and re-subscribing to the same
  filter does not redeliver them. Repeated discovery in one process returns nothing the second
  time; use one long-lived subscription (`client.NodeWatcher`) and follow it.
- **Never close a channel the transport writes into.** The forwarding goroutine has no recover,
  so a late message panics the process. Drain instead.
- **One MQTT client id per process.** A shared client cert used to force the id to the cert CN,
  which made two `rv` runs kick each other off the broker; `clinit` passes
  `<nodename>-<random>` and the library only falls back to the CN when no id is given.
- **Subscriptions do not survive a reconnect** unless the transport restores them (fixed in
  zerosvc; before that a blip left a daemon heartbeating happily and deaf to every command).
- **`Publish` blocks while disconnected** - `token.Wait()` has no timeout, so a reply or a
  heartbeat can park for the length of an outage. It recovers on reconnect.
- **Not every payload is an event.** Anything can publish to a topic; a payload whose first byte
  is not zero used to be read as a signature length and crash the daemon on a nil verifier.

## Testing

`go test ./...` runs in a few seconds and needs no broker. `go vet ./...` and `gofmt -l .` are
expected to be clean. Use **`go test -race`** for anything touching the node, the heartbeat or a
subscription - that is how the `Services` map race was found.

- **`t-data/`** is the canonical fixture: `facts.yaml`, `classes.txt`, `last_run_summary.yaml` of
  one real node. Load it with `puppet.NewQueryHarness(dir, nil)`, `puppet.NewQueryHarnessOpts` or
  `puppet.LoadSnapshotFiles`. New query behaviour goes in the `queryTests` table in
  `plugin/puppet/query-harness_test.go`.
- **`client`**: `session_test.go` has a loopback `zerosvc.Transport` (in-process publish/subscribe
  with MQTT `#` matching) plus a fake daemon side, which covers correlation demuxing, deadlines,
  node errors and stray replies. `main_test.go` shortens `DefaultQueryTimeout` and
  `DefaultDiscoverOpts` for the suite - a broadcast call always runs to its deadline, so with the
  real values the suite would be minutes of waiting. `TestPackageDefaults` asserts the shipped
  values, so shortening them cannot hide a change to what production uses.
- **`plugin/puppet`**: `handleCommand` is pure, so `handler_test.go` builds a `Puppet` from a
  harness and needs no MQ, no daemon and no puppet binary (`New()` requires all three).
  `server_test.go` adds a fake transport for the `HandleEvent` plumbing.
- **`query/repl`**: `Session.EvalLine` and `Complete` are pure and table-tested; backends are
  faked (`fakeBackend`) and input is scripted (`scriptedReader`). `help_test.go` extracts every
  example from `:syntax` and runs it against `t-data`, so the built-in docs cannot rot.
- **`daemon`**: the cleanup decision (`staleHeartbeat`, `relativeTopic`, `jittered`) is pure; the
  pass itself takes a `cleanupPublisher`, so the fake records what would be removed.
- **Terminal behaviour**: `interruptResult` and `completeRunes` are pure, so most of the REPL
  needs no tty. For the rest, drive a real one with `python3 pty.fork` and set the window size
  with `TIOCSWINSZ` - readline reports "output is not a terminal" on a zero-size pty.
- **Live testing** against the mosquitto on `127.0.0.1:1883`: write a client and server config
  with `mq_prefix: rvtest/`, put a stub `puppet` executable on `PATH` (else `puppet.New` refuses
  to start), then run `rvd -c server.yaml` and point `rv` at it with `RV_CONFIG`. To test
  reconnect behaviour, connect the daemon through `socat TCP-LISTEN:11884,fork,reuseaddr
  TCP:127.0.0.1:1883` and kill the proxy; verify recovery by checking both that presence
  reappears **and** that requests are answered, because the interesting failure is a daemon that
  heartbeats but no longer listens.
- **Deliberately untested**: `sysrq` (writing to `/proc/sysrq-trigger` reboots the host), `web`,
  `hvminfo` (UDP/serial), `plugin/ipset` (shells out to `ipset`), `clinit` (connects and panics by
  design) and the thin `cmd/rv/commands/*` wrappers. Each needs root, hardware or a seam that
  does not exist yet.

## Conventions

- Errors name the node or topic they came from. A heartbeat that cannot be parsed is identified
  by its topic, since the payload is exactly what is broken.
- Never log a raw MQ URL: use `common.RedactURL`, and `common.causeOf` to strip the URL out of a
  `url.Parse` error, which embeds it password and all.
- Defaults that tests need to shorten are `var`, not `const`, and documented as such.
- Files that carry host data are `0600` and written via temp file plus rename: the query history
  (`~/.rv_query_history`) and saved snapshots.
- A filter that fails to parse is answered with an error rather than silence; silence is
  indistinguishable from "did not match" and that ambiguity has cost debugging time before.
