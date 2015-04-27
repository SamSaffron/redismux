FROM gliderlabs/alpine

RUN mkdir /redismux
ADD files/redismux /redismux/redismux
ADD files/redis-server /redismux/bin/redis-server
ADD files/redis.conf /redismux/config/redis.conf

EXPOSE 6379

CMD /redismux/redismux
