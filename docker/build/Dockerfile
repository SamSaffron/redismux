FROM ubuntu:14.04

ENV REDIS_VERSION 2.8.17
ENV GOPATH /go

RUN apt-get -y update && apt-get -y upgrade && apt-get install -y git wget build-essential
RUN mkdir /src && cd /src && wget https://godeb.s3.amazonaws.com/godeb-amd64.tar.gz &&\
          tar -xzvf godeb-amd64.tar.gz && ./godeb install
RUN cd / && git clone https://github.com/SamSaffron/redismux.git
RUN mkdir /go && cd redismux && go build

RUN cd /src && wget http://download.redis.io/releases/redis-${REDIS_VERSION}.tar.gz
RUN cd /src && tar -xzvf redis-${REDIS_VERSION}.tar.gz
RUN cd /src && mv redis-${REDIS_VERSION} redis
RUN cd /src/redis && make



