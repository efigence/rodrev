# rodrev

Puppet fleet orchestration over MQTT.

* **`rvd`** - the daemon, runs on every managed node. Answers requests about puppet state,
  triggers puppet runs and hosts the optional modules (fencing, ipset, downtime, hvminfo).
* **`rv`** - the CLI. Short lived, connects to the same broker, asks the fleet and prints the
  answers.

There is no central server: the broker is the only shared component. Nodes announce themselves
with a retained heartbeat, so `rv` learns who exists by listening rather than by reading an
inventory file.

## Building

```
make            # builds ./rv and ./rvd for the local arch
make release    # release/{rv,rvd}.{amd64,arm64,i386} plus release/fence_rvd
make version    # version string that would get compiled in (git describe)
```

Only a Go toolchain is needed - no cgo. The version is stamped in at build time, so a binary
built with plain `go build` reports an empty version; use the Makefile.

Runtime requirements:

* an MQTT broker (mosquitto and friends) reachable from every node
* on nodes running `rvd`: the `puppet` binary in `PATH` or an executable `/usr/local/bin/puppet`.
  Without it the daemon refuses to start.
* the optional modules need their own tools: `ipset` for the ipset module, an Icinga2 API for
  downtimes, `/proc/sysrq-trigger` for fencing.

## Quick start

`/etc/rodrev/server.conf` on every node:

```yaml
---
mq_prefix: rv/
mq_address: tcp://mqtt:mqtt@mq.example.com:1883
node_meta:
    fqdn: node1.example.com
```

`/etc/rodrev/client.conf` wherever `rv` is run (usually the same file, minus the node specific
parts):

```yaml
---
mq_prefix: rv/
mq_address: tcp://mqtt:mqtt@mq.example.com:1883
```

Then:

```
rvd -c /etc/rodrev/server.conf          # normally started by systemd
rv status rodrev                        # who is out there
rv puppet status                        # last puppet run of every node
rv puppet run -n node1.example.com      # run puppet on one node
```

## Configuration

Both binaries read one YAML file. The first existing file of the list wins:

| binary | search order |
|---|---|
| `rvd` | `-c <file>`, `/etc/rodrev/server.conf`, `./cfg/server-local.yaml`, `./cfg/server.yaml` |
| `rv` | `-c <file>`, `$RV_CONFIG`, `$HOME/.config/rodrev/client.conf`, `/etc/rodrev/client.conf`, `./cfg/client-local.yaml`, `./cfg/client.yaml` |

`-c` pointing at a nonexistent file is a fatal error rather than a fallback. `rv` can also work
with no config file at all as long as `--mqtt-url` is given, and commands that do not need the
cluster (`rv query --data-dir ...`) do not need either.

### Common keys

Both sides read the same struct, so a key that only makes sense for one of them is simply
ignored by the other.

| key | used by | meaning |
|---|---|---|
| `mq_address` | both | broker URL. `tcp://`, `ssl://` (`tls://` is accepted and normalized). Credentials go in the URL |
| `mq_prefix` | both | topic root, trailing slash included. **No default** - leave it out and rodrev publishes at the root of the broker. It has to be identical on client and daemon |
| `ca_certs` | both | CA bundle for TLS. System CA store is used when empty |
| `client_cert` | both | client certificate (cert+key in one PEM). Its CN is used as the certname |
| `node_meta` | `rvd` | free form map exposed to filter expressions as the `node` variable. Nothing fills it in, so set at least `fqdn` if filters are going to use it |
| `debug` | both | same as `-d` |
| `fence` | `rvd` | fencing module, see below. `fence.group` is read by `rv` too, to tag its requests |
| `ipset` | `rvd` | ipset module |
| `hvm_info_server`, `hvm_info_client` | `rvd` | hypervisor info over UDP/serial |
| `icinga_api_url`, `icinga_api_user`, `icinga_api_pass` | `rvd` | enables the downtime module on this node |
| `heartbeat_cleanup` | `rvd` | removal of retained presence left by nodes that are gone |

