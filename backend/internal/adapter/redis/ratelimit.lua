-- One attempt under a sliding-log rate limit (ADR 0021).
--
-- KEYS[1]  the limit's sorted set: one member per counted attempt, scored
--          by its time in milliseconds
-- ARGV[1]  now, in milliseconds, from the API's clock
-- ARGV[2]  the window, in milliseconds
-- ARGV[3]  the limit: attempts allowed in any window
-- ARGV[4]  a member unique to this attempt
--
-- Returns {1, 0} when the attempt is allowed and counted, and {0, wait}
-- when it is refused, wait being the milliseconds until the oldest counted
-- attempt leaves the window. A refused attempt is not counted, so hammering
-- a used-up limit never extends it.

local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- An attempt counts for exactly one window: at now - window it is gone.
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)

if redis.call('ZCARD', KEYS[1]) < limit then
  redis.call('ZADD', KEYS[1], now, ARGV[4])
  redis.call('PEXPIRE', KEYS[1], window)
  return {1, 0}
end

local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
return {0, tonumber(oldest[2]) + window - now}
