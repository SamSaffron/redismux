## redismux

A trivial redis multiplexer

This service will act as a middleman between N redis instances.

- It will automatically provision new redis instances if needed
- All child redis instances listen on a socket
- It will gracefully terminate all managed redis instances on SIGTERM

It provisions new redis instances on the fly by hijacking auth, the password
specified is the db it will connect to, for example:

```
% ./redismux -listen 0.0.0.0:9911


% redis-cli -p 9911 -a meta
redis 127.0.0.1:9911> set hello world
OK

% redis-cli -p 9911 -a meta
redis 127.0.0.1:9911> get hello
"world"

% redis-cli -p 9911 -a test
redis 127.0.0.1:9911> get hello
(nil)
```

This provides very simple way of provisioning redis instances.

All dbs are stored in the `db/` subdirectory together with .sock files