### Server config file

```yaml
---
mq_prefix: rv/
mq_address: tls://dc1-mq.non.3dart.com:8883
ca_certs: /etc/rodrev/certs/ca.pem
client_cert: /etc/rodrev/certs/daemon-client.pem
node_meta:
    fqdn: d1-puppet1.example.com
    certname: d1-puppet1.example.com
    site: dc1
    mq_group: dc1
    project: dc1_puppet
    accounting_project: dc1
## optional, will listen to UDP port and serve node info
hvm_info_server:
    listen: 127.0.0.1:2121
## optional
ipset:
    sets:
        blocked-nets:
            name: blocked-nets
            type: hash:net
            broadcast_group: dc1
            timeout: 1h
```

### MQ URL

The broker URL can come from the config file (`mq_address`), from `--mqtt-url`, or from the
`RF_MQTT_URL` environment variable; the flag wins over the file. With none of them set both
binaries fall back to `tcp://mqtt:mqtt@127.0.0.1:1883`.

`ca` and `cert` query parameters override `ca_certs`/`client_cert`, which is the way to point at
certificates without a config file:

```
rv --mqtt-url 'ssl://user:pass@mq.example.com:8883/?ca=/etc/ssl/ca.pem&cert=/etc/rodrev/client.pem' status rodrev
```

URLs are redacted before they are logged, so a password in the config file does not end up in
the journal.

## Using rv

Global flags, valid for every subcommand:

| flag | meaning |
|---|---|
| `-c`, `--config` | config file |
| `--mqtt-url` | broker URL, overrides the config file |
| `-o`, `--output-format` | `stderr` (human readable, the default), `csv`, `json` |
| `-d`, `--debug` | log everything, including per-request chatter |
| `-q`, `--quiet` | warnings and errors only |

| command | what it does |
|---|---|
| `rv status rodrev` | discovery: which nodes are present, which are stale, what services they run |
| `rv puppet status` | last puppet run summary of every node (`rv status puppet` is the same command) |
| `rv puppet run` | trigger a puppet run |
| `rv puppet fact <name>` | value of one fact from every matching node |
| `rv query [expr]` | interactive REPL / one shot evaluation of filter expressions |
| `rv downtime <duration> [reason]` | schedule an Icinga2 downtime for this host |
| `rv fence run <node>` | fence a node |
| `rv fence status <node>` | check whether fencing answers on a node |
| `rv ipset add\|delete <group> <set> <addr>` | change an ipset across a broadcast group |
| `rv version` | version |

Human readable output goes to stderr, `csv` and `json` go to stdout, so redirecting stdout gives
a clean machine readable file with the log left on the terminal.

Exit codes are `0`/`1`/`2` (matched / did not match / error) for `rv query` and `rv fence status`;
the other commands report problems in the log and mostly exit `0`.

### Timing

A broadcast request has no idea how many answers to expect, so every command that asks the whole
fleet runs to a deadline:

* `rv puppet status`, `rv puppet fact` and `rv query` collect replies for **4 seconds**
  (`rv query --timeout`, `:timeout` in the REPL).
* discovery (`rv status rodrev`, node lists in the REPL) waits up to **10 seconds** for the first
  retained heartbeat, then stops **4 seconds** after the last one arrived.
* `rvd` heartbeats every **minute**, and a node counts as stale after three of its own intervals,
  at least 15 minutes. Stale means "the retained heartbeat is old", which usually means the node
  died without a clean disconnect.

### Puppet runs

```
rv puppet run -n node1.example.com                          # one node, right now
rv puppet run -n all -t 30m                                 # whole fleet, spread over 30 minutes
rv puppet run --filter '(== (class "nginx") true)' -t 5m     # only nodes matching the filter
rv puppet run -n node1.example.com --noop                   # puppet agent --noop
```

