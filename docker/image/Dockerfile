FROM ubuntu:14.04

RUN apt-get -y update && apt-get -y upgrade
RUN mkdir /redismux
ADD files/redismux /redismux/redismux
ADD files/redis-server /redismux/bin/redis-server
ADD files/redis.conf /redismux/config/redis.conf
