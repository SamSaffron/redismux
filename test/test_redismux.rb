require_relative './test_helper'

class TestMux < Minitest::Test

  def test_large_set
    redis = new_redis
    data = "a"*1000
    redis.set("test", data)
    got = redis.get("test")
    assert(got == data)
  end

  def test_cross_talk
    redis1 = new_redis("a")
    redis2 = new_redis("b")

    redis1.flushall
    redis2.flushall

    redis1.set "hello", "world"
    assert(redis2.get("hello") == nil)
    assert(redis1.get("hello") == "world")
  end

  def test_multi_connect
    redis1 = new_redis("a")
    redis2 = new_redis("a")

    redis1.set "hello", "worlds"
    assert(redis2.get("hello") == "worlds")
  end

  def test_reconnect
    redis1 = new_redis("a")
    redis1.set "hello", "frog"

    redis1.client.reconnect
    assert_equal(redis1.get("hello"), "frog")
  end


  def test_lots_of_connections
    items = Queue.new
    (0...1000).map do
      Thread.new do
        redis = new_redis("a")
        redis.set("hello", "world")
        redis.get("hello")
        items << 1
      end
    end.each(&:join)

    assert_equal(1000, items.length)
  end

end