* `-n`/`--node` takes an fqdn or `all` (the default). `--target` is the deprecated spelling.
* `-t`/`--random-delay` is an upper bound: every node picks its own random delay below it, so the
  runs spread out instead of hitting the puppet master together. The daemon caps it at 24h.
* `-n all` without a delay is **refused** - it would be a self inflicted DDoS on the puppet
  master. Pass `-t 1s` if that is really what you want. With a `--filter` a 1s delay is assumed.
* One run at a time per node: a request arriving while puppet is running returns the current run
  status instead of starting a second run. The daemon runs
  `puppet agent --onetime --no-daemonize --verbose --no-splay --color=false` and logs its output.
* `--filter` works on every puppet subcommand and takes a query expression (see below).

`rvd` reads puppet's own state files - `/var/lib/puppet/facts.yaml`,
`/var/lib/puppet/state/classes.txt` and `/var/lib/puppet/state/last_run_summary.yaml` - and
refreshes them once a minute. Those paths are fixed. Facts and classes are what `fact`/`class`
in a filter expression, `rv puppet status` and `rv puppet fact` all answer from, so a node that
has never completed a puppet run answers with an error rather than a match.

### Querying

Query engine uses [zygo](https://github.com/glycerine/zygomys). [Basic syntax](https://github.com/glycerine/zygomys/wiki/Language).

There is few added functions and global variables:

* `regexp` function matches value against regexp:
  * `(regexp (-> node %fqdn) "^dev.*")` matches any node whose fqdn matches `^dev.*` 
* `fact` function returns fact value:
  * `(== (fact "virtual") "kvm")` checks whether "virtual" fact matches "kvm"
  * request nested entries by just passing more parameters; `(== (fact "processors" "count") 4)` returns value of the `$processors["count"]` fact
* `class` returns present classes, could be used like `(== (class "systemd::common") true)`
* `node` is the `node_meta` map from the daemon config file, reached with `(-> node %key)`. It is
  not facter data and it is empty unless the config file sets it.

A query has to end up as a boolean; a non-empty string or an int > 0 counts as true. Anything
else (a whole fact hash, for instance) is an error, and so is a filter that does not parse - the
node answers with the error rather than staying quiet, because silence is indistinguishable from
"did not match".

### Examples

* `rv --out=csv puppet --filter '(== (class "systemd::common") true)'  status` - list puppet nodes containing that class
* `rv query` - try a filter expression out before using it with `--filter` (see below)

### rv query - interactive query REPL

`rv query` builds and tests filter expressions before they are used with
`rv puppet --filter`. By default a typed expression is sent to the cluster and the
matching nodes are listed; with `--facts`/`--classes` (or `--data-dir`) it is evaluated
locally instead, with no MQ involved.

```
$ rv query --data-dir t-data
rv query - local files:t-data/facts.yaml, 97 facts, 117 classes
  (== (class "nginx") true)          which nodes have that class
  (== (fact "virtual") "kvm")        which nodes have that fact value
:help for commands, :syntax for the query language, TAB completes
rv(local)> (== (fact "os" "distro" "codename") "bookworm")
=> true
rv(local)> :fact apt_has*
apt_has_dist_updates: true
apt_has_updates: true
```

TAB completes meta commands, query functions, fact paths (`(fact "os" "distro" "<TAB>`)
and class names; queries are checked for syntax locally before being sent to the fleet, and
history is kept in `~/.rv_query_history` (`--history`, `--no-history`, `RV_QUERY_HISTORY`).

`--snapshot <fqdn>` pulls that node's data at startup, so completion works from the first
prompt in cluster mode. `--node-meta key=value` (repeatable) overrides entries of the `node`
variable, for checking what a query would do on a node with different metadata.

A snapshot can be kept: `:snapshot save prod-node.json` writes it out (mode 0600 - facts
describe a host in detail), `:snapshot load prod-node.json` reads it back, and
`rv query --snapshot-file prod-node.json` starts a local session against it. Pull one node
once, then iterate on queries with no cluster at hand.

Ctrl-C throws away the line being typed, or exits when there is nothing to throw away, so
pressing it twice always gets you out; Ctrl-D and `:quit` exit as well. Ctrl-C while a query
is running cancels that query and keeps the session.

Meta commands: `:help`, `:syntax` (query language reference with examples), `:fact`,
`:class`, `:nodes`, `:snapshot`, `:local`, `:cluster`, `:out`, `:timeout`, `:verbose`,
`:quit`.

Non-interactive use - expressions can also be passed with `-e` or on stdin, and the exit
code is 0 when something matched, 1 when nothing did, 2 on error:

```
rv query -e '(== (class "nginx") true)'                     # on the cluster
rv query --data-dir t-data -o json -e '(fact "is_virtual")' # local, machine readable
echo '(== (class "nginx") true)' | rv query -o csv
```

`:snapshot <fqdn>` pulls a node's full fact set and class list over the MQ, so completion,
`:fact` and `:class` work against real fleet data; `:local` then re-evaluates queries against
that snapshot with no round trip per query, which is the fast way to iterate on an
expression before letting it loose on the fleet.

How exact the cluster counts are depends on the daemons. An `rvd` that supports the `query`
command answers every query - matched, not matched, or "the query broke here" - so counts and
per-node errors are exact. Older daemons only answer when a filter matches, so a query
reports how many matched but can not tell "did not match" apart from "is down". `:nodes`
lists what discovery found and which of the two you are getting.

## Running rvd

`rvd` runs in the foreground and logs to stderr, which is what systemd wants. Flags:

| flag | meaning |
|---|---|
| `-c`, `--config` | config file |
| `--mqtt-url` | broker URL, overrides the config file |
| `-d`, `--debug` | verbose logging |
| `--profile-addr` | serve Go pprof on this address, e.g. `localhost:6060` |

### Signals

* HUP - schedules a daemon exit within a minute. Useful for upgrading it via puppet as doing `systemctl restart` would kill currently running puppet

### Local status socket

The daemon serves a small HTTP endpoint over a unix socket - `/run/rodrev/rvd.sock` when running
as root, `./rvd.sock` otherwise:

```
curl --unix-socket /run/rodrev/rvd.sock http://localhost/_status/health
curl --unix-socket /run/rodrev/rvd.sock http://localhost/_status/metrics
```

`/_status/health` returns JSON with one component per running module (`puppet`, `fencing`,
`ipset`, `icinga_api`) and answers with HTTP 500 when a module is not up; `/_status/metrics` is
the raw metric registry. Useful for a local check that tells "the daemon is connected and its
modules are alive" apart from "the process exists".

### Retained presence and its cleanup

A heartbeat is retained with no TTL, so a node clears its own with an empty retained message
via the MQTT will when it disconnects. A node that never got the chance leaves one behind, and
`rvd` cleans those up: the first pass runs a few minutes after start, then roughly weekly, both
randomized per node so a fleet does not do it in one spike. It removes retained presence whose
timestamp is older than `max_age`, whose payload can not be parsed, and anything still sitting
on the heartbeat topic older daemons used. Its own presence and any node still checking in are
left alone.

```yaml
heartbeat_cleanup:
    # disabled: true
    max_age: 720h      # how long a node may be silent before its presence goes
    interval: 168h     # roughly how often to look
    initial_delay: 5m  # first pass after start
    dry_run: false     # log what would go, remove nothing
```

The values above are the defaults. A node whose presence was removed reappears as soon as it
sends its next heartbeat, so cleaning up too eagerly only loses the record of nodes that are
already gone - but it does lose it, which is why `max_age` is a month.

## Modules

Every module except puppet is off until the config file enables it, and each one is independent:
a node can run only fencing, only ipset, or nothing but the puppet module.

### Downtime

Schedules an Icinga2 host downtime from the node that wants it, so a maintenance script does not
need Icinga credentials:

```
rv downtime 8h                  # 8h downtime for this host, no reason given
rv downtime 20m disk swap        # everything after the duration is the comment
rv downtime --host abc 30m
```

Only durations in `h`/`m`/`s` are accepted, and the maximum is 60 days.

The request is picked up by whichever daemon in the fleet has `icinga_api_url` set - usually the
one that can reach the Icinga API:

```yaml
icinga_api_url: https://icinga.example.com:5665/
icinga_api_user: rodrev
icinga_api_pass: secret
```

**A host may only downtime itself.** The requesting host is taken from the topic the request
arrived on, not from the message, and a request whose `--host` does not match the sender's short
hostname is logged and dropped. There is no reply, so check the daemon log (or Icinga) to confirm.

### Fence

Default fence method will sysrq the host (sync -> umount -> reboot). It is the node fencing
*itself* on request, so it only works while `rvd` on the target is still alive and connected -
it replaces a network-based fence agent for the common "kernel is fine, service is not" case,
not a power fence.

`rv fence run <node>` waits up to 21 seconds for the answer, which the target sends after it has
already gone read-only, roughly 11 seconds in; the reboot follows 20 seconds later.
`rv fence status <node>` only checks that the node answers, and exits 0/1 accordingly - it does
not verify that sysrq actually works.

`release/fence_rvd` (from `scripts/fence_rvd.pl`) is a pacemaker/stonith fence agent wrapping
`rv fence`, with the usual `--action reboot|status|monitor|metadata --nodename <fqdn>` interface.

Fence needs to be enabled on server with ACLs on which node is allowed to fence what.
Either set `node_map` to matrix of nodes, or set `group`

This is NOT for security (checks are weak, password not implemented yet), just to avoid accidents

#### Config

```yaml
---
fence:
    enabled: true
    # log what would happen and answer without fencing. For testing only
    # fake: true
    # maps clients to the nodes each of them is allowed to fence. The key is the
    # client node name, which is rf-client-<hostname> unless a client cert says
    # otherwise; it is the name that shows up in the daemon log when a request
    # is refused
    node_map: 
        rf-client-node1-fence:        
            nodes:
                - node1.example.com
                - node2.example.com
            password: asdg 
    # alternatively, define fence group with password,
    # every node in the group will be allowed to fence eachother
    group: sql
    group_password: nasudjb
```

With neither `group` nor `node_map` set, any node may fence this one. `fake: true` makes the
daemon acknowledge fence requests without doing anything and shouts about it in the log on every
start - it will eat your data if it is left on in production.

### ipset

Keeps an ipset in sync across a group of nodes: `rv ipset add <group> <set> <addr>` is broadcast
to every daemon whose config has a set with that name and broadcast group.

```yaml
ipset:
    sets:
        blocked-nets:
            name: blocked-nets        # the ipset itself, as named in the kernel
            type: hash:net            # hash:net, hash:ip or bitmap:ip
            broadcast_group: dc1      # nodes sharing this see the same commands
            timeout: 1h               # optional, entries expire after this
```

The daemon creates the sets at startup (so it needs root and the `ipset` binary) and refuses to
start the module if a set cannot be created or its type is unsupported. Commands are fire and
forget: there is no reply, and a command naming a set the node does not have is logged and
dropped. Use it for propagating short lived blocks, not as a source of truth - a node that was
down while an address was added does not learn about it later.

### UDP server for VM info

Serves hypervisor info on UDP. Designed so VMs can have that mapped via serial port to get their parent info

```yaml
## on the hypervisor
hvm_info_server:
    listen: 127.0.0.1:2121
```

A UDP packet containing `I` is answered with `{"fqdn":"..."}` of the hypervisor. On the guest
side the same daemon can read that off a serial port and write it out as a puppet fact:

```yaml
## in the VM
hvm_info_client:
    port: /dev/ttyS1
    baudrate: 115200
    puppet_fact_path: /etc/puppetlabs/facter/facts.d/vm_host.yaml
```

The client asks every 5 minutes and writes `vm_host` and `rodrev_version` facts, so
`(== (fact "vm_host") "hv1.example.com")` becomes a usable filter for "everything on that
hypervisor".

## Protocol

Events are CBOR bodies inside a signed-envelope frame, published under the configured
`mq_prefix` (`rv/` in every example here, but there is no built-in default): `rv/puppet` reaches
every node, `rv/puppet/<fqdn>` one of them, and replies go to a per-client
`rv/reply/<client>/<id>/<call>` topic that carries the correlation id back.

Presence works through retained heartbeats on `rv/discovery/<node name>/<node uuid>`, as plain
JSON node info. That info only carries the name, uuid, timestamp and service list, so
everything else a client needs - fqdn, daemon version, supported commands, heartbeat interval
- is published as the `Data` of the daemon's own `rodrev` service entry. Anything on that
topic tree without a `rodrev` service entry is not a fleet node (the cli announces itself
there too) and is ignored by discovery.

For TLS use `ssl://` in `mq_address` - `tls://` is accepted and normalized, since that is what
older rodrev configs use.

`rv` and `rvd` speak this protocol from the same release onwards and are not compatible with
older daemons, so both sides have to be upgraded together.

### Daemon commands

The puppet module answers these commands (`rv` sends them, the daemon announces the list in
its heartbeat as `features` so clients can tell an older daemon apart):

| command | what it does |
|---|---|
| `status` | last run summary, optionally filtered |
| `run` | trigger a puppet run |
| `fact` | value of one named fact |
| `query` | evaluate an expression and always answer with the result |
| `facts` | dump the whole fact set. Has to be addressed at a node (`puppet/<fqdn>`) or narrowed with a filter |
| `classes` | dump the class list, same addressing rules |

Any command can carry `answer_always`, which asks the nodes a filter did **not** match to
say so instead of staying quiet. The replies then tell you how many nodes actually ran the
filter, so `rv` does not have to ask heartbeats who is supposed to exist - useful because a
retained heartbeat only proves a node published one at some point, while an answer proves the
node received and evaluated this request. Daemons that predate the flag ignore it and stay
silent, so a mixed fleet still reports exact match counts and an approximate total.

Heartbeats are still what tells you which nodes *should* be there, including ones that are
down, which is why `rv query` reports both: `3/366 matched, 366 answered`.

Every daemon subscribes to the whole `puppet/#` tree, so a request addressed at one node is
delivered to all of them and the node itself decides whether it was meant for it. That is
why `facts`/`classes` refuse a bare broadcast: a fact dump is tens of kilobytes per node.

## Testing queries

`t-data/` holds an example node state (`facts.yaml`, `classes.txt`, `last_run_summary.yaml`)
and `puppet.QueryHarness` loads it into a query engine wired up the same way daemon does,
so CLI filter expressions can be checked without a running cluster:

```go
h, err := puppet.NewQueryHarness("../../t-data", nil)  // nil == generate `node` metadata out of facts
match, err := h.Query(`(== (class "systemd::common") true)`)
```

To check a new query just add a line to `queryTests` in `plugin/puppet/query-harness_test.go`:

```go
{query: `(== (fact "os" "distro" "codename") "bookworm")`, want: true},
{query: `(fact "os")`, wantErr: true}, // hash is not a boolean
```

and run `go test ./plugin/puppet/ -run TestQueryHarness -v`. Point the harness at your own
directory with the same three file names to test against another node's data.

Without any Go involved, `rv query --data-dir t-data` (or `--facts`/`--classes` pointing at a
real node's files) evaluates expressions against the same data from the command line.
