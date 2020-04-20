### Signals

* HUP - schedules a daemon exit within a minute. Useful for upgrading it via puppet as doing `systemctl restart` would kill currently running puppet


### Querying

Query engine uses [zygo](https://github.com/glycerine/zygomys). [Basic syntax](https://github.com/glycerine/zygomys/wiki/Language).

There is few added functions and global variables:

* `regexp` function matches value against regexp:
  * `(regexp (-> node %fqdn) "^dev.*")` matches any node whose fqdn matches `^dev.*` 
* `fact` function returns fact value:
  * `(== (fact "virtual") "kvm")` checks whether "virtual" fact matches "kvm"
  * request nested entries by just passing more parameters; `(== (fact "processors" "count") 4)` returns value of the `$processors["count"]` fact
  
  
