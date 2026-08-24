-- KEYS[1] 限流 key；ARGV[1] 窗口毫秒；ARGV[2] 限制数；ARGV[3] 当前时间戳毫秒；ARGV[4] 唯一 member
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
redis.call('zremrangebyscore', KEYS[1], 0, now - window)
local count = redis.call('zcard', KEYS[1])
if count >= limit then
    return 0
end
redis.call('zadd', KEYS[1], now, ARGV[4])
redis.call('expire', KEYS[1], math.ceil(window / 1000) + 1)
return 1
