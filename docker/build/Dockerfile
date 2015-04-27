FROM gliderlabs/alpine

RUN apk --update add bash git go make gcc
RUN apk add build-base

RUN mkdir -p /gopath/src/github.com/samsaffron && cd /gopath/src/github.com/samsaffron && git clone https://github.com/samsaffron/redismux
ENV GOPATH /gopath
RUN mkdir -p /gopath
RUN cd /gopath/src/github.com/samsaffron/redismux && go get
RUN cd /gopath/src/github.com/samsaffron/redismux && CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w' .

ENV REDIS_VERSION 2.8.19
RUN mkdir /src && cd /src && wget http://download.redis.io/releases/redis-${REDIS_VERSION}.tar.gz

RUN cd /src && tar -xzvf redis-${REDIS_VERSION}.tar.gz
RUN cd /src && mv redis-${REDIS_VERSION} redis

# RUN sed -i 's/sys\/types.h/stdint.h/g' /src/redis/src/sha1.c
# RUN sed -i 's/u_int32_t/uint32_t/g' /src/redis/src/sha1.c
# RUN sed -i 's/u_int32_t/uint32_t/g' /src/redis/src/sha1.h

RUN apk add linux-headers
ADD no-backtrace.patch /src/redis/no-backtrace.patch
RUN cd /src/redis && patch -p1 -i no-backtrace.patch
RUN cd /src/redis && CFLAGS="$CFLAGS -D_GNU_SOURCE" make



