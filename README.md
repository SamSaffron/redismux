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

##Master / Slave mode

Redismux can automatically configure master slave replication. 

The master service will ensure backup dbs are provisioned.

The backup service will ensure replication is fired and runs against the master. 

To configure

```
# for master (on 10.0.0.1) run:
./redismux -backup_redis 10.0.0.2:6379

# for slave (on 10.0.0.2) run:
./redismux -master_redis 10.0.0.1:6379

```

### Official Docker container

Redismux ships an official docker container, it is a tiny 12MB container that ships with both redismux and redis.

To run:

```
docker run -d -v /var/redismux:/redismux/dbs --restart=always --name redismux -p 6379:6379 samsaffron/redismux
```

This will keep all the data outside the container in the `/var/redismux` directory, and allows you to easily play with redismux.
