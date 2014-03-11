require 'minitest/pride'
require 'redis'

def new_redis(db = "test")
  Redis.new(port: 9911, password: db)
end
