### Signals

* HUP - schedules a daemon exit within a minute. Useful for upgrading it via puppet as doing `systemctl restart` would kill currently running puppet


### Server config file

```
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
        blocked-net:
            type: host:net
            key: lb::blocked_nets
            rotate-interval: 1d
```




### Querying

Query engine uses [zygo](https://github.com/glycerine/zygomys). [Basic syntax](https://github.com/glycerine/zygomys/wiki/Language).

There is few added functions and global variables:

* `regexp` function matches value against regexp:
  * `(regexp (-> node %fqdn) "^dev.*")` matches any node whose fqdn matches `^dev.*` 
* `fact` function returns fact value:
  * `(== (fact "virtual") "kvm")` checks whether "virtual" fact matches "kvm"
  * request nested entries by just passing more parameters; `(== (fact "processors" "count") 4)` returns value of the `$processors["count"]` fact
* `class` returns present classes, could be used like `(== (class "systemd::common") true)`


### Data

  
### Examples

* `rv --out=csv puppet --filter '(== (class "systemd::common") true)'  status` - list puppet nodes containing that class

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
history is kept in `~/.rv_query_history` (`--history`, `--no-history`).

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

### Testing queries

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

## Feature list

### ...

## UDP server for VM info

Serves hypervisor info on UDP. Designed so VMs can have that mapped via serial port to get their parent info

## Modules

### Fence

default fence method will sysrq the host (sync -> umount -> reboot).

Fence needs to be enabled on server with ACLs on which node is allowed to fence what.
Either set `node_map` to matrix of nodes, or set `group`

This is NOT for security (checks are weak, password not implemented yet), just to avoid accidents


#### Config

```yaml
---
fence:
    enabled: true
    # maps clients to nodes it is allowed to fence
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
