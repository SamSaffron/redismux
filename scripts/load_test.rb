require 'redis'

$connection_count = 400
$db_count = 20
$key_count = 10000
$commands_per_connection = 100

def r(max)
  (Random.rand * max).to_i
end

def iteration
  redis = Redis.new(port: 8000, password: "test#{r($db_count)}")
  $commands_per_connection.times do
    redis.set("a#{r($key_count)}", 1)
    redis.get("a#{r($key_count)}")
  end
end


def test
  while true
    iteration rescue nil
  end
end


(0..$connection_count).map{ Thread.new{ test } }.each(&:join)

