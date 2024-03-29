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
