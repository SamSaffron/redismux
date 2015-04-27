FROM gliderlabs/alpine

RUN mkdir /redismux
ADD files/redismux /redismux/redismux
ADD files/redis-server /redismux/bin/redis-server
ADD files/redis.conf /redismux/config/redis.conf
ADD start.sh /redismux/start.sh

EXPOSE 6379

WORKDIR /redismux

CMD /redismux/start.sh
