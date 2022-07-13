package ratelimiter

import (
	"time"
	"github.com/gin-gonic/gin"
	"github.com/juju/ratelimit"
)

type RateLimiter struct {
	buckets map[string]*ratelimit.Bucket
	capacity int64
	interval time.Duration
	getRateKey func(*gin.Context) (string,error)
	onLimitReached func(*gin.Context, string)
}

func NewRateLimiter(
	interval time.Duration,
	capacity int64,
	getRateKey func(c *gin.Context) (string,error),
	onLimitReached func(c *gin.Context, key string), // TODO: instead of this, allow overwriting member func using another method?
) RateLimiter {

	return RateLimiter{
		make(map[string]*ratelimit.Bucket),
		capacity,
		interval,
		getRateKey,
		onLimitReached,
	}
}

func (rl *RateLimiter) getBucket(key string) *ratelimit.Bucket {

	if bucket, exists := rl.buckets[key]; exists {
		return bucket
	}
	bucket := ratelimit.NewBucketWithQuantum(rl.interval,rl.capacity,rl.capacity)
	rl.buckets[key] = bucket
	return bucket
}

func (rl *RateLimiter) LimitRate() gin.HandlerFunc {
	return func(c *gin.Context) {
		ratekey,err := rl.getRateKey(c)

		// Don't do anything if encountered error getting key.
		// Error should be handled by the caller in `getRateKey`
		if err != nil {
			return
		}
		bucket := rl.getBucket(ratekey)
		if bucket.TakeAvailable(1) == 0 {
			rl.onLimitReached(c, ratekey)
			c.Abort()
		} else {
			c.Next()
		}
	}
}
